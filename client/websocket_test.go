// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientWSDisconnect(t *testing.T) {
	th := setupTestHelper(t, "")

	connectCh := make(chan struct{}, 2)
	th.userClient.On(WSConnectEvent, func() error {
		connectCh <- struct{}{}
		return nil
	})

	disconnectCh := make(chan struct{})
	th.userClient.On(WSDisconnectEvent, func() error {
		close(disconnectCh)
		return nil
	})

	err := th.userClient.Connect()
	require.NoError(t, err)

	select {
	case <-connectCh:
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for connect event")
	}

	err = th.userClient.ws.Close()
	require.NoError(t, err)

	select {
	case <-disconnectCh:
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for disconnect event")
	}

	err = th.userClient.Close()
	require.NoError(t, err)
}

func TestClientWSReconnect(t *testing.T) {
	th := setupTestHelper(t, "")

	connectCh := make(chan struct{}, 2)
	th.userClient.On(WSConnectEvent, func() error {
		connectCh <- struct{}{}
		return nil
	})

	disconnectCh := make(chan struct{})
	th.userClient.On(WSDisconnectEvent, func() error {
		close(disconnectCh)
		return nil
	})

	err := th.userClient.Connect()
	require.NoError(t, err)

	select {
	case <-connectCh:
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for connect event")
	}

	err = th.userClient.ws.Close()
	require.NoError(t, err)

	select {
	case <-disconnectCh:
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for disconnect event")
	}

	select {
	case <-connectCh:
	case <-time.After(2 * time.Second):
		require.Fail(t, "timed out waiting for reconnect event")
	}

	err = th.userClient.Close()
	require.NoError(t, err)
}

func TestClientWSReconnectTimeout(t *testing.T) {
	wsReconnectionTimeout = 10 * time.Second

	th := setupTestHelper(t, "")

	connectCh := make(chan struct{}, 2)
	th.userClient.On(WSConnectEvent, func() error {
		connectCh <- struct{}{}
		return nil
	})

	err := th.userClient.Connect()
	require.NoError(t, err)

	select {
	case <-connectCh:
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for connect event")
	}

	th.userClient.cfg.wsURL = "ws://localhost:8080"
	err = th.userClient.ws.Close()
	require.NoError(t, err)

	closeCh := make(chan struct{})
	th.userClient.On(CloseEvent, func() error {
		close(closeCh)
		return nil
	})

	select {
	case <-closeCh:
	case <-time.After(wsReconnectionTimeout * 2):
		require.Fail(t, "timed out waiting for close event")
	}
}
