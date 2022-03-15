// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"fmt"
	"time"

	"github.com/mattermost/rtcd/service/api"
	"github.com/mattermost/rtcd/service/ws"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

type Service struct {
	cfg       Config
	apiServer *api.Server
	wsServer  *ws.Server
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

	return nil
}
