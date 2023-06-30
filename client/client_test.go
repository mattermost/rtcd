// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"context"
	"testing"
	"time"

	"github.com/mattermost/rtcd/service/random"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/stretchr/testify/require"
)

func TestConfigParse(t *testing.T) {
	t.Run("empty struct", func(t *testing.T) {
		var cfg Config
		err := cfg.Parse()
		require.Error(t, err)
		require.Equal(t, "invalid SiteURL value: should not be empty", err.Error())
	})

	t.Run("invalid SiteURL scheme", func(t *testing.T) {
		cfg := Config{
			SiteURL: "ws://host",
		}
		err := cfg.Parse()
		require.Error(t, err)
		require.Equal(t, "invalid SiteURL scheme \"ws\"", err.Error())
	})

	t.Run("spaces in SiteURL", func(t *testing.T) {
		cfg := Config{
			SiteURL:   " http://host  ",
			AuthToken: random.NewID(),
		}
		err := cfg.Parse()
		require.NoError(t, err)
	})

	t.Run("slashes in SiteURL", func(t *testing.T) {
		cfg := Config{
			SiteURL:   "http://host/subpath////",
			AuthToken: random.NewID(),
		}
		err := cfg.Parse()
		require.NoError(t, err)
		require.Equal(t, "http://host/subpath", cfg.SiteURL)
	})

	t.Run("empty AuthToken", func(t *testing.T) {
		cfg := Config{
			SiteURL: "http://mm-url",
		}
		err := cfg.Parse()
		require.Error(t, err)
		require.Equal(t, "invalid AuthToken value: should not be empty", err.Error())
	})

	t.Run("wsURL", func(t *testing.T) {
		cfg := Config{
			SiteURL:   "https://mm-url:8065/",
			AuthToken: random.NewID(),
		}
		err := cfg.Parse()
		require.NoError(t, err)
		require.Equal(t, "wss://mm-url:8065/api/v4/websocket", cfg.wsURL)

		cfg.SiteURL = "http://mm-url//"
		err = cfg.Parse()
		require.NoError(t, err)
		require.Equal(t, "ws://mm-url/api/v4/websocket", cfg.wsURL)

		cfg.SiteURL = "http://mm-url/subpath/"
		err = cfg.Parse()
		require.NoError(t, err)
		require.Equal(t, "ws://mm-url/subpath/api/v4/websocket", cfg.wsURL)
	})

	t.Run("valid", func(t *testing.T) {
		cfg := Config{
			SiteURL:   "https://mm-url:8065/",
			AuthToken: random.NewID(),
		}
		err := cfg.Parse()
		require.NoError(t, err)
	})
}

func TestClientConnect(t *testing.T) {
	// Setup

	siteURL := "http://localhost:8065"
	username := "calls-user0"
	password := "U$er-sample1"

	apiClient := model.NewAPIv4Client(siteURL)

	// login (or create) user
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _, err := apiClient.Login(ctx, username, password)
	require.Nil(t, err)

	cfg := Config{
		SiteURL:   siteURL,
		AuthToken: apiClient.AuthToken,
	}

	c, err := New(cfg)
	require.NoError(t, err)
	require.NotNil(t, c)

	connectCh := make(chan struct{})
	c.On(ConnectEvent, func() error {
		close(connectCh)
		return nil
	})

	err = c.Connect(random.NewID())
	require.NoError(t, err)

	<-connectCh

	err = c.Close()
	require.NoError(t, err)
}
