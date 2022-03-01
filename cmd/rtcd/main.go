// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
	"github.com/mattermost/rtcd/logger"
	"github.com/mattermost/rtcd/service"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "config/config.toml", "Path to the configuration file for the rtcd service.")
	flag.Parse()

	cfg, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("rtcd: failed to load config: %s", err.Error())
	}

	if err := cfg.IsValid(); err != nil {
		log.Fatalf("rtcd: failed to validate config: %s", err.Error())
	}

	logger, err := logger.New(cfg.Logger)
	if err != nil {
		log.Fatalf("rtcd: failed to init logger: %s", err.Error())
	}
	defer func() {
		if err := logger.Shutdown(); err != nil {
			log.Printf("rtcd: failed to shutdown logger: %s", err.Error())
		}
	}()

	logger.Info("rtcd: starting up")

	service, err := service.New(cfg.Service, logger)
	if err != nil {
		logger.Error("rtcd: failed to create service", mlog.Err(err))
		return
	}

	if err := service.Start(); err != nil {
		logger.Error("rtcd: failed to start service", mlog.Err(err))
		return
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	logger.Info("rtcd: shutting down")

	if err := service.Stop(); err != nil {
		logger.Error("rtcd: failed to stop service", mlog.Err(err))
		return
	}
}
