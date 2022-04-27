// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"errors"
	"sync"
	"testing"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
)

func TestAddSession(t *testing.T) {
	server, shutdown := setupServer(t)
	defer shutdown()

	t.Run("invalid config", func(t *testing.T) {
		us, err := server.addSession(SessionConfig{}, nil, nil)
		require.Error(t, err)
		require.Nil(t, us)
	})

	t.Run("nil peerConn", func(t *testing.T) {
		cfg := SessionConfig{
			GroupID:   "test",
			CallID:    "test",
			UserID:    "test",
			SessionID: "test",
		}
		us, err := server.addSession(cfg, nil, nil)
		require.Error(t, err)
		require.Equal(t, "peerConn should not be nil", err.Error())
		require.Nil(t, us)
	})

	t.Run("success", func(t *testing.T) {
		cfg := SessionConfig{
			GroupID:   "test",
			CallID:    "test",
			UserID:    "test",
			SessionID: "test",
		}

		peerConn, err := webrtc.NewPeerConnection(webrtc.Configuration{})
		require.NoError(t, err)

		us, err := server.addSession(cfg, peerConn, nil)
		require.NoError(t, err)
		require.NotNil(t, us)

		err = server.CloseSession(cfg.SessionID)
		require.NoError(t, err)
	})

	t.Run("closeCb", func(t *testing.T) {
		cfg := SessionConfig{
			GroupID:   "test",
			CallID:    "test",
			UserID:    "test",
			SessionID: "test",
		}

		peerConn, err := webrtc.NewPeerConnection(webrtc.Configuration{})
		require.NoError(t, err)

		cbError := errors.New("closeCb failed")
		closeCbError := func() error {
			return cbError
		}

		var cbCalled bool
		closeCbSuccess := func() error {
			cbCalled = true
			return nil
		}

		us, err := server.addSession(cfg, peerConn, closeCbError)
		require.NoError(t, err)
		require.NotNil(t, us)

		err = server.CloseSession(cfg.SessionID)
		require.Error(t, err)
		require.Equal(t, cbError, err)

		us, err = server.addSession(cfg, peerConn, closeCbSuccess)
		require.NoError(t, err)
		require.NotNil(t, us)

		err = server.CloseSession(cfg.SessionID)
		require.NoError(t, err)
		require.True(t, cbCalled)
	})
}

func TestCloseSessionConcurrent(t *testing.T) {
	server, shutdown := setupServer(t)
	defer shutdown()

	cfg := SessionConfig{
		GroupID:   "test",
		CallID:    "test",
		UserID:    "test",
		SessionID: "test",
	}

	peerConn, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)

	us, err := server.addSession(cfg, peerConn, nil)
	require.NoError(t, err)
	require.NotNil(t, us)

	var wg sync.WaitGroup
	n := 20
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			err := server.CloseSession(cfg.SessionID)
			require.Nil(t, err)
		}()
	}
	wg.Wait()
}
