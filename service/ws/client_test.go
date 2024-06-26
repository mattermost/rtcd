// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

import (
	"context"
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
			URL:      u.String(),
			AuthType: BasicClientAuthType,
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

	t.Run("custom dialing function", func(t *testing.T) {
		_, port, err := net.SplitHostPort(addr)
		require.NoError(t, err)
		u := url.URL{Scheme: "ws", Host: "localhost:" + port, Path: "/ws"}

		var called bool
		dialFn := func(ctx context.Context, network, addr string) (net.Conn, error) {
			called = true
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		}

		dialErrorFn := func(_ context.Context, _, _ string) (net.Conn, error) {
			return nil, fmt.Errorf("dial error test")
		}

		cfg := ClientConfig{
			URL:      u.String(),
			AuthType: BasicClientAuthType,
		}
		c, err := NewClient(cfg, WithDialFunc(dialFn))
		require.NoError(t, err)
		require.NotNil(t, c)
		require.True(t, called)

		err = c.Close()
		require.NoError(t, err)

		c, err = NewClient(cfg, WithDialFunc(dialErrorFn))
		require.Error(t, err)
		require.Nil(t, c)
		require.Equal(t, "failed to dial: dial error test", err.Error())
	})
}

func TestNewClientWithAuth(t *testing.T) {
	authToken := random.NewID()
	clientID := random.NewID()

	authCb := func(_ http.ResponseWriter, r *http.Request) (string, int, error) {
		authHeader := r.Header.Get("Authorization")
		require.NotEmpty(t, authHeader)
		if fields := strings.Fields(authHeader); len(fields) > 1 && fields[1] == authToken {
			return clientID, 200, nil
		}

		return "", 401, fmt.Errorf("auth check failed")
	}

	server, addr, shutdown := setupServer(t, WithAuthCb(authCb))
	defer shutdown()

	t.Run("auth failure", func(t *testing.T) {
		_, port, err := net.SplitHostPort(addr)
		require.NoError(t, err)
		u := url.URL{Scheme: "ws", Host: "localhost:" + port, Path: "/ws"}

		cfg := ClientConfig{
			URL:      u.String(),
			AuthType: BasicClientAuthType,
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
			AuthType:  BasicClientAuthType,
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
		c.pingHandlerFn = func(_ string) error {
			return nil
		}
		return nil
	}

	_, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	u := url.URL{Scheme: "ws", Host: "localhost:" + port, Path: "/ws"}
	cfg := ClientConfig{
		URL:      u.String(),
		AuthType: BasicClientAuthType,
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

func TestMultipleClose(t *testing.T) {
	server, addr, shutdown := setupServer(t)
	defer shutdown()

	_, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	u := url.URL{Scheme: "ws", Host: "localhost:" + port, Path: "/ws"}
	cfg := ClientConfig{
		URL:      u.String(),
		AuthType: BasicClientAuthType,
	}
	c, err := NewClient(cfg)
	require.NoError(t, err)
	require.NotNil(t, c)

	msg, ok := <-server.ReceiveCh()
	require.True(t, ok)
	require.Equal(t, OpenMessage, msg.Type)

	err = c.Close()
	require.NoError(t, err)

	err = c.Close()
	require.Error(t, err)

	err = c.Close()
	require.Error(t, err)
}
