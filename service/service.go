// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"fmt"
	"net/http/pprof"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/mattermost/rtcd/logger"
	"github.com/mattermost/rtcd/service/api"
	"github.com/mattermost/rtcd/service/auth"
	"github.com/mattermost/rtcd/service/perf"
	"github.com/mattermost/rtcd/service/rtc"
	"github.com/mattermost/rtcd/service/store"
	"github.com/mattermost/rtcd/service/ws"

	godeltaprof "github.com/grafana/pyroscope-go/godeltaprof/http/pprof"
	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

type Service struct {
	cfg          Config
	apiServer    *api.Server
	wsServer     *ws.Server
	rtcServer    *rtc.Server
	store        store.Store
	auth         *auth.Service
	metrics      *perf.Metrics
	log          *mlog.Logger
	sessionCache *auth.SessionCache
	// connMap maps user sessions to the websocket connection they originated
	// from. This is needed to keep track of the MM instance end users are
	// connected to in order to route any message to it and avoid the additional
	// intra-cluster messaging layer that can introduce race conditions.
	connMap map[string]string
	mut     sync.RWMutex
}

func New(cfg Config) (*Service, error) {
	if err := cfg.IsValid(); err != nil {
		return nil, err
	}

	s := &Service{
		cfg:     cfg,
		metrics: perf.NewMetrics("rtcd", nil),
		connMap: map[string]string{},
	}

	var err error
	s.log, err = logger.New(cfg.Logger)
	if err != nil {
		return nil, fmt.Errorf("rtcd: failed to init logger: %w", err)
	}

	s.log.Info("rtcd: starting up", getVersionInfo().logFields()...)

	s.store, err = store.New(cfg.Store.DataSource)
	if err != nil {
		return nil, fmt.Errorf("failed to create store: %w", err)
	}
	s.log.Info("initiated data store", mlog.String("DataSource", cfg.Store.DataSource))

	s.sessionCache, err = auth.NewSessionCache(cfg.API.Security.SessionCache)
	if err != nil {
		return nil, fmt.Errorf("failed to create session cache: %w", err)
	}

	s.auth, err = auth.NewService(s.store, s.sessionCache)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth service: %w", err)
	}
	s.log.Info("initiated auth service")

	s.apiServer, err = api.NewServer(cfg.API.HTTP, s.log)
	if err != nil {
		return nil, fmt.Errorf("failed to create api server: %w", err)
	}

	wsConfig := ws.ServerConfig{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    10 * time.Second,
	}
	s.wsServer, err = ws.NewServer(wsConfig, s.log, ws.WithAuthCb(s.authHandler))
	if err != nil {
		return nil, fmt.Errorf("failed to create ws server: %w", err)
	}

	s.rtcServer, err = rtc.NewServer(cfg.RTC, s.log, s.metrics)
	if err != nil {
		return nil, fmt.Errorf("failed to create rtc server: %w", err)
	}

	s.apiServer.RegisterHandleFunc("/version", s.getVersion)
	s.apiServer.RegisterHandleFunc("/login", s.loginClient)
	s.apiServer.RegisterHandleFunc("/register", s.registerClient)
	s.apiServer.RegisterHandleFunc("/unregister", s.unregisterClient)
	s.apiServer.RegisterHandler("/ws", s.wsServer)

	if val := os.Getenv("PERF_PROFILES"); val == "true" {
		runtime.SetMutexProfileFraction(5)
		runtime.SetBlockProfileRate(5)
	}

	s.apiServer.RegisterHandler("/metrics", s.metrics.Handler())
	s.apiServer.RegisterHandler("/debug/pprof/heap", pprof.Handler("heap"))
	s.apiServer.RegisterHandleFunc("/debug/pprof/delta_heap", godeltaprof.Heap)
	s.apiServer.RegisterHandleFunc("/debug/pprof/delta_block", godeltaprof.Block)
	s.apiServer.RegisterHandleFunc("/debug/pprof/delta_mutex", godeltaprof.Mutex)
	s.apiServer.RegisterHandler("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	s.apiServer.RegisterHandler("/debug/pprof/mutex", pprof.Handler("mutex"))
	s.apiServer.RegisterHandleFunc("/debug/pprof/profile", pprof.Profile)
	s.apiServer.RegisterHandleFunc("/debug/pprof/trace", pprof.Trace)

	return s, nil
}

