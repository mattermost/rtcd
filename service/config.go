// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"github.com/mattermost/rtcd/logger"
)

type Config struct {
	LogSettings logger.Settings `toml:"logging"`
}

func (c Config) IsValid() error {
	if err := c.LogSettings.IsValid(); err != nil {
		return err
	}
	return nil
}
