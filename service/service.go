// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"fmt"
	"time"

	"github.com/mattermost/rtcd/service/api"
	"github.com/mattermost/rtcd/service/auth"
	"github.com/mattermost/rtcd/service/rtc"
	"github.com/mattermost/rtcd/service/store"
	"github.com/mattermost/rtcd/service/ws"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

type Service struct {
	cfg       Config
	apiServer *api.Server
	wsServer  *ws.Server
	rtcServer *rtc.Server
	store     store.Store
	auth      *auth.Service
	log       mlog.LoggerIFace
}

func New(cfg Config, log mlog.LoggerIFace) (*Service, error) {
	if err := cfg.IsValid(); err != nil {
		return nil, err
	}

	s := &Service{
		log: log,
		cfg: cfg,
	}

	var err error

	s.store, err = store.New(cfg.Store.DataSource)
	if err != nil {
		return nil, fmt.Errorf("failed to create store: %w", err)
	}
	log.Info("initiated data store", mlog.String("DataSource", cfg.Store.DataSource))

	s.auth, err = auth.NewService(s.store)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth service: %w", err)
	}
	log.Info("initiated auth service")

	s.apiServer, err = api.NewServer(cfg.API, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create api server: %w", err)
	}

	wsConfig := ws.ServerConfig{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    10 * time.Second,
	}
	s.wsServer, err = ws.NewServer(wsConfig, log, ws.WithAuthCb(s.wsAuthHandler))
	if err != nil {
		return nil, fmt.Errorf("failed to create ws server: %w", err)
	}

	s.rtcServer, err = rtc.NewServer(cfg.RTC, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create rtc server: %w", err)
	}

	s.apiServer.RegisterHandleFunc("/version", s.getVersion)
	s.apiServer.RegisterHandleFunc("/register", s.registerClient)
	s.apiServer.RegisterHandler("/ws", s.wsServer)

	return s, nil
}

func (s *Service) Start() error {
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
				s.log.Debug("connect", mlog.String("connID", msg.ConnID))
			case ws.CloseMessage:
				s.log.Debug("disconnect", mlog.String("connID", msg.ConnID))
			case ws.TextMessage:
				s.log.Warn("unexpected text message", mlog.String("connID", msg.ConnID))
			case ws.BinaryMessage:
				var clientMsg ClientMessage
				if err := clientMsg.Unpack(msg.Data); err != nil {
					s.log.Error("failed to unpack message", mlog.Err(err), mlog.String("connID", msg.ConnID))
					continue
				}
			default:
			}
		}
	}()

	return nil
}

func (s *Service) Stop() error {
	if err := s.rtcServer.Stop(); err != nil {
		return fmt.Errorf("failed to stop rtc server: %w", err)
	}

	if err := s.apiServer.Stop(); err != nil {
		return fmt.Errorf("failed to stop api server: %w", err)
	}

	s.wsServer.Close()

	if err := s.store.Close(); err != nil {
		return fmt.Errorf("failed to close store: %w", err)
	}

	return nil
}