func (s *Service) Start() error {
	defer s.log.Flush()

	if err := s.apiServer.Start(); err != nil {
		return fmt.Errorf("failed to start api server: %w", err)
	}

	if err := s.rtcServer.Start(); err != nil {
		return fmt.Errorf("failed to start rtc server: %w", err)
	}

	go func() {
		for msg := range s.wsServer.ReceiveCh() {
			switch msg.Type {
			case ws.OpenMessage:
				s.log.Debug("connect", mlog.String("connID", msg.ConnID), mlog.String("clientID", msg.ClientID))
				s.metrics.IncWSConnections(msg.ClientID)

				data, err := NewPackedClientMessage(ClientMessageHello, map[string]string{
					"clientID": msg.ClientID,
					"connID":   msg.ConnID,
				})
				if err != nil {
					s.log.Error("failed to pack hello message", mlog.Err(err))
					continue
				}

				if err := s.sendClientMessage(msg.ConnID, msg.ClientID, data); err != nil {
					s.log.Error("failed to send hello message", mlog.Err(err))
					continue
				}
			case ws.CloseMessage:
				s.log.Debug("disconnect", mlog.String("connID", msg.ConnID), mlog.String("clientID", msg.ClientID))
				s.metrics.DecWSConnections(msg.ClientID)
			case ws.TextMessage:
				s.log.Warn("unexpected text message", mlog.String("connID", msg.ConnID), mlog.String("clientID", msg.ClientID))
			case ws.BinaryMessage:
				if err := s.handleClientMsg(msg); err != nil {
					s.log.Error("failed to handle message",
						mlog.Err(err),
						mlog.String("connID", msg.ConnID),
						mlog.String("clientID", msg.ClientID))
					continue
				}
			default:
				s.log.Warn("unexpected ws message", mlog.String("connID", msg.ConnID), mlog.String("clientID", msg.ClientID))
			}
		}
	}()

	go func() {
		for msg := range s.rtcServer.ReceiveCh() {
			if err := s.handleRTCMsg(msg); err != nil {
				s.log.Error("failed to handle message",
					mlog.Err(err),
					mlog.String("groupID", msg.GroupID),
					mlog.String("sessionID", msg.SessionID))
				continue
			}
		}
	}()

	return nil
}

func (s *Service) Stop() error {
	defer s.log.Flush()
	s.log.Info("rtcd: shutting down")

	if err := s.rtcServer.Stop(); err != nil {
		return fmt.Errorf("failed to stop rtc server: %w", err)
	}

	s.wsServer.Close()

	if err := s.apiServer.Stop(); err != nil {
		return fmt.Errorf("failed to stop api server: %w", err)
	}

	if err := s.store.Close(); err != nil {
		return fmt.Errorf("failed to close store: %w", err)
	}

	if err := s.log.Shutdown(); err != nil {
		return fmt.Errorf("failed to shutdown logger: %w", err)
	}

	return nil
}

func (s *Service) handleRTCMsg(msg rtc.Message) error {
	var cm ClientMessage
	switch msg.Type {
	case rtc.SDPMessage, rtc.ICEMessage:
		cm.Type = ClientMessageRTC
	case rtc.VoiceOnMessage, rtc.VoiceOffMessage:
		cm.Type = ClientMessageVAD
	default:
		return fmt.Errorf("unexpected rtc message type: %s", cm.Type)
	}

	s.mut.RLock()
	connID := s.connMap[msg.SessionID]
	s.mut.RUnlock()
	if connID == "" {
		return fmt.Errorf("unexpected empty connID")
	}

	cm.Data = msg

	data, err := cm.Pack()
	if err != nil {
		return fmt.Errorf("failed to pack message: %w", err)
	}

	wsMsg := ws.Message{
		ConnID:   connID,
		ClientID: msg.GroupID,
		Type:     ws.BinaryMessage,
		Data:     data,
	}

	if err := s.wsServer.Send(wsMsg); err != nil {
		return err
	}

	s.metrics.IncWSMessages(msg.GroupID, cm.Type, "out")

	return nil
}

