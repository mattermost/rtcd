// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/mattermost/rtcd/service/perf"
	"github.com/mattermost/rtcd/service/random"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/rtcd/logger"
	"github.com/pion/ice/v2"
	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
)

func setupServer(t *testing.T) (*Server, func()) {
	t.Helper()

	log, err := mlog.NewLogger()
	require.NoError(t, err)

	metrics := perf.NewMetrics("rtcd", nil)
	require.NotNil(t, metrics)

	cfg := ServerConfig{
		ICEPortUDP: 30433,
		ICEPortTCP: 30433,
	}

	s, err := NewServer(cfg, log, metrics)
	require.NoError(t, err)

	return s, func() {
		err := s.Stop()
		require.NoError(t, err)
		err = log.Shutdown()
		require.NoError(t, err)
	}
}

func TestNewServer(t *testing.T) {
	log, err := mlog.NewLogger()
	require.NoError(t, err)
	defer func() {
		err := log.Shutdown()
		require.NoError(t, err)
	}()

	metrics := perf.NewMetrics("rtcd", nil)
	require.NotNil(t, metrics)

	t.Run("invalid config", func(t *testing.T) {
		s, err := NewServer(ServerConfig{}, log, metrics)
		require.Error(t, err)
		require.Nil(t, s)
	})

	t.Run("missing logger", func(t *testing.T) {
		cfg := ServerConfig{
			ICEPortUDP: 30433,
			ICEPortTCP: 30433,
		}
		s, err := NewServer(cfg, nil, metrics)
		require.Error(t, err)
		require.Nil(t, s)
	})

	t.Run("missing metrics", func(t *testing.T) {
		cfg := ServerConfig{
			ICEPortUDP: 30433,
			ICEPortTCP: 30433,
		}
		s, err := NewServer(cfg, log, nil)
		require.Error(t, err)
		require.Nil(t, s)
	})

	t.Run("valid", func(t *testing.T) {
		cfg := ServerConfig{
			ICEPortUDP: 30433,
			ICEPortTCP: 30433,
		}
		s, err := NewServer(cfg, log, metrics)
		require.NoError(t, err)
		require.NotNil(t, s)
	})
}

func TestStartServer(t *testing.T) {
	log, err := mlog.NewLogger()
	require.NoError(t, err)
	defer func() {
		err := log.Shutdown()
		require.NoError(t, err)
	}()

	metrics := perf.NewMetrics("rtcd", nil)
	require.NotNil(t, metrics)

	cfg := ServerConfig{
		ICEPortUDP: 30433,
		ICEPortTCP: 30433,
	}

	t.Run("port unavailable", func(t *testing.T) {
		s, err := NewServer(cfg, log, metrics)
		require.NoError(t, err)
		require.NotNil(t, s)

		udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{
			Port: cfg.ICEPortUDP,
		})
		require.NoError(t, err)
		defer udpConn.Close()

		ips, err := getSystemIPs(log, false)
		require.NoError(t, err)
		require.NotEmpty(t, ips)

		err = s.Start()
		defer func() {
			err := s.Stop()
			require.NoError(t, err)
		}()
		require.Error(t, err)
		require.Equal(t, fmt.Sprintf("failed to create UDP connections: failed to listen on udp: listen udp4 %s:%d: bind: address already in use",
			ips[0], cfg.ICEPortUDP), err.Error())
	})

	t.Run("started", func(t *testing.T) {
		s, err := NewServer(cfg, log, metrics)
		require.NoError(t, err)
		require.NotNil(t, s)

		err = s.Start()
		defer func() {
			err := s.Stop()
			require.NoError(t, err)
		}()
		require.NoError(t, err)
	})
}

func TestDraining(t *testing.T) {
	log, err := mlog.NewLogger()
	require.NoError(t, err)
	defer func() {
		err := log.Shutdown()
		require.NoError(t, err)
	}()

	cfg := ServerConfig{
		ICEPortUDP: 30433,
		ICEPortTCP: 30433,
	}

	metrics := perf.NewMetrics("rtcd", nil)
	require.NotNil(t, metrics)

	t.Run("no session", func(t *testing.T) {
		s, err := NewServer(cfg, log, metrics)
		require.NoError(t, err)
		require.NotNil(t, s)

		err = s.Start()
		require.NoError(t, err)

		err = s.Stop()
		require.NoError(t, err)
	})

	t.Run("sessions ongoing", func(t *testing.T) {
		s, err := NewServer(cfg, log, metrics)
		require.NoError(t, err)
		require.NotNil(t, s)

		err = s.Start()
		require.NoError(t, err)

		s.mut.Lock()
		s.sessions["test"] = SessionConfig{}
		s.sessions["test1"] = SessionConfig{}
		s.mut.Unlock()

		go func() {
			time.Sleep(time.Second * 2)
			_ = s.CloseSession("test")
			_ = s.CloseSession("test1")
		}()

		beforeStop := time.Now()

		err = s.Stop()
		require.NoError(t, err)

		require.True(t, time.Since(beforeStop) > time.Second)
	})
}

