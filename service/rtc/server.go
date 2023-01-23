// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"runtime"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

const (
	udpSocketBufferSize = 1024 * 1024 * 16 // 16MB
	msgChSize           = 256
	signalingTimeout    = 10 * time.Second
)

type Server struct {
	cfg     ServerConfig
	log     mlog.LoggerIFace
	metrics Metrics

	groups   map[string]*group
	sessions map[string]SessionConfig

	udpConn net.PacketConn
	udpMux  ice.UDPMux

	sendCh    chan Message
	receiveCh chan Message
	drainCh   chan struct{}
	bufPool   *sync.Pool

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
		bufPool:   &sync.Pool{New: func() interface{} { return make([]byte, receiveMTU) }},
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
	if s.cfg.ICEHostOverride == "" && len(s.cfg.ICEServers) > 0 {
		addr, err := getPublicIP(s.cfg.ICEPortUDP, s.cfg.ICEServers.getSTUN())
		if err != nil {
			return fmt.Errorf("failed to get public IP address: %w", err)
		}
		s.cfg.ICEHostOverride = addr
		s.log.Info("got public IP address", mlog.String("addr", addr))
	}

	var conns []net.PacketConn
	for i := 0; i < runtime.NumCPU(); i++ {
		listenConfig := net.ListenConfig{
			Control: func(network, address string, c syscall.RawConn) error {
				return c.Control(func(fd uintptr) {
					err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
					if err != nil {
						s.log.Error("failed to set reuseaddr option", mlog.Err(err))
						return
					}
					err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
					if err != nil {
						s.log.Error("failed to set reuseport option", mlog.Err(err))
						return
					}
				})
			},
		}

		listenAddress := fmt.Sprintf("%s:%d", s.cfg.ICEAddressUDP, s.cfg.ICEPortUDP)
		udpConn, err := listenConfig.ListenPacket(context.Background(), "udp4", listenAddress)
		if err != nil {
			return fmt.Errorf("failed to listen on udp: %w", err)
		}

		s.log.Info(fmt.Sprintf("rtc: server is listening on udp %s", listenAddress))

		if err := udpConn.(*net.UDPConn).SetWriteBuffer(udpSocketBufferSize); err != nil {
			s.log.Warn("rtc: failed to set udp send buffer", mlog.Err(err))
		}

		if err := udpConn.(*net.UDPConn).SetReadBuffer(udpSocketBufferSize); err != nil {
			s.log.Warn("rtc: failed to set udp receive buffer", mlog.Err(err))
		}

		connFile, err := udpConn.(*net.UDPConn).File()
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

		conns = append(conns, udpConn)
	}
	var err error
	s.udpConn, err = newMultiConn(conns)
	if err != nil {
		return fmt.Errorf("failed to create multiconn: %w", err)
	}

	s.udpMux = webrtc.NewICEUDPMux(nil, s.udpConn)

	go s.msgReader()

	return nil
}

func (s *Server) Stop() error {
	var drainCh chan struct{}
	s.mut.Lock()
	if len(s.sessions) > 0 {
		s.log.Info("rtc: sessions ongoing, draining before exiting")
		drainCh = make(chan struct{})
		s.drainCh = drainCh
	} else {
		s.log.Debug("rtc: no sessions ongoing, exiting")
	}
	s.mut.Unlock()

	if drainCh != nil {
		<-drainCh
	}

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
				s.log.Error("failed to send sdp message: channel is full", mlog.Any("session", session.cfg))
			}
		case SDPMessage:
			var sdp webrtc.SessionDescription
			if err := json.Unmarshal(msg.Data, &sdp); err != nil {
				s.log.Error("failed to unmarshal sdp", mlog.Err(err), mlog.Any("session", session.cfg))
				continue
			}

			s.log.Debug("signaling", mlog.Int("sdpType", int(sdp.Type)), mlog.Any("session", session.cfg))

			if sdp.Type == webrtc.SDPTypeOffer && session.HasSignalingConflict() {
				s.log.Debug("signaling conflict detected, ignoring offer", mlog.Any("session", session.cfg))
				continue
			}

			var sdpCh chan webrtc.SessionDescription
			if sdp.Type == webrtc.SDPTypeOffer {
				sdpCh = session.sdpOfferInCh
			} else if sdp.Type == webrtc.SDPTypeAnswer {
				sdpCh = session.sdpAnswerInCh
			} else {
				s.log.Error("unexpected sdp type", mlog.Int("type", int(sdp.Type)), mlog.Any("session", session.cfg))
				return
			}
			select {
			case sdpCh <- sdp:
			default:
				s.log.Error("failed to send sdp message: channel is full", mlog.Any("session", session.cfg))
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
			session.mut.RLock()
			track := session.outVoiceTrack
			session.mut.RUnlock()
			if track == nil {
				continue
			}

			var enabled bool
			if msg.Type == UnmuteMessage {
				enabled = true
			} else {
				session.mut.Lock()
				if session.vadMonitor != nil {
					s.log.Debug("resetting vad monitor for session",
						mlog.String("sessionID", session.cfg.SessionID))
					session.vadMonitor.Reset()
				}
				session.mut.Unlock()
			}

			s.log.Debug("setting voice track state",
				mlog.Bool("enabled", enabled),
				mlog.String("sessionID", session.cfg.SessionID))

			session.mut.Lock()
			session.outVoiceTrackEnabled = enabled
			session.mut.Unlock()
		default:
			s.log.Error("received unexpected message type")
		}
	}
}
