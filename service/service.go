// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"fmt"
	"time"

	"github.com/mattermost/rtcd/service/api"
	"github.com/mattermost/rtcd/service/auth"
	"github.com/mattermost/rtcd/service/store"
	"github.com/mattermost/rtcd/service/ws"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

type Service struct {
	cfg       Config
	apiServer *api.Server
	wsServer  *ws.Server
	store     store.Store
	auth      *auth.Service
	log       *mlog.Logger
}

func New(cfg Config, log *mlog.Logger) (*Service, error) {
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
	s.wsServer, err = ws.NewServer(wsConfig, log, ws.WithUpgradeCb(s.wsAuthHandler))
	if err != nil {
		return nil, fmt.Errorf("failed to create ws server: %w", err)
	}

	s.apiServer.RegisterHandleFunc("/version", s.getVersion)
	s.apiServer.RegisterHandleFunc("/register", s.registerClient)
	s.apiServer.RegisterHandler("/ws", s.wsServer)

	return s, nil
}

func (s *Service) Start() error {
	if err := s.apiServer.Start(); err != nil {
		return fmt.Errorf("failed to start API server: %w", err)
	}

	return nil
}

func (s *Service) Stop() error {
	if err := s.apiServer.Stop(); err != nil {
		return fmt.Errorf("failed to stop API server: %w", err)
	}

	if err := s.store.Close(); err != nil {
		return fmt.Errorf("failed to close store: %w", err)
	}

	return nil
}
