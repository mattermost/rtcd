// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

const (
	msgChSize        = 256
	signalingTimeout = 10 * time.Second
	catchAllIP       = "0.0.0.0"
)

type Server struct {
	cfg     ServerConfig
	log     mlog.LoggerIFace
	metrics Metrics

	groups   map[string]*group
	sessions map[string]SessionConfig

	udpMux         ice.UDPMux
	tcpMux         ice.TCPMux
	publicAddrsMap map[string]string
	localIPs       []string

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
		cfg:            cfg,
		log:            log,
		metrics:        metrics,
		groups:         map[string]*group{},
		sessions:       map[string]SessionConfig{},
		sendCh:         make(chan Message, msgChSize),
		receiveCh:      make(chan Message, msgChSize),
		bufPool:        &sync.Pool{New: func() interface{} { return make([]byte, receiveMTU) }},
		publicAddrsMap: make(map[string]string),
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

	localIPs, err := getSystemIPs(s.log)
	if err != nil {
		return fmt.Errorf("failed to get system IPs: %w", err)
	}
	if len(localIPs) == 0 {
		return fmt.Errorf("no valid address to listen on was found")
	}

	s.localIPs = localIPs

	// Populate public IP addresses map if override is not set and STUN is provided.
	if s.cfg.ICEHostOverride == "" && len(s.cfg.ICEServers) > 0 {
		for _, ip := range localIPs {
			udpListenAddr := fmt.Sprintf("%s:%d", ip, s.cfg.ICEPortUDP)
			udpAddr, err := net.ResolveUDPAddr("udp4", udpListenAddr)
			if err != nil {
				s.log.Error("failed to resolve UDP address", mlog.Err(err))
				continue
			}

			// TODO: consider making this logic concurrent to lower total time taken
			// in case of multiple interfaces.
			addr, err := getPublicIP(udpAddr, s.cfg.ICEServers.getSTUN())
			if err != nil {
				s.log.Warn("failed to get public IP address for local interface", mlog.String("localAddr", ip), mlog.Err(err))
			} else {
				s.log.Info("got public IP address for local interface", mlog.String("localAddr", ip), mlog.String("remoteAddr", addr))
			}

			s.publicAddrsMap[ip] = addr
		}
	}

	if err := s.initUDP(localIPs); err != nil {
		return err
	}

	if err := s.initTCP(); err != nil {
		return err
	}

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

	if s.tcpMux != nil {
		if err := s.tcpMux.Close(); err != nil {
			return fmt.Errorf("failed to close udp mux: %w", err)
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

			if sdp.Type == webrtc.SDPTypeOffer && session.hasSignalingConflict() {
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
			call.clearScreenState(session)
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

func (s *Server) initUDP(localIPs []string) error {
	var udpMuxes []ice.UDPMux

	initUDPMux := func(addr string) error {
		conns, err := createUDPConnsForAddr(s.log, addr)
		if err != nil {
			return fmt.Errorf("failed to create UDP connections: %w", err)
		}

		udpConn, err := newMultiConn(conns)
		if err != nil {
			return fmt.Errorf("failed to create multiconn: %w", err)
		}

		udpMuxes = append(udpMuxes, ice.NewUDPMuxDefault(ice.UDPMuxParams{
			Logger:  newPionLeveledLogger(s.log),
			UDPConn: udpConn,
		}))

		return nil
	}

	// If an address is specified we create a single udp mux.
	if s.cfg.ICEAddressUDP != "" {
		if err := initUDPMux(fmt.Sprintf("%s:%d", s.cfg.ICEAddressUDP, s.cfg.ICEPortUDP)); err != nil {
			return err
		}
		s.udpMux = udpMuxes[0]
		return nil
	}

	// If no address is specified we create a mux for each interface we find.
	for _, ip := range localIPs {
		if err := initUDPMux(fmt.Sprintf("%s:%d", ip, s.cfg.ICEPortUDP)); err != nil {
			return err
		}
	}

	s.udpMux = ice.NewMultiUDPMuxDefault(udpMuxes...)

	return nil
}

func (s *Server) initTCP() error {
	tcpListener, err := net.Listen("tcp4", fmt.Sprintf("%s:%d", s.cfg.ICEAddressTCP, s.cfg.ICEPortTCP))
	if err != nil {
		return fmt.Errorf("failed to create TCP listener: %w", err)
	}

	s.tcpMux = ice.NewTCPMuxDefault(ice.TCPMuxParams{
		Logger:          newPionLeveledLogger(s.log),
		Listener:        tcpListener,
		ReadBufferSize:  tcpConnReadBufferLength,
		WriteBufferSize: tcpSocketWriteBufferSize,
	})

	return nil
}
