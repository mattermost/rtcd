// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"testing"
	"time"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
)

func TestClientConnectCall(t *testing.T) {
	th := setupTestHelper(t, "calls0")

	closeCh := make(chan struct{})
	th.userClient.On(CloseEvent, func(_ any) error {
		close(closeCh)
		return nil
	})

	rtcConnectCh := make(chan struct{})
	th.userClient.On(RTCConnectEvent, func(_ any) error {
		close(rtcConnectCh)
		return nil
	})

	rtcDisconnectCh := make(chan struct{})
	th.userClient.On(RTCDisconnectEvent, func(_ any) error {
		close(rtcDisconnectCh)
		return nil
	})

	err := th.userClient.Connect()
	require.NoError(t, err)

	select {
	case <-rtcConnectCh:
	case <-time.After(4 * time.Second):
		require.Fail(t, "timed out waiting for rtc connect event")
	}

	err = th.userClient.Close()
	require.NoError(t, err)

	select {
	case <-rtcDisconnectCh:
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for rtc disconnect event")
	}

	select {
	case <-closeCh:
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for close event")
	}
}

func TestRTCDisconnect(t *testing.T) {
	th := setupTestHelper(t, "calls0")

	closeCh := make(chan struct{})
	th.userClient.On(CloseEvent, func(_ any) error {
		close(closeCh)
		return nil
	})

	rtcConnectCh := make(chan struct{})
	th.userClient.On(RTCConnectEvent, func(_ any) error {
		close(rtcConnectCh)
		return nil
	})

	rtcDisconnectCh := make(chan struct{})
	th.userClient.On(RTCDisconnectEvent, func(_ any) error {
		close(rtcDisconnectCh)
		return nil
	})

	err := th.userClient.Connect()
	require.NoError(t, err)

	select {
	case <-rtcConnectCh:
	case <-time.After(4 * time.Second):
		require.Fail(t, "timed out waiting for rtc connect event")
	}

	err = th.userClient.pc.Close()
	require.NoError(t, err)

	select {
	case <-rtcDisconnectCh:
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for rtc disconnect event")
	}

	select {
	case <-closeCh:
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for close event")
	}
}

