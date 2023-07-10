// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientConnect(t *testing.T) {
	th := setupTestHelper(t, "")

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

	err := th.userClient.Connect()
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
	th := setupTestHelper(t, "")

	t.Run("double connect", func(t *testing.T) {
		err := th.userClient.Connect()
		require.NoError(t, err)

		require.Equal(t, clientStateInit, th.userClient.state)

		err = th.userClient.Connect()
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
		err := th.userClient.Connect()
		require.EqualError(t, err, "ws client is already initialized")
	})
}

func TestClientConcurrency(t *testing.T) {
	th := setupTestHelper(t, "")

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

			err := th.userClient.Connect()
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
