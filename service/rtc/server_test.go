// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/mattermost/rtcd/service/perf"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
	"github.com/stretchr/testify/require"
)

func setupServer(t *testing.T) (*Server, func()) {
	t.Helper()

	log, err := mlog.NewLogger()
	require.NoError(t, err)

	metrics := perf.NewMetrics("rtcd", nil)
	require.NotNil(t, metrics)

	cfg := ServerConfig{
		ICEPortUDP: 30433,
	}

	s, err := NewServer(cfg, log, metrics)
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

	metrics := perf.NewMetrics("rtcd", nil)
	require.NotNil(t, metrics)

	t.Run("invalid config", func(t *testing.T) {
		s, err := NewServer(ServerConfig{}, log, metrics)
		require.Error(t, err)
		require.Nil(t, s)
	})

	t.Run("missing logger", func(t *testing.T) {
		cfg := ServerConfig{
			ICEPortUDP: 30433,
		}
		s, err := NewServer(cfg, nil, metrics)
		require.Error(t, err)
		require.Nil(t, s)
	})

	t.Run("missing metrics", func(t *testing.T) {
		cfg := ServerConfig{
			ICEPortUDP: 30433,
		}
		s, err := NewServer(cfg, log, nil)
		require.Error(t, err)
		require.Nil(t, s)
	})

	t.Run("valid", func(t *testing.T) {
		cfg := ServerConfig{
			ICEPortUDP: 30433,
		}
		s, err := NewServer(cfg, log, metrics)
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

	metrics := perf.NewMetrics("rtcd", nil)
	require.NotNil(t, metrics)

	cfg := ServerConfig{
		ICEPortUDP: 30433,
	}

	t.Run("port unavailable", func(t *testing.T) {
		s, err := NewServer(cfg, log, metrics)
		require.NoError(t, err)
		require.NotNil(t, s)

		udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{
			Port: cfg.ICEPortUDP,
		})
		require.NoError(t, err)
		defer udpConn.Close()

		ips, err := getSystemIPs(log)
		require.NoError(t, err)
		require.NotEmpty(t, ips)

		err = s.Start()
		defer func() {
			err := s.Stop()
			require.NoError(t, err)
		}()
		require.Error(t, err)
		require.Equal(t, fmt.Sprintf("failed to create UDP connections: failed to listen on udp: listen udp4 %s:%d: bind: address already in use",
			ips[0], cfg.ICEPortUDP), err.Error())
	})

	t.Run("started", func(t *testing.T) {
		s, err := NewServer(cfg, log, metrics)
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

func TestDraining(t *testing.T) {
	log, err := mlog.NewLogger()
	require.NoError(t, err)
	defer func() {
		err := log.Shutdown()
		require.NoError(t, err)
	}()

	cfg := ServerConfig{
		ICEPortUDP: 30433,
	}

	metrics := perf.NewMetrics("rtcd", nil)
	require.NotNil(t, metrics)

	t.Run("no session", func(t *testing.T) {
		s, err := NewServer(cfg, log, metrics)
		require.NoError(t, err)
		require.NotNil(t, s)

		err = s.Start()
		require.NoError(t, err)

		err = s.Stop()
		require.NoError(t, err)
	})

	t.Run("sessions ongoing", func(t *testing.T) {
		s, err := NewServer(cfg, log, metrics)
		require.NoError(t, err)
		require.NotNil(t, s)

		err = s.Start()
		require.NoError(t, err)

		s.mut.Lock()
		s.sessions["test"] = SessionConfig{}
		s.sessions["test1"] = SessionConfig{}
		s.mut.Unlock()

		go func() {
			time.Sleep(time.Second * 2)
			_ = s.CloseSession("test")
			_ = s.CloseSession("test1")
		}()

		beforeStop := time.Now()

		err = s.Stop()
		require.NoError(t, err)

		require.True(t, time.Since(beforeStop) > time.Second)
	})
}
