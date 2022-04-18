// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecurityConfigIsValid(t *testing.T) {
	t.Run("empty struct", func(t *testing.T) {
		var cfg SecurityConfig
		err := cfg.IsValid()
		require.NoError(t, err)
	})

	t.Run("empty key", func(t *testing.T) {
		var cfg SecurityConfig
		cfg.EnableAdmin = true
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid AdminSecretKey value: should not be empty", err.Error())
	})

	t.Run("valid", func(t *testing.T) {
		var cfg SecurityConfig
		cfg.EnableAdmin = true
		cfg.AdminSecretKey = "secret_key"
		err := cfg.IsValid()
		require.NoError(t, err)
	})
}

func TestStoreConfigIsValid(t *testing.T) {
	t.Run("empty struct", func(t *testing.T) {
		var cfg StoreConfig
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid DataSource value: should not be empty", err.Error())
	})

	t.Run("valid", func(t *testing.T) {
		var cfg StoreConfig
		cfg.DataSource = "/tmp/rtcd_db"
		err := cfg.IsValid()
		require.NoError(t, err)
	})
}

func TestClientConfigParse(t *testing.T) {
	t.Run("empty struct", func(t *testing.T) {
		var cfg ClientConfig
		err := cfg.Parse()
		require.Error(t, err)
		require.Equal(t, "invalid URL value: should not be empty", err.Error())
	})

	t.Run("invalid URL", func(t *testing.T) {
		var cfg ClientConfig
		cfg.URL = "//sd"
		err := cfg.Parse()
		require.Error(t, err)
		require.Equal(t, `invalid url scheme: "" is not valid`, err.Error())
	})

	t.Run("missing host", func(t *testing.T) {
		var cfg ClientConfig
		cfg.URL = "https:///test"
		err := cfg.Parse()
		require.Error(t, err)
		require.Equal(t, "invalid url host: should not be empty", err.Error())
	})

	t.Run("valid http", func(t *testing.T) {
		var cfg ClientConfig
		cfg.URL = "http://rtcd.example.com"
		err := cfg.Parse()
		require.NoError(t, err)
		require.Equal(t, "ws://rtcd.example.com/ws", cfg.wsURL)
	})

	t.Run("valid https", func(t *testing.T) {
		var cfg ClientConfig
		cfg.URL = "https://rtcd.example.com"
		err := cfg.Parse()
		require.NoError(t, err)
		require.Equal(t, "wss://rtcd.example.com/ws", cfg.wsURL)
	})
}
