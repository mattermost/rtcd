// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"fmt"

	"github.com/mattermost/rtcd/service/api"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

type Service struct {
	cfg       Config
	apiServer *api.Server
	log       *mlog.Logger
}

func New(cfg Config, log *mlog.Logger) (*Service, error) {
	if err := cfg.API.IsValid(); err != nil {
		return nil, err
	}

	apiServer, err := api.NewServer(cfg.API, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create api server: %w", err)
	}

	s := &Service{
		apiServer: apiServer,
		log:       log,
		cfg:       cfg,
	}

	apiServer.RegisterHandler("/version", s.getVersion)

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
