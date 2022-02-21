// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mattermost/rtcd/logger"
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

	logger, err := logger.New(cfg.LogSettings)
	if err != nil {
		log.Fatalf("rtcd: failed to init logger: %s", err.Error())
	}
	defer logger.Shutdown()

	logger.Info("rtcd: starting up")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	logger.Info("rtcd: shutting down")
}
