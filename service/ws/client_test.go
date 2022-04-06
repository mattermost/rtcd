// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/mattermost/rtcd/service/random"

	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	server, addr, shutdown := setupServer(t)
	defer shutdown()

	t.Run("invalid config", func(t *testing.T) {
		c, err := NewClient(ClientConfig{})
		require.Error(t, err)
		require.Nil(t, c)
	})

	t.Run("valid config", func(t *testing.T) {
		_, port, err := net.SplitHostPort(addr)
		require.NoError(t, err)
		u := url.URL{Scheme: "ws", Host: "localhost:" + port, Path: "/ws"}

		cfg := ClientConfig{
			URL: u.String(),
		}
		c, err := NewClient(cfg)
		require.NoError(t, err)
		require.NotNil(t, c)

		msg, ok := <-server.ReceiveCh()
		require.True(t, ok)
		require.NotEmpty(t, msg)
		require.NotEmpty(t, msg.ConnID)
		require.Equal(t, OpenMessage, msg.Type)

		err = c.Close()
		require.NoError(t, err)

		msg, ok = <-server.ReceiveCh()
		require.True(t, ok)
		require.NotEmpty(t, msg)
		require.NotEmpty(t, msg.ConnID)
		require.Equal(t, CloseMessage, msg.Type)
	})
}

func TestNewClientWithAuth(t *testing.T) {
	authToken := random.NewID()
	clientID := random.NewID()

	authCb := func(w http.ResponseWriter, r *http.Request) (string, error) {
		authHeader := r.Header.Get("Authorization")
		require.NotEmpty(t, authHeader)
		if fields := strings.Fields(authHeader); len(fields) > 1 && fields[1] == authToken {
			return clientID, nil
		}

		return "", fmt.Errorf("auth check failed")
	}

	server, addr, shutdown := setupServer(t, WithAuthCb(authCb))
	defer shutdown()

	t.Run("auth failure", func(t *testing.T) {
		_, port, err := net.SplitHostPort(addr)
		require.NoError(t, err)
		u := url.URL{Scheme: "ws", Host: "localhost:" + port, Path: "/ws"}

		cfg := ClientConfig{
			URL: u.String(),
		}
		c, err := NewClient(cfg)
		require.Error(t, err)
		require.Nil(t, c)
	})

	t.Run("auth success", func(t *testing.T) {
		_, port, err := net.SplitHostPort(addr)
		require.NoError(t, err)
		u := url.URL{Scheme: "ws", Host: "localhost:" + port, Path: "/ws"}
		cfg := ClientConfig{
			URL:       u.String(),
			AuthToken: authToken,
		}
		c, err := NewClient(cfg)
		require.NoError(t, err)
		require.NotNil(t, c)

		msg, ok := <-server.ReceiveCh()
		require.True(t, ok)
		require.NotEmpty(t, msg)
		require.NotEmpty(t, msg.ConnID)
		require.Equal(t, OpenMessage, msg.Type)

		server.mut.RLock()
		require.Equal(t, clientID, server.conns[msg.ConnID].clientID)
		server.mut.RUnlock()

		require.NoError(t, c.Close())
	})
}

func TestClientPing(t *testing.T) {
	server, addr, shutdown := setupServer(t)
	defer shutdown()

	withCustomPingHandler := func(c *Client) error {
		c.conn.ws.SetPingHandler(func(_ string) error {
			return nil
		})
		return nil
	}

	_, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	u := url.URL{Scheme: "ws", Host: "localhost:" + port, Path: "/ws"}
	cfg := ClientConfig{
		URL: u.String(),
	}
	c, err := NewClient(cfg, withCustomPingHandler)
	require.NoError(t, err)
	require.NotNil(t, c)
	defer c.Close()

	msg, ok := <-server.ReceiveCh()
	require.True(t, ok)
	require.Equal(t, OpenMessage, msg.Type)

	// server should disconnect due to missing ping
	err = <-c.ErrorCh()
	require.NotNil(t, err)
	msg = <-c.ReceiveCh()
	require.Empty(t, msg)
	require.Empty(t, <-c.conn.closeCh)

	msg, ok = <-server.ReceiveCh()
	require.True(t, ok)
	require.Equal(t, CloseMessage, msg.Type)
}
