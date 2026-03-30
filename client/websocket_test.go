// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientWSDisconnect(t *testing.T) {
	th := setupTestHelper(t, "")

	connectCh := make(chan struct{}, 2)
	err := th.userClient.On(WSConnectEvent, func(_ any) error {
		connectCh <- struct{}{}
		return nil
	})
	require.NoError(t, err)

	disconnectCh := make(chan struct{})
	err = th.userClient.On(WSDisconnectEvent, func(_ any) error {
		close(disconnectCh)
		return nil
	})
	require.NoError(t, err)

	err = th.userClient.Connect()
	require.NoError(t, err)

	select {
	case <-connectCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for connect event")
	}

	err = th.userClient.ws.Close()
	require.NoError(t, err)

	select {
	case <-disconnectCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for disconnect event")
	}

	err = th.userClient.Close()
	require.NoError(t, err)
}

func TestClientWSReconnect(t *testing.T) {
	th := setupTestHelper(t, "")

	connectCh := make(chan struct{}, 2)
	err := th.userClient.On(WSConnectEvent, func(_ any) error {
		connectCh <- struct{}{}
		return nil
	})
	require.NoError(t, err)

	disconnectCh := make(chan struct{})
	err = th.userClient.On(WSDisconnectEvent, func(_ any) error {
		close(disconnectCh)
		return nil
	})
	require.NoError(t, err)

	err = th.userClient.Connect()
	require.NoError(t, err)

	select {
	case <-connectCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for connect event")
	}

	err = th.userClient.ws.Close()
	require.NoError(t, err)

	select {
	case <-disconnectCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for disconnect event")
	}

	select {
	case <-connectCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for reconnect event")
	}

	err = th.userClient.Close()
	require.NoError(t, err)
}

func TestClientWSReconnectTimeout(t *testing.T) {
	wsReconnectionTimeout = 10 * time.Second

	th := setupTestHelper(t, "")

	connectCh := make(chan struct{}, 2)
	err := th.userClient.On(WSConnectEvent, func(_ any) error {
		connectCh <- struct{}{}
		return nil
	})
	require.NoError(t, err)

	err = th.userClient.Connect()
	require.NoError(t, err)

	select {
	case <-connectCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for connect event")
	}

	// Bind a listener to get an unused port, then close it so the port gives
	// immediate ECONNREFUSED on reconnect (avoids slow TCP timeouts from
	// non-routable IPs, and avoids accidentally hitting a real server).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	unusedAddr := ln.Addr().String()
	ln.Close()
	th.userClient.cfg.wsURL = fmt.Sprintf("ws://%s", unusedAddr)

	errorCh := make(chan error, 1)
	err = th.userClient.On(ErrorEvent, func(ctx any) error {
		select {
		case errorCh <- ctx.(error):
		default:
		}
		return nil
	})
	require.NoError(t, err)

	closeCh := make(chan struct{})
	err = th.userClient.On(CloseEvent, func(_ any) error {
		close(closeCh)
		return nil
	})
	require.NoError(t, err)

	err = th.userClient.ws.Close()
	require.NoError(t, err)

	select {
	case err := <-errorCh:
		require.EqualError(t, err, "ws reconnection timeout reached")
	case <-time.After(wsReconnectionTimeout * 2):
		require.Fail(t, "timed out waiting for error event")
	}

	select {
	case <-closeCh:
	case <-time.After(wsReconnectionTimeout * 2):
		require.Fail(t, "timed out waiting for close event")
	}
}
