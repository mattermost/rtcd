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
	err := th.userClient.On(WSConnectEvent, func(_ any) error {
		close(connectCh)
		return nil
	})
	require.NoError(t, err)

	closeCh := make(chan struct{})
	err = th.userClient.On(CloseEvent, func(_ any) error {
		close(closeCh)
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

	err = th.userClient.Close()
	require.NoError(t, err)

	select {
	case <-closeCh:
	case <-time.After(waitTimeout):
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

			_ = th.userClient.On(WSConnectEvent, func(_ any) error {
				close(connectCh)
				return nil
			})

			_ = th.userClient.On(CloseEvent, func(_ any) error {
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

func TestClientJoinCall(t *testing.T) {
	t.Run("invalid channel", func(t *testing.T) {
		th := setupTestHelper(t, "")

		err := th.userClient.Connect()
		require.NoError(t, err)

		connectCh := make(chan struct{})
		err = th.userClient.On(WSConnectEvent, func(_ any) error {
			close(connectCh)
			return nil
		})
		require.NoError(t, err)

		closeCh := make(chan struct{})
		err = th.userClient.On(CloseEvent, func(_ any) error {
			close(closeCh)
			return nil
		})
		require.NoError(t, err)

		errorCh := make(chan struct{})
		err = th.userClient.On(ErrorEvent, func(err any) error {
			require.EqualError(t, err.(error), "ws error: forbidden")
			close(errorCh)
			return nil
		})
		require.NoError(t, err)

		select {
		case <-connectCh:
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for connect event")
		}

		select {
		case <-errorCh:
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for error event")
		}

		err = th.userClient.Close()
		require.NoError(t, err)

		select {
		case <-closeCh:
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for close event")
		}
	})

	t.Run("success", func(t *testing.T) {
		th := setupTestHelper(t, "calls0")

		err := th.userClient.Connect()
		require.NoError(t, err)

		connectCh := make(chan struct{})
		err = th.userClient.On(WSConnectEvent, func(_ any) error {
			close(connectCh)
			return nil
		})
		require.NoError(t, err)

		closeCh := make(chan struct{})
		err = th.userClient.On(CloseEvent, func(_ any) error {
			close(closeCh)
			return nil
		})
		require.NoError(t, err)

		joinCh := make(chan struct{})
		err = th.userClient.On(WSCallJoinEvent, func(_ any) error {
			close(joinCh)
			return nil
		})
		require.NoError(t, err)

		select {
		case <-connectCh:
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for connect event")
		}

		select {
		case <-joinCh:
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for join event")
		}

		err = th.userClient.Close()
		require.NoError(t, err)

		select {
		case <-closeCh:
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for close event")
		}
	})
}

func TestClientOn(t *testing.T) {
	th := setupTestHelper(t, "")

	t.Run("invalid event", func(t *testing.T) {
		err := th.userClient.On("invalid", func(_ any) error {
			return nil
		})
		require.EqualError(t, err, "invalid event type \"invalid\"")
	})

	t.Run("double registration", func(t *testing.T) {
		err := th.userClient.On(WSConnectEvent, func(_ any) error {
			return nil
		})
		require.NoError(t, err)

		err = th.userClient.On(WSConnectEvent, func(_ any) error {
			return nil
		})
		require.EqualError(t, err, "already subscribed")
	})
}
