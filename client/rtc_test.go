// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

func TestClientConnectCall(t *testing.T) {
	th := setupTestHelper(t, "calls0")

	closeCh := make(chan struct{})
	err := th.userClient.On(CloseEvent, func(_ any) error {
		close(closeCh)
		return nil
	})
	require.NoError(t, err)

	rtcConnectCh := make(chan struct{})
	err = th.userClient.On(RTCConnectEvent, func(_ any) error {
		close(rtcConnectCh)
		return nil
	})
	require.NoError(t, err)

	rtcDisconnectCh := make(chan struct{})
	err = th.userClient.On(RTCDisconnectEvent, func(_ any) error {
		close(rtcDisconnectCh)
		return nil
	})
	require.NoError(t, err)

	err = th.userClient.Connect()
	require.NoError(t, err)

	select {
	case <-rtcConnectCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for rtc connect event")
	}

	err = th.userClient.Close()
	require.NoError(t, err)

	select {
	case <-rtcDisconnectCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for rtc disconnect event")
	}

	select {
	case <-closeCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for close event")
	}
}

func TestRTCDisconnect(t *testing.T) {
	th := setupTestHelper(t, "calls0")

	closeCh := make(chan struct{})
	err := th.userClient.On(CloseEvent, func(_ any) error {
		close(closeCh)
		return nil
	})
	require.NoError(t, err)

	rtcConnectCh := make(chan struct{})
	err = th.userClient.On(RTCConnectEvent, func(_ any) error {
		close(rtcConnectCh)
		return nil
	})
	require.NoError(t, err)

	rtcDisconnectCh := make(chan struct{})
	err = th.userClient.On(RTCDisconnectEvent, func(_ any) error {
		close(rtcDisconnectCh)
		return nil
	})
	require.NoError(t, err)

	err = th.userClient.Connect()
	require.NoError(t, err)

	select {
	case <-rtcConnectCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for rtc connect event")
	}

	err = th.userClient.pc.Close()
	require.NoError(t, err)

	select {
	case <-rtcDisconnectCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for rtc disconnect event")
	}

	select {
	case <-closeCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for close event")
	}
}

