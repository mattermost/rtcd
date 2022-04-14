// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

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

	service, err := service.New(cfg)
	if err != nil {
		log.Fatalf("rtcd: failed to create service: %s", err.Error())
	}

	if err := service.Start(); err != nil {
		log.Fatalf("rtcd: failed to start service: %s", err.Error())
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	if err := service.Stop(); err != nil {
		log.Fatalf("rtcd: failed to stop service: %s", err.Error())
	}
}
