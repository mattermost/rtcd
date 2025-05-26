// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/webrtc/v4"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

const (
	msgChSize        = 2000
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
	publicAddrsMap map[netip.Addr]string
	localIPs       []netip.Addr

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
		publicAddrsMap: make(map[netip.Addr]string),
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
	udpNetwork := "udp4"
	tcpNetwork := "tcp4"

	if s.cfg.EnableIPv6 {
		s.log.Info("rtc: experimental IPv6 support enabled")
		udpNetwork = "udp"
		tcpNetwork = "tcp"
	}

	localIPs, err := getSystemIPs(s.log, s.cfg.EnableIPv6)
	if err != nil {
		return fmt.Errorf("failed to get system IPs: %w", err)
	}
	if len(localIPs) == 0 {
		return fmt.Errorf("no valid address to listen on was found")
	}

	s.localIPs = localIPs

	s.log.Debug("rtc: found local IPs", mlog.Any("ips", s.localIPs))

	if m, _ := s.cfg.ICEHostPortOverride.ParseMap(); len(m) > 0 {
		s.log.Debug("rtc: found ice host port override mappings", mlog.Any("mappings", s.cfg.ICEHostPortOverride))

		for _, ip := range localIPs {
			if port, ok := m[ip.String()]; ok {
				s.log.Debug("rtc: found port override for local address", mlog.String("address", ip.String()), mlog.Int("port", port))
				s.cfg.ICEHostPortOverride = ICEHostPortOverride(fmt.Sprintf("%d", port))
				// NOTE: currently not supporting multiple ip/port mappings for the same rtcd instance.
				break
			}
		}
	}

	// Populate public IP addresses map if override is not set and STUN is provided.
	if s.cfg.ICEHostOverride == "" && len(s.cfg.ICEServers) > 0 {
		for _, ip := range localIPs {
			udpListenAddr := netip.AddrPortFrom(ip, uint16(s.cfg.ICEPortUDP)).String()
			udpAddr, err := net.ResolveUDPAddr(udpNetwork, udpListenAddr)
			if err != nil {
				s.log.Error("failed to resolve UDP address", mlog.Err(err))
				continue
			}

			// TODO: consider making this logic concurrent to lower total time taken
			// in case of multiple interfaces.
			addr, err := getPublicIP(udpAddr, udpNetwork, s.cfg.ICEServers.getSTUN())
			if err != nil {
				s.log.Warn("failed to get public IP address for local interface", mlog.String("localAddr", ip.String()), mlog.Err(err))
			} else {
				s.log.Info("got public IP address for local interface", mlog.String("localAddr", ip.String()), mlog.String("remoteAddr", addr))
			}

			s.publicAddrsMap[ip] = addr
		}
	}

	if err := s.initUDP(localIPs, udpNetwork); err != nil {
		return err
	}

	if err := s.initTCP(tcpNetwork); err != nil {
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

	close(s.receiveCh)
	close(s.sendCh)

	if s.tcpMux != nil {
		if err := s.tcpMux.Close(); err != nil {
			return fmt.Errorf("failed to close tcp mux: %w", err)
		}
	}

	if s.udpMux != nil {
		if err := s.udpMux.Close(); err != nil {
			return fmt.Errorf("failed to close udp mux: %w", err)
		}
	}

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
			if err := s.handleIncomingSDP(session, s.receiveCh, msg.Data); err != nil {
				s.log.Error("failed to handle incoming sdp", mlog.Err(err), mlog.Any("session", session.cfg))
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
			if err := call.clearScreenState(session); err != nil {
				s.log.Error("failed to clear screen state", mlog.Err(err))
			}
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

func (s *Server) initUDP(localIPs []netip.Addr, network string) error {
	var udpMuxes []ice.UDPMux

	initUDPMux := func(addr string) error {
		conns, err := createUDPConnsForAddr(s.log, network, addr, s.cfg.UDPSocketsCount)
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

	// If an address is specified we create udp mux for each address specified (it could a comma separated list).
	if s.cfg.ICEAddressUDP != "" {
		for _, ip := range s.cfg.ICEAddressUDP.Parse() {
			if err := initUDPMux(net.JoinHostPort(ip, fmt.Sprintf("%d", s.cfg.ICEPortUDP))); err != nil {
				return err
			}
			s.udpMux = ice.NewMultiUDPMuxDefault(udpMuxes...)
		}

		return nil
	}

	// If no address is specified we create a UDP mux for each interface we find.
	for _, ip := range localIPs {
		if err := initUDPMux(netip.AddrPortFrom(ip, uint16(s.cfg.ICEPortUDP)).String()); err != nil {
			return err
		}
	}

	s.udpMux = ice.NewMultiUDPMuxDefault(udpMuxes...)

	return nil
}

func (s *Server) initTCP(network string) error {
	initTCPMux := func(host string) (ice.TCPMux, error) {
		tcpListener, err := net.Listen(network, net.JoinHostPort(host, fmt.Sprintf("%d", s.cfg.ICEPortTCP)))
		if err != nil {
			return nil, fmt.Errorf("failed to create TCP listener: %w", err)
		}

		return ice.NewTCPMuxDefault(ice.TCPMuxParams{
			Logger:          newPionLeveledLogger(s.log),
			Listener:        tcpListener,
			ReadBufferSize:  tcpConnReadBufferLength,
			WriteBufferSize: tcpSocketWriteBufferSize,
		}), nil
	}

	// If an address is specified we create a TCP mux for each address specified (it could a comma separated list).
	if s.cfg.ICEAddressTCP != "" {
		var tcpMuxes []ice.TCPMux
		for _, ip := range s.cfg.ICEAddressTCP.Parse() {
			tcpMux, err := initTCPMux(ip)
			if err != nil {
				return err
			}

			tcpMuxes = append(tcpMuxes, tcpMux)
		}

		s.tcpMux = ice.NewMultiTCPMuxDefault(tcpMuxes...)

		return nil
	}

	tcpMux, err := initTCPMux(string(s.cfg.ICEAddressTCP))
	if err != nil {
		return err
	}

	s.tcpMux = tcpMux

	return nil
}

func (s *Server) handleIncomingSDP(us *session, answerCh chan<- Message, data []byte) error {
	var sdp webrtc.SessionDescription
	if err := json.Unmarshal(data, &sdp); err != nil {
		return fmt.Errorf("failed to unmarshal sdp: %w", err)
	}

	s.log.Debug("signaling", mlog.Int("sdpType", int(sdp.Type)), mlog.Any("session", us.cfg))

	if sdp.Type == webrtc.SDPTypeOffer {
		select {
		case us.sdpOfferInCh <- offerMessage{sdp: sdp, answerCh: answerCh}:
		default:
			return fmt.Errorf("failed to send sdp offer: channel is full")
		}
	} else if sdp.Type == webrtc.SDPTypeAnswer {
		select {
		case us.sdpAnswerInCh <- sdp:
		default:
			return fmt.Errorf("failed to send sdp answer: channel is full")
		}
	} else {
		return fmt.Errorf("unexpected sdp type: %d", sdp.Type)
	}

	return nil
}

func (s *Server) GetSessionConfig(groupID, callID, sessionID string) (SessionConfig, error) {
	group := s.getGroup(groupID)
	if group == nil {
		return SessionConfig{}, fmt.Errorf("group not found")
	}

	call := group.getCall(callID)
	if call == nil {
		return SessionConfig{}, fmt.Errorf("call not found")
	}

	session := call.getSession(sessionID)
	if session == nil {
		return SessionConfig{}, fmt.Errorf("session not found")
	}

	return session.cfg, nil
}

func (s *Server) GetSessionConfigs(groupID, callID string) ([]SessionConfig, error) {
	group := s.getGroup(groupID)
	if group == nil {
		return nil, fmt.Errorf("group not found")
	}

	call := group.getCall(callID)
	if call == nil {
		return nil, fmt.Errorf("call not found")
	}

	call.mut.RLock()
	defer call.mut.RUnlock()
	if len(call.sessions) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}

	cfgs := make([]SessionConfig, 0, len(call.sessions))
	for _, session := range call.sessions {
		cfgs = append(cfgs, session.cfg)
	}

	return cfgs, nil
}