func (s *Service) handleClientMsg(msg ws.Message) error {
	var cm ClientMessage
	if err := cm.Unpack(msg.Data); err != nil {
		return fmt.Errorf("failed to unpack data: %w", err)
	}

	s.metrics.IncWSMessages(msg.ClientID, cm.Type, "in")

	var rtcMsg rtc.Message
	switch cm.Type {
	case ClientMessageJoin:
		data, ok := cm.Data.(map[string]string)
		if !ok {
			return fmt.Errorf("unexpected data type: %T", cm.Data)
		}
		callID := data["callID"]
		if callID == "" {
			return fmt.Errorf("missing callID in client message")
		}
		userID := data["userID"]
		if userID == "" {
			return fmt.Errorf("missing userID in client message")
		}
		sessionID := data["sessionID"]
		if sessionID == "" {
			return fmt.Errorf("missing sessionID in client message")
		}

		closeCb := func() error {
			s.mut.Lock()
			defer s.mut.Unlock()
			delete(s.connMap, sessionID)

			data, err := NewPackedClientMessage(ClientMessageClose, map[string]string{
				"sessionID": sessionID,
			})
			if err != nil {
				return fmt.Errorf("failed to pack close message: %w", err)
			}

			if err := s.sendClientMessage(msg.ConnID, msg.ClientID, data); err != nil {
				return fmt.Errorf("failed to send close message: %w", err)
			}

			return nil
		}

		cfg := rtc.SessionConfig{
			GroupID:   msg.ClientID,
			CallID:    callID,
			UserID:    userID,
			SessionID: sessionID,
		}
		s.log.Debug("join message", mlog.Any("sessionCfg", cfg))
		if err := s.rtcServer.InitSession(cfg, closeCb); err != nil {
			return fmt.Errorf("failed to initialize rtc session: %w", err)
		}

		s.mut.Lock()
		s.connMap[sessionID] = msg.ConnID
		s.mut.Unlock()

		return nil
	case ClientMessageReconnect:
		data, ok := cm.Data.(map[string]string)
		if !ok {
			return fmt.Errorf("unexpected data type: %T", cm.Data)
		}
		sessionID := data["sessionID"]
		if sessionID == "" {
			return fmt.Errorf("missing sessionID in client message")
		}

		s.log.Debug("reconnect message, updating connMap", mlog.String("sessionID", sessionID))
		s.mut.Lock()
		s.connMap[sessionID] = msg.ConnID
		s.mut.Unlock()

		return nil
	case ClientMessageLeave:
		data, ok := cm.Data.(map[string]string)
		if !ok {
			return fmt.Errorf("unexpected data type: %T", cm.Data)
		}
		sessionID := data["sessionID"]
		if sessionID == "" {
			return fmt.Errorf("missing sessionID in client message")
		}

		s.log.Debug("leave message", mlog.String("sessionID", sessionID))
		if err := s.rtcServer.CloseSession(sessionID); err != nil {
			return fmt.Errorf("failed to close session: %w", err)
		}
		return nil
	case ClientMessageRTC:
		var ok bool
		rtcMsg, ok = cm.Data.(rtc.Message)
		if !ok {
			return fmt.Errorf("unexpected data type: %T", cm.Data)
		}
		s.log.Debug("rtc message", mlog.String("sessionID", rtcMsg.SessionID), mlog.Int("type", int(rtcMsg.Type)))
	default:
		return fmt.Errorf("unexpected client message type: %s", cm.Type)
	}

	if err := s.rtcServer.Send(rtcMsg); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

func (s *Service) sendClientMessage(connID, clientID string, data []byte) error {
	wsMsg := ws.Message{
		ConnID:   connID,
		ClientID: clientID,
		Type:     ws.BinaryMessage,
		Data:     data,
	}

	if err := s.wsServer.Send(wsMsg); err != nil {
		return fmt.Errorf("failed to send client message: %w", err)
	}

	return nil
}
