// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package main

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/mattermost/rtcd/service"

	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	var defaultCfg service.Config
	defaultCfg.SetDefaults()

	t.Run("non existant file", func(t *testing.T) {
		cfg, err := loadConfig("")
		require.NoError(t, err)
		require.NotEmpty(t, cfg)
		require.Equal(t, defaultCfg, cfg)
	})

	t.Run("empty file", func(t *testing.T) {
		file, err := ioutil.TempFile("", "config.toml")
		require.NoError(t, err)
		require.NotNil(t, file)
		defer file.Close()
		defer os.Remove(file.Name())

		cfg, err := loadConfig(file.Name())
		require.NoError(t, err)
		require.Equal(t, defaultCfg, cfg)
	})

	t.Run("invalid config", func(t *testing.T) {
		file, err := ioutil.TempFile("", "config.toml")
		require.NoError(t, err)
		require.NotNil(t, file)
		defer file.Close()
		defer os.Remove(file.Name())
		configData := `[invalid]`
		_, err = file.Write([]byte(configData))
		require.NoError(t, err)

		cfg, err := loadConfig(file.Name())
		require.NoError(t, err)
		require.Equal(t, defaultCfg, cfg)
	})

	t.Run("valid config", func(t *testing.T) {
		cfg, err := loadConfig("../../config/config.sample.toml")
		require.NoError(t, err)
		require.NotEmpty(t, cfg)
	})

	t.Run("env override", func(t *testing.T) {
		cfg, err := loadConfig("../../config/config.sample.toml")
		require.NoError(t, err)
		require.NotEmpty(t, cfg)
		require.Equal(t, "DEBUG", cfg.Logger.FileLevel)

		os.Setenv("RTCD_LOGGER_FILELEVEL", "ERROR")
		defer os.Unsetenv("RTCD_LOGGER_FILELEVEL")
		cfg, err = loadConfig("../../config/config.sample.toml")
		require.NoError(t, err)
		require.NotEmpty(t, cfg)
		require.Equal(t, "ERROR", cfg.Logger.FileLevel)
	})
}