func TestRTCTrack(t *testing.T) {
	t.Run("connect before track", func(t *testing.T) {
		th := setupTestHelper(t, "calls0")

		rtcConnectChA := make(chan struct{})
		err := th.userClient.On(RTCConnectEvent, func(_ any) error {
			close(rtcConnectChA)
			return nil
		})
		require.NoError(t, err)

		rtcConnectChB := make(chan struct{})
		err = th.adminClient.On(RTCConnectEvent, func(_ any) error {
			close(rtcConnectChB)
			return nil
		})
		require.NoError(t, err)

		closeChA := make(chan struct{})
		err = th.userClient.On(CloseEvent, func(_ any) error {
			close(closeChA)
			return nil
		})
		require.NoError(t, err)

		closeChB := make(chan struct{})
		err = th.adminClient.On(CloseEvent, func(_ any) error {
			close(closeChB)
			return nil
		})
		require.NoError(t, err)

		rtcTrackCh := make(chan struct{})
		err = th.userClient.On(RTCTrackEvent, func(ctx any) error {
			ctxMap, ok := ctx.(map[string]any)
			require.True(t, ok)
			track, ok := ctxMap["track"].(*webrtc.TrackRemote)
			require.True(t, ok)
			require.Equal(t, webrtc.PayloadType(0x6f), track.PayloadType())
			require.Equal(t, "audio/opus", track.Codec().MimeType)

			close(rtcTrackCh)
			return nil
		})
		require.NoError(t, err)

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
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for rtc connect event")
		}

		select {
		case <-rtcConnectChB:
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for rtc connect event")
		}

		th.transmitAudioTrack(th.adminClient)

		select {
		case <-rtcTrackCh:
		case <-time.After(waitTimeout):
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
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for close event")
		}

		select {
		case <-closeChB:
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for close event")
		}
	})

	t.Run("connect after track", func(t *testing.T) {
		th := setupTestHelper(t, "calls0")

		rtcConnectChA := make(chan struct{})
		err := th.userClient.On(RTCConnectEvent, func(_ any) error {
			close(rtcConnectChA)
			return nil
		})
		require.NoError(t, err)

		rtcConnectChB := make(chan struct{})
		err = th.adminClient.On(RTCConnectEvent, func(_ any) error {
			close(rtcConnectChB)
			return nil
		})
		require.NoError(t, err)

		closeChA := make(chan struct{})
		err = th.userClient.On(CloseEvent, func(_ any) error {
			close(closeChA)
			return nil
		})
		require.NoError(t, err)

		closeChB := make(chan struct{})
		err = th.adminClient.On(CloseEvent, func(_ any) error {
			close(closeChB)
			return nil
		})
		require.NoError(t, err)

		rtcTrackCh := make(chan struct{})
		err = th.userClient.On(RTCTrackEvent, func(ctx any) error {
			ctxMap, ok := ctx.(map[string]any)
			require.True(t, ok)
			track, ok := ctxMap["track"].(*webrtc.TrackRemote)
			require.True(t, ok)
			require.Equal(t, webrtc.PayloadType(0x6f), track.PayloadType())
			require.Equal(t, "audio/opus", track.Codec().MimeType)

			close(rtcTrackCh)
			return nil
		})
		require.NoError(t, err)

		err = th.adminClient.Connect()
		require.NoError(t, err)

		select {
		case <-rtcConnectChB:
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for rtc connect event")
		}

		th.transmitAudioTrack(th.adminClient)

		err = th.userClient.Connect()
		require.NoError(t, err)

		select {
		case <-rtcConnectChA:
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for rtc connect event")
		}

		select {
		case <-rtcTrackCh:
		case <-time.After(waitTimeout):
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
		case <-time.After(waitTimeout):
		}

		select {
		case <-closeChB:
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for close event")
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		th := setupTestHelper(t, "calls0")

		rtcConnectChA := make(chan struct{})
		err := th.userClient.On(RTCConnectEvent, func(_ any) error {
			close(rtcConnectChA)
			return nil
		})
		require.NoError(t, err)

		rtcConnectChB := make(chan struct{})
		err = th.adminClient.On(RTCConnectEvent, func(_ any) error {
			close(rtcConnectChB)
			return nil
		})
		require.NoError(t, err)

		closeChA := make(chan struct{})
		err = th.userClient.On(CloseEvent, func(_ any) error {
			close(closeChA)
			return nil
		})
		require.NoError(t, err)

		closeChB := make(chan struct{})
		err = th.adminClient.On(CloseEvent, func(_ any) error {
			close(closeChB)
			return nil
		})
		require.NoError(t, err)

		rtcTrackCh := make(chan struct{})
		err = th.userClient.On(RTCTrackEvent, func(ctx any) error {
			ctxMap, ok := ctx.(map[string]any)
			require.True(t, ok)
			track, ok := ctxMap["track"].(*webrtc.TrackRemote)
			require.True(t, ok)
			require.Equal(t, webrtc.PayloadType(0x6f), track.PayloadType())
			require.Equal(t, "audio/opus", track.Codec().MimeType)

			th.userClient.mut.RLock()
			require.Len(t, th.userClient.receivers, 1)
			require.NotNil(t, th.userClient.receivers[th.adminClient.originalConnID])
			th.userClient.mut.RUnlock()

			close(rtcTrackCh)
			return nil
		})
		require.NoError(t, err)

		err = th.adminClient.Connect()
		require.NoError(t, err)

		select {
		case <-rtcConnectChB:
		case <-time.After(waitTimeout):
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
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for rtc connect event")
		}

		select {
		case <-rtcTrackCh:
		case <-time.After(waitTimeout):
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
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for close event")
		}

		select {
		case <-closeChB:
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for close event")
		}
	})

	t.Run("multiple remote tracks per session", func(t *testing.T) {
		th := setupTestHelper(t, "calls0")

		rtcConnectChA := make(chan struct{})
		err := th.userClient.On(RTCConnectEvent, func(_ any) error {
			close(rtcConnectChA)
			return nil
		})
		require.NoError(t, err)

		rtcConnectChB := make(chan struct{})
		err = th.adminClient.On(RTCConnectEvent, func(_ any) error {
			close(rtcConnectChB)
			return nil
		})
		require.NoError(t, err)

		go func() {
			err := th.adminClient.Connect()
			require.NoError(t, err)
		}()

		go func() {
			err := th.userClient.Connect()
			require.NoError(t, err)
		}()

		select {
		case <-rtcConnectChA:
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for rtc connect event")
		}

		select {
		case <-rtcConnectChB:
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for rtc connect event")
		}

		voiceTrackCh := make(chan struct{})
		screenTrackCh := make(chan struct{})
		err = th.adminClient.On(RTCTrackEvent, func(ctx any) error {
			ctxMap, ok := ctx.(map[string]any)
			require.True(t, ok)
			track, ok := ctxMap["track"].(*webrtc.TrackRemote)
			require.True(t, ok)

			receiver, ok := ctxMap["receiver"].(*webrtc.RTPReceiver)
			require.True(t, ok)
			require.Equal(t, track, receiver.Track())

			trackType, sessionID, err := ParseTrackID(track.ID())
			require.NoError(t, err)

			require.Equal(t, th.userClient.originalConnID, sessionID)

			if trackType == TrackTypeVoice {
				require.Equal(t, webrtc.PayloadType(0x6f), track.PayloadType())
				require.Equal(t, "audio/opus", track.Codec().MimeType)
				close(voiceTrackCh)
			} else if trackType == TrackTypeScreen {
				require.Equal(t, webrtc.PayloadType(0x60), track.PayloadType())
				require.Equal(t, "video/VP8", track.Codec().MimeType)
				close(screenTrackCh)
			} else {
				require.Fail(t, "unexpected track type received")
			}

			return nil
		})
		require.NoError(t, err)

		go func() {
			th.transmitAudioTrack(th.userClient)
		}()

		go func() {
			th.transmitScreenTrack(th.userClient)
		}()

		select {
		case <-voiceTrackCh:
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for voice track")
		}

		select {
		case <-screenTrackCh:
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for screen track")
		}

		th.userClient.mut.RLock()
		require.Len(t, th.adminClient.receivers, 1)
		require.Len(t, th.adminClient.receivers[th.userClient.originalConnID], 2)
		th.userClient.mut.RUnlock()

		closeChA := make(chan struct{})
		err = th.userClient.On(CloseEvent, func(_ any) error {
			close(closeChA)
			return nil
		})
		require.NoError(t, err)

		closeChB := make(chan struct{})
		err = th.adminClient.On(CloseEvent, func(_ any) error {
			close(closeChB)
			return nil
		})
		require.NoError(t, err)

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
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for close event")
		}

		select {
		case <-closeChB:
		case <-time.After(waitTimeout):
			require.Fail(t, "timed out waiting for close event")
		}
	})
}
