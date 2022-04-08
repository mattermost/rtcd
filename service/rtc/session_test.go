// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"testing"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
)

func TestAddSession(t *testing.T) {
	server, shutdown := setupServer(t)
	defer shutdown()

	t.Run("invalid config", func(t *testing.T) {
		us, err := server.addSession(SessionConfig{}, nil)
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
		us, err := server.addSession(cfg, nil)
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

		us, err := server.addSession(cfg, peerConn)
		require.NoError(t, err)
		require.NotNil(t, us)

		err = server.CloseSession(cfg)
		require.NoError(t, err)
	})
}