func TestInitSession(t *testing.T) {
	log, err := logger.New(logger.Config{
		EnableConsole: true,
		ConsoleLevel:  "INFO",
	})
	require.NoError(t, err)
	defer func() {
		err := log.Shutdown()
		require.NoError(t, err)
	}()

	metrics := perf.NewMetrics("rtcd", nil)
	require.NotNil(t, metrics)

	cfg := ServerConfig{
		ICEPortUDP: 30433,
		ICEPortTCP: 30433,
	}

	s, err := NewServer(cfg, log, metrics)
	require.NoError(t, err)
	require.NotNil(t, s)

	nCalls := 10
	nSessionsPerCall := 10

	err = s.Start()
	require.NoError(t, err)

	var sessions []SessionConfig
	for i := 0; i < nCalls; i++ {
		callID := random.NewID()
		for j := 0; j < nSessionsPerCall; j++ {
			sessions = append(sessions, SessionConfig{
				GroupID:   "groupID",
				CallID:    callID,
				UserID:    random.NewID(),
				SessionID: random.NewID(),
			})
		}
	}

	for _, cfg := range sessions {
		go func(cfg SessionConfig) {
			err := s.InitSession(cfg, nil)
			require.NoError(t, err)
		}(cfg)
	}

	for _, cfg := range sessions {
		go func(id string) {
			err := s.CloseSession(id)
			require.NoError(t, err)
		}(cfg.SessionID)
	}

	err = s.Stop()
	require.NoError(t, err)
}

func connectSession(t *testing.T, cfg SessionConfig, s *Server, receiveCh chan Message) {
	t.Helper()

	connectCh := make(chan struct{})
	gatheringDoneCh := make(chan struct{})
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected {
			close(connectCh)
		}
	})

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			close(gatheringDoneCh)
			return
		}

		iceData, err := json.Marshal(candidate.ToJSON())
		require.NoError(t, err)

		err = s.Send(Message{
			GroupID:   cfg.GroupID,
			CallID:    cfg.CallID,
			UserID:    cfg.UserID,
			SessionID: cfg.SessionID,
			Type:      ICEMessage,
			Data:      iceData,
		})
		require.NoError(t, err)
	})

	dc, err := pc.CreateDataChannel("calls-dc", nil)
	require.NoError(t, err)
	require.NotNil(t, dc)

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)

	err = pc.SetLocalDescription(offer)
	require.NoError(t, err)

	offerData, err := json.Marshal(&offer)
	require.NoError(t, err)

	err = s.Send(Message{
		GroupID:   cfg.GroupID,
		CallID:    cfg.CallID,
		UserID:    cfg.UserID,
		SessionID: cfg.SessionID,
		Type:      SDPMessage,
		Data:      offerData,
	})
	require.NoError(t, err)

	var connectWg sync.WaitGroup
	iceCh := make(chan []byte, 20)
	connectWg.Add(1)
	go func() {
		defer connectWg.Done()
		for {
			select {
			case msg := <-receiveCh:
				if msg.Type == ICEMessage {
					iceCh <- msg.Data
				} else if msg.Type == SDPMessage {
					var answer webrtc.SessionDescription
					err := json.Unmarshal(msg.Data, &answer)
					require.NoError(t, err)
					err = pc.SetRemoteDescription(answer)
					require.NoError(t, err)

					go func() {
						for candidate := range iceCh {
							data := make(map[string]interface{})
							err := json.Unmarshal(candidate, &data)
							require.NoError(t, err)
							err = pc.AddICECandidate(webrtc.ICECandidateInit{Candidate: data["candidate"].(map[string]interface{})["candidate"].(string)})
							require.NoError(t, err)
						}
					}()
				}
			case <-connectCh:
				return
			case <-time.After(10 * time.Second):
				require.FailNow(t, "timed out connecting")
			}
		}
	}()

	select {
	case <-time.After(10 * time.Second):
		require.FailNow(t, "timed out gathering candidates")
	case <-gatheringDoneCh:
	}
	connectWg.Wait()
}

