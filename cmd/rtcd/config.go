// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"fmt"

	"github.com/mattermost/rtcd/logger"
	"github.com/mattermost/rtcd/service"

	"github.com/BurntSushi/toml"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Service service.Config
	Logger  logger.Config
}

func (c Config) IsValid() error {
	if err := c.Service.IsValid(); err != nil {
		return err
	}
	if err := c.Logger.IsValid(); err != nil {
		return err
	}
	return nil
}

// loadConfig reads the config file and returns a new Config,
// This method overrides values in the file if there is any environment
// variables corresponding to a specific setting.
func loadConfig(path string) (Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, fmt.Errorf("failed to decode config file: %w", err)
	}
	if err := envconfig.Process("rtcd", &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
