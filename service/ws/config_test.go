// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestServerConfigIsValid(t *testing.T) {
	t.Run("empty struct", func(t *testing.T) {
		var cfg ServerConfig
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid ReadBufferSize value: should be greater than zero", err.Error())
	})

	t.Run("invalid WriteBufferSize", func(t *testing.T) {
		var cfg ServerConfig
		cfg.ReadBufferSize = 1024
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid WriteBufferSize value: should be greater than zero", err.Error())
	})

	t.Run("invalid PingInterval", func(t *testing.T) {
		var cfg ServerConfig
		cfg.ReadBufferSize = 1024
		cfg.WriteBufferSize = 1024
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid PingInterval value: should be at least 1 second", err.Error())
	})

	t.Run("valid", func(t *testing.T) {
		var cfg ServerConfig
		cfg.ReadBufferSize = 1024
		cfg.WriteBufferSize = 1024
		cfg.PingInterval = 1 * time.Second
		err := cfg.IsValid()
		require.NoError(t, err)
	})
}

func TestClientConfigIsValid(t *testing.T) {
	t.Run("empty struct", func(t *testing.T) {
		var cfg ClientConfig
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid URL value: should not be empty", err.Error())
	})

	t.Run("invalid URL", func(t *testing.T) {
		var cfg ClientConfig
		cfg.URL = "http://invalid"
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, `invalid URL value: should start with "ws://" or "wss://"`, err.Error())
	})

	t.Run("invalid ConnID", func(t *testing.T) {
		var cfg ClientConfig
		cfg.URL = "wss://localhost:8045"
		cfg.ConnID = "invalid"
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid ConnID value: should be 26 characters long", err.Error())
	})

	t.Run("empty connID", func(t *testing.T) {
		var cfg ClientConfig
		cfg.URL = "wss://localhost:8045"
		cfg.ConnID = ""
		err := cfg.IsValid()
		require.NoError(t, err)
	})

	t.Run("valid", func(t *testing.T) {
		var cfg ClientConfig
		cfg.URL = "wss://localhost:8045"
		cfg.ConnID = newID()
		err := cfg.IsValid()
		require.NoError(t, err)
	})
}