func TestCalls(t *testing.T) {
	log, err := logger.New(logger.Config{
		EnableConsole: true,
		ConsoleLevel:  "INFO",
	})
	require.NoError(t, err)
	defer func() {
		err := log.Shutdown()
		require.NoError(t, err)
	}()

	metrics := perf.NewMetrics("rtcd", nil)
	require.NotNil(t, metrics)

	cfg := ServerConfig{
		ICEPortUDP: 30433,
		ICEPortTCP: 30433,
	}

	s, err := NewServer(cfg, log, metrics)
	require.NoError(t, err)
	require.NotNil(t, s)

	nCalls := 5
	nSessionsPerCall := 5

	err = s.Start()
	require.NoError(t, err)

	receiveChans := make(map[string]chan Message)
	var sessions []SessionConfig
	groupID := random.NewID()
	for i := 0; i < nCalls; i++ {
		callID := random.NewID()
		for j := 0; j < nSessionsPerCall; j++ {
			sessionID := random.NewID()
			sessions = append(sessions, SessionConfig{
				GroupID:   groupID,
				CallID:    callID,
				UserID:    random.NewID(),
				SessionID: sessionID,
			})

			receiveChans[sessionID] = make(chan Message, 50)
		}
	}

	go func() {
		for msg := range s.ReceiveCh() {
			select {
			case receiveChans[msg.SessionID] <- msg:
			default:
				require.FailNow(t, "channel is full")
			}
		}
	}()

	var initWg sync.WaitGroup
	initWg.Add(len(sessions))
	for _, cfg := range sessions {
		go func(cfg SessionConfig) {
			defer initWg.Done()
			err := s.InitSession(cfg, nil)
			require.NoError(t, err)
		}(cfg)
	}
	initWg.Wait()

	var connectWg sync.WaitGroup
	connectWg.Add(len(sessions))
	for _, cfg := range sessions {
		go func(cfg SessionConfig) {
			defer connectWg.Done()
			connectSession(t, cfg, s, receiveChans[cfg.SessionID])
		}(cfg)
	}
	connectWg.Wait()

	var closeWg sync.WaitGroup
	closeWg.Add(len(sessions))
	for _, cfg := range sessions {
		go func(id string) {
			defer closeWg.Done()
			err := s.CloseSession(id)
			require.NoError(t, err)
		}(cfg.SessionID)
	}
	closeWg.Wait()

	err = s.Stop()
	require.NoError(t, err)
}

func TestTCPCandidates(t *testing.T) {
	log, err := logger.New(logger.Config{
		EnableConsole: true,
		ConsoleLevel:  "DEBUG",
	})
	require.NoError(t, err)
	defer func() {
		err := log.Shutdown()
		require.NoError(t, err)
	}()

	metrics := perf.NewMetrics("rtcd", nil)
	require.NotNil(t, metrics)

	serverCfg := ServerConfig{
		ICEPortUDP: 30433,
		ICEPortTCP: 30433,
	}

	s, err := NewServer(serverCfg, log, metrics)
	require.NoError(t, err)
	require.NotNil(t, s)

	err = s.Start()
	require.NoError(t, err)
	defer func() {
		err := s.Stop()
		require.NoError(t, err)
	}()

	cfg := SessionConfig{
		GroupID:   random.NewID(),
		CallID:    random.NewID(),
		UserID:    random.NewID(),
		SessionID: random.NewID(),
	}
	err = s.InitSession(cfg, nil)
	require.NoError(t, err)

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	defer pc.Close()

	dc, err := pc.CreateDataChannel("calls-dc", nil)
	require.NoError(t, err)
	require.NotNil(t, dc)

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)

	err = pc.SetLocalDescription(offer)
	require.NoError(t, err)

	offerData, err := json.Marshal(&offer)
	require.NoError(t, err)

	err = s.Send(Message{
		GroupID:   cfg.GroupID,
		CallID:    cfg.CallID,
		UserID:    cfg.UserID,
		SessionID: cfg.SessionID,
		Type:      SDPMessage,
		Data:      offerData,
	})
	require.NoError(t, err)

	for msg := range s.ReceiveCh() {
		if msg.Type == ICEMessage {
			data := make(map[string]any)
			err := json.Unmarshal(msg.Data, &data)
			require.NoError(t, err)

			iceString := data["candidate"].(map[string]interface{})["candidate"].(string)

			candidate, err := ice.UnmarshalCandidate(iceString)
			require.NoError(t, err)

			require.Equal(t, ice.CandidateTypeHost, candidate.Type())
			require.Equal(t, serverCfg.ICEPortTCP, candidate.Port())

			if candidate.NetworkType() == ice.NetworkTypeTCP4 {
				break
			}
		}
	}

	err = s.CloseSession(cfg.SessionID)
	require.NoError(t, err)
}
