// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

const (
	udpSocketBufferSize = 1024 * 1024 * 8 // 8MB
	msgChSize           = 256
	signalingTimeout    = 10 * time.Second
)

type Server struct {
	cfg     ServerConfig
	log     mlog.LoggerIFace
	metrics Metrics

	groups   map[string]*group
	sessions map[string]SessionConfig

	udpConn *net.UDPConn
	udpMux  ice.UDPMux

	sendCh    chan Message
	receiveCh chan Message

	mut sync.RWMutex
}

func NewServer(cfg ServerConfig, log mlog.LoggerIFace, metrics Metrics) (*Server, error) {
	if err := cfg.IsValid(); err != nil {
		return nil, err
	}
	if log == nil {
		return nil, fmt.Errorf("log should not be nil")
	}
	if metrics == nil {
		return nil, fmt.Errorf("metrics should not be nil")
	}

	s := &Server{
		cfg:       cfg,
		log:       log,
		metrics:   metrics,
		groups:    map[string]*group{},
		sessions:  map[string]SessionConfig{},
		sendCh:    make(chan Message, msgChSize),
		receiveCh: make(chan Message, msgChSize),
	}

	return s, nil
}

func (s *Server) Send(msg Message) error {
	select {
	case s.sendCh <- msg:
	default:
		return fmt.Errorf("failed to send rtc message, channel is full")
	}
	return nil
}

func (s *Server) ReceiveCh() <-chan Message {
	return s.receiveCh
}

func (s *Server) Start() error {
	var err error

	s.udpConn, err = net.ListenUDP("udp4", &net.UDPAddr{
		Port: s.cfg.ICEPortUDP,
	})
	if err != nil {
		return fmt.Errorf("failed to listen on udp: %w", err)
	}

	s.log.Info(fmt.Sprintf("rtc: server is listening on udp %d", s.cfg.ICEPortUDP))

	if err := s.udpConn.SetWriteBuffer(udpSocketBufferSize); err != nil {
		return fmt.Errorf("failed to set udp send buffer: %w", err)
	}

	if err := s.udpConn.SetReadBuffer(udpSocketBufferSize); err != nil {
		return fmt.Errorf("failed to set udp receive buffer: %w", err)
	}
	connFile, err := s.udpConn.File()
	if err != nil {
		return fmt.Errorf("failed to get udp conn file: %w", err)
	}
	defer connFile.Close()

	sysConn, err := connFile.SyscallConn()
	if err != nil {
		return fmt.Errorf("failed to get syscall conn: %w", err)
	}
	err = sysConn.Control(func(fd uintptr) {
		writeBufSize, err := syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF)
		if err != nil {
			s.log.Error("failed to get buffer size", mlog.Err(err))
			return
		}
		readBufSize, err := syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF)
		if err != nil {
			s.log.Error("failed to get buffer size", mlog.Err(err))
			return
		}
		s.log.Debug("rtc: udp buffers", mlog.Int("writeBufSize", writeBufSize), mlog.Int("readBufSize", readBufSize))
	})
	if err != nil {
		return fmt.Errorf("Control call failed: %w", err)
	}

	s.udpMux = webrtc.NewICEUDPMux(nil, s.udpConn)

	go s.msgReader()

	return nil
}

func (s *Server) Stop() error {
	if s.udpMux != nil {
		if err := s.udpMux.Close(); err != nil {
			return fmt.Errorf("failed to close udp mux: %w", err)
		}
	}

	if s.udpConn != nil {
		if err := s.udpConn.Close(); err != nil {
			return fmt.Errorf("failed to close udp conn: %w", err)
		}
	}

	close(s.receiveCh)
	close(s.sendCh)

	s.log.Info("rtc: server was shutdown")

	return nil
}

func (s *Server) msgReader() {
	for msg := range s.sendCh {
		if err := msg.IsValid(); err != nil {
			s.log.Error("invalid message", mlog.Err(err), mlog.Int("msgType", int(msg.Type)))
			continue
		}

		s.mut.RLock()
		cfg, ok := s.sessions[msg.SessionID]
		if !ok {
			s.mut.RUnlock()
			s.log.Error("session not found",
				mlog.String("sessionID", msg.SessionID),
				mlog.String("groupID", msg.GroupID),
				mlog.Int("msgType", int(msg.Type)))
			continue
		}
		s.mut.RUnlock()

		group := s.getGroup(cfg.GroupID)
		if group == nil {
			s.log.Error("group not found", mlog.String("groupID", cfg.GroupID))
			continue
		}

		call := group.getCall(cfg.CallID)
		if call == nil {
			s.log.Error("call not found", mlog.String("callID", cfg.CallID))
			continue
		}

		session := call.getSession(cfg.SessionID)
		if session == nil {
			s.log.Error("session not found", mlog.String("sessionID", cfg.SessionID))
			continue
		}

		switch msg.Type {
		case ICEMessage:
			select {
			case session.iceInCh <- msg.Data:
			default:
				s.log.Error("failed to send sdp message: channel is full")
			}
		case SDPMessage:
			select {
			case session.sdpInCh <- msg.Data:
			default:
				s.log.Error("failed to send sdp message: channel is full")
			}
		case ScreenOnMessage:
			data := map[string]string{}
			if err := json.Unmarshal(msg.Data, &data); err != nil {
				s.log.Error("failed to unmarshal screen msg data", mlog.Err(err))
				continue
			}

			s.log.Debug("received screen sharing stream ID", mlog.String("screenStreamID", data["screenStreamID"]))

			session.mut.Lock()
			session.screenStreamID = data["screenStreamID"]
			session.mut.Unlock()

			if ok := call.setScreenSession(session); !ok {
				s.log.Error("screen session should not be set")
			}
		case ScreenOffMessage:
			call.mut.Lock()
			if session == call.screenSession {
				call.screenSession = nil
			}
			call.mut.Unlock()
		case MuteMessage, UnmuteMessage:
			select {
			case session.trackEnableCh <- (msg.Type == MuteMessage):
			default:
				s.log.Error("failed to send track enable message: channel is full")
			}
		default:
			s.log.Error("received unexpected message type")
		}
	}
}