func TestRTCTrack(t *testing.T) {
	t.Run("connect before track", func(t *testing.T) {
		th := setupTestHelper(t, "calls0")

		rtcConnectChA := make(chan struct{})
		th.userClient.On(RTCConnectEvent, func(_ any) error {
			close(rtcConnectChA)
			return nil
		})

		rtcConnectChB := make(chan struct{})
		th.adminClient.On(RTCConnectEvent, func(_ any) error {
			close(rtcConnectChB)
			return nil
		})

		closeChA := make(chan struct{})
		th.userClient.On(CloseEvent, func(_ any) error {
			close(closeChA)
			return nil
		})

		closeChB := make(chan struct{})
		th.adminClient.On(CloseEvent, func(_ any) error {
			close(closeChB)
			return nil
		})

		rtcTrackCh := make(chan struct{})
		th.userClient.On(RTCTrackEvent, func(ctx any) error {
			track, ok := ctx.(*webrtc.TrackRemote)
			require.True(t, ok)
			require.Equal(t, webrtc.PayloadType(0x6f), track.PayloadType())
			require.Equal(t, "audio/opus", track.Codec().MimeType)

			close(rtcTrackCh)
			return nil
		})

		go func() {
			err := th.userClient.Connect()
			require.NoError(t, err)
		}()

		go func() {
			err := th.adminClient.Connect()
			require.NoError(t, err)
		}()

		select {
		case <-rtcConnectChA:
		case <-time.After(4 * time.Second):
			require.Fail(t, "timed out waiting for rtc connect event")
		}

		select {
		case <-rtcConnectChB:
		case <-time.After(4 * time.Second):
			require.Fail(t, "timed out waiting for rtc connect event")
		}

		th.transmitAudioTrack(th.adminClient)

		select {
		case <-rtcTrackCh:
		case <-time.After(4 * time.Second):
			require.Fail(t, "timed out waiting for rtc track event")
		}

		go func() {
			err := th.userClient.Close()
			require.NoError(t, err)
		}()

		go func() {
			err := th.adminClient.Close()
			require.NoError(t, err)
		}()

		select {
		case <-closeChA:
		case <-time.After(time.Second):
			require.Fail(t, "timed out waiting for close event")
		}

		select {
		case <-closeChB:
		case <-time.After(time.Second):
			require.Fail(t, "timed out waiting for close event")
		}
	})

	t.Run("connect after track", func(t *testing.T) {
		th := setupTestHelper(t, "calls0")

		rtcConnectChA := make(chan struct{})
		th.userClient.On(RTCConnectEvent, func(_ any) error {
			close(rtcConnectChA)
			return nil
		})

		rtcConnectChB := make(chan struct{})
		th.adminClient.On(RTCConnectEvent, func(_ any) error {
			close(rtcConnectChB)
			return nil
		})

		closeChA := make(chan struct{})
		th.userClient.On(CloseEvent, func(_ any) error {
			close(closeChA)
			return nil
		})

		closeChB := make(chan struct{})
		th.adminClient.On(CloseEvent, func(_ any) error {
			close(closeChB)
			return nil
		})

		rtcTrackCh := make(chan struct{})
		th.userClient.On(RTCTrackEvent, func(ctx any) error {
			track, ok := ctx.(*webrtc.TrackRemote)
			require.True(t, ok)
			require.Equal(t, webrtc.PayloadType(0x6f), track.PayloadType())
			require.Equal(t, "audio/opus", track.Codec().MimeType)

			close(rtcTrackCh)
			return nil
		})

		err := th.adminClient.Connect()
		require.NoError(t, err)

		select {
		case <-rtcConnectChB:
		case <-time.After(4 * time.Second):
			require.Fail(t, "timed out waiting for rtc connect event")
		}

		th.transmitAudioTrack(th.adminClient)

		err = th.userClient.Connect()
		require.NoError(t, err)

		select {
		case <-rtcConnectChA:
		case <-time.After(4 * time.Second):
			require.Fail(t, "timed out waiting for rtc connect event")
		}

		select {
		case <-rtcTrackCh:
		case <-time.After(4 * time.Second):
			require.Fail(t, "timed out waiting for rtc track event")
		}

		go func() {
			err := th.userClient.Close()
			require.NoError(t, err)
		}()

		go func() {
			err := th.adminClient.Close()
			require.NoError(t, err)
		}()

		select {
		case <-closeChA:
		case <-time.After(time.Second):
			require.Fail(t, "timed out waiting for close event")
		}

		select {
		case <-closeChB:
		case <-time.After(time.Second):
			require.Fail(t, "timed out waiting for close event")
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		th := setupTestHelper(t, "calls0")

		rtcConnectChA := make(chan struct{})
		th.userClient.On(RTCConnectEvent, func(_ any) error {
			close(rtcConnectChA)
			return nil
		})

		rtcConnectChB := make(chan struct{})
		th.adminClient.On(RTCConnectEvent, func(_ any) error {
			close(rtcConnectChB)
			return nil
		})

		closeChA := make(chan struct{})
		th.userClient.On(CloseEvent, func(_ any) error {
			close(closeChA)
			return nil
		})

		closeChB := make(chan struct{})
		th.adminClient.On(CloseEvent, func(_ any) error {
			close(closeChB)
			return nil
		})

		rtcTrackCh := make(chan struct{})
		th.userClient.On(RTCTrackEvent, func(ctx any) error {
			track, ok := ctx.(*webrtc.TrackRemote)
			require.True(t, ok)
			require.Equal(t, webrtc.PayloadType(0x6f), track.PayloadType())
			require.Equal(t, "audio/opus", track.Codec().MimeType)

			close(rtcTrackCh)
			return nil
		})

		err := th.adminClient.Connect()
		require.NoError(t, err)

		select {
		case <-rtcConnectChB:
		case <-time.After(4 * time.Second):
			require.Fail(t, "timed out waiting for rtc connect event")
		}

		go func() {
			th.transmitAudioTrack(th.adminClient)
		}()

		go func() {
			err := th.userClient.Connect()
			require.NoError(t, err)
		}()

		select {
		case <-rtcConnectChA:
		case <-time.After(4 * time.Second):
			require.Fail(t, "timed out waiting for rtc connect event")
		}

		select {
		case <-rtcTrackCh:
		case <-time.After(4 * time.Second):
			require.Fail(t, "timed out waiting for rtc track event")
		}

		go func() {
			err := th.userClient.Close()
			require.NoError(t, err)
		}()

		go func() {
			err := th.adminClient.Close()
			require.NoError(t, err)
		}()

		select {
		case <-closeChA:
		case <-time.After(time.Second):
			require.Fail(t, "timed out waiting for close event")
		}

		select {
		case <-closeChB:
		case <-time.After(time.Second):
			require.Fail(t, "timed out waiting for close event")
		}
	})
}
