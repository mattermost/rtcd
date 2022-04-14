// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/mattermost/rtcd/service"

	"github.com/BurntSushi/toml"
	"github.com/kelseyhightower/envconfig"
)

// loadConfig reads the config file and returns a new Config,
// This method overrides values in the file if there is any environment
// variables corresponding to a specific setting.
func loadConfig(path string) (service.Config, error) {
	var cfg service.Config
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		log.Printf("config file not found at %s, using defaults", path)
		cfg.SetDefaults()
	} else if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, fmt.Errorf("failed to decode config file: %w", err)
	}
	if err := envconfig.Process("rtcd", &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
