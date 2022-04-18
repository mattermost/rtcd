// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

type Server struct {
	cfg      Config
	listener net.Listener
	srv      *http.Server
	mux      *http.ServeMux
	log      mlog.LoggerIFace
}

func NewServer(cfg Config, log mlog.LoggerIFace) (*Server, error) {
	if err := cfg.IsValid(); err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	s := &Server{
		srv: &http.Server{
			Addr:         cfg.ListenAddress,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  30 * time.Second,
			TLSConfig: &tls.Config{
				MinVersion:               tls.VersionTLS12,
				PreferServerCipherSuites: true,
				CurvePreferences: []tls.CurveID{
					tls.CurveP256,
				},
			},
			Handler: mux,
		},
		log: log,
		cfg: cfg,
		mux: mux,
	}
	return s, nil
}

func (s *Server) Start() error {
	var err error
	s.listener, err = net.Listen("tcp", s.cfg.ListenAddress)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	s.log.Info("api: server is listening on " + s.listener.Addr().String())

	go func() {
		var err error
		if s.cfg.TLS.Enable && s.cfg.TLS.CertFile != "" && s.cfg.TLS.CertKey != "" {
			s.log.Debug("api: serving with tls")
			err = s.srv.ServeTLS(s.listener, s.cfg.TLS.CertFile, s.cfg.TLS.CertKey)
		} else {
			s.log.Debug("api: serving plaintext")
			err = s.srv.Serve(s.listener)
		}
		if err != nil && err != http.ErrServerClosed {
			s.log.Critical("error starting HTTP server", mlog.Err(err))
		}
	}()

	return nil
}

func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}
	s.log.Info("api: server was shutdown")
	return nil
}

func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}
