// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mattermost/rtcd/service/random"

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
	th := setupTestHelper(t)

	connectCh := make(chan struct{})
	th.userClient.On(WSConnectEvent, func() error {
		close(connectCh)
		return nil
	})

	disconnectCh := make(chan struct{})
	th.userClient.On(WSDisconnectEvent, func() error {
		close(disconnectCh)
		return nil
	})

	closeCh := make(chan struct{})
	th.userClient.On(CloseEvent, func() error {
		close(closeCh)
		return nil
	})

	err := th.userClient.Connect(random.NewID())
	require.NoError(t, err)

	select {
	case <-connectCh:
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for connect event")
	}

	err = th.userClient.Close()
	require.NoError(t, err)

	select {
	case <-disconnectCh:
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for disconnect event")
	}

	select {
	case <-closeCh:
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for close event")
	}
}

func TestClientConsistency(t *testing.T) {
	th := setupTestHelper(t)

	t.Run("double connect", func(t *testing.T) {
		err := th.userClient.Connect(random.NewID())
		require.NoError(t, err)

		require.Equal(t, clientStateInit, th.userClient.state)

		err = th.userClient.Connect(random.NewID())
		require.EqualError(t, err, "ws client is already initialized")
	})

	t.Run("double close", func(t *testing.T) {
		err := th.userClient.Close()
		require.NoError(t, err)

		require.Equal(t, clientStateClosed, th.userClient.state)

		err = th.userClient.Close()
		require.EqualError(t, err, "client is not initialized")
	})

	t.Run("reuse client", func(t *testing.T) {
		err := th.userClient.Connect(random.NewID())
		require.EqualError(t, err, "ws client is already initialized")
	})
}

func TestClientConcurrency(t *testing.T) {
	th := setupTestHelper(t)

	connectCh := make(chan struct{})
	closeCh := make(chan struct{})

	var wg sync.WaitGroup
	n := 10
	wg.Add(n * 3)

	var connectErrors int32
	var closeErrors int32

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()

			th.userClient.On(WSConnectEvent, func() error {
				close(connectCh)
				return nil
			})

			th.userClient.On(CloseEvent, func() error {
				close(closeCh)
				return nil
			})
		}()
	}

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()

			err := th.userClient.Connect(random.NewID())
			if err != nil {
				atomic.AddInt32(&connectErrors, 1)
			}
		}()
	}

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()

			err := th.userClient.Close()
			if err != nil {
				atomic.AddInt32(&closeErrors, 1)
			}
		}()
	}

	wg.Wait()

	require.GreaterOrEqual(t, connectErrors, int32(n-1))
	require.GreaterOrEqual(t, closeErrors, int32(n-1))
}
