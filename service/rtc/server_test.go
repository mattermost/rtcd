// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"net"
	"testing"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
	"github.com/stretchr/testify/require"
)

func setupServer(t *testing.T) (*Server, func()) {
	t.Helper()

	log, err := mlog.NewLogger()
	require.NoError(t, err)

	cfg := ServerConfig{
		ICEPortUDP: 30433,
	}

	s, err := NewServer(cfg, log)
	require.NoError(t, err)

	return s, func() {
		err := s.Stop()
		require.NoError(t, err)
		err = log.Shutdown()
		require.NoError(t, err)
	}
}

func TestNewServer(t *testing.T) {
	log, err := mlog.NewLogger()
	require.NoError(t, err)
	defer func() {
		err := log.Shutdown()
		require.NoError(t, err)
	}()

	t.Run("invalid config", func(t *testing.T) {
		s, err := NewServer(ServerConfig{}, log)
		require.Error(t, err)
		require.Nil(t, s)
	})

	t.Run("missing logger", func(t *testing.T) {
		cfg := ServerConfig{
			ICEPortUDP: 30433,
		}
		s, err := NewServer(cfg, nil)
		require.Error(t, err)
		require.Nil(t, s)
	})

	t.Run("valid", func(t *testing.T) {
		cfg := ServerConfig{
			ICEPortUDP: 30433,
		}
		s, err := NewServer(cfg, log)
		require.NoError(t, err)
		require.NotNil(t, s)
	})
}

func TestStartServer(t *testing.T) {
	log, err := mlog.NewLogger()
	require.NoError(t, err)
	defer func() {
		err := log.Shutdown()
		require.NoError(t, err)
	}()

	cfg := ServerConfig{
		ICEPortUDP: 30433,
	}

	t.Run("port unavailable", func(t *testing.T) {
		s, err := NewServer(cfg, log)
		require.NoError(t, err)
		require.NotNil(t, s)

		udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{
			Port: cfg.ICEPortUDP,
		})
		require.NoError(t, err)
		defer udpConn.Close()

		err = s.Start()
		defer func() {
			err := s.Stop()
			require.NoError(t, err)
		}()
		require.Error(t, err)
		require.Equal(t, "failed to listen on udp: listen udp4 :30433: bind: address already in use", err.Error())
	})

	t.Run("started", func(t *testing.T) {
		s, err := NewServer(cfg, log)
		require.NoError(t, err)
		require.NotNil(t, s)

		err = s.Start()
		defer func() {
			err := s.Stop()
			require.NoError(t, err)
		}()
		require.NoError(t, err)
	})
}
