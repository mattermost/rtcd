// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"testing"
	"time"

	"github.com/pion/webrtc/v3"

	"github.com/stretchr/testify/require"
)

func TestAPIGetConfig(t *testing.T) {
	th := setupTestHelper(t, "")

	cfg, err := th.userClient.GetCallsConfig()
	require.NoError(t, err)
	require.NotEmpty(t, cfg)

	require.True(t, cfg["AllowEnableCalls"].(bool))
}

func TestAPIMuteUnmute(t *testing.T) {
	th := setupTestHelper(t, "calls0")

	// Setup
	userConnectCh := make(chan struct{})
	err := th.userClient.On(RTCConnectEvent, func(_ any) error {
		close(userConnectCh)
		return nil
	})
	require.NoError(t, err)

	adminConnectCh := make(chan struct{})
	err = th.adminClient.On(RTCConnectEvent, func(_ any) error {
		close(adminConnectCh)
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
	case <-userConnectCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for user connect event")
	}

	select {
	case <-adminConnectCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for admin connect event")
	}

	// Test logic

	userCloseCh := make(chan struct{})
	adminCloseCh := make(chan struct{})

	adminTrackCh := make(chan struct{})
	err = th.adminClient.On(RTCTrackEvent, func(_ any) error {
		close(adminTrackCh)
		return nil
	})
	require.NoError(t, err)

	userUnmutedCh := make(chan struct{})
	err = th.adminClient.On(WSCallUserUnmuted, func(ctx any) error {
		sessionID := ctx.(string)
		if sessionID == th.userClient.originalConnID {
			close(userUnmutedCh)
		}
		return nil
	})
	require.NoError(t, err)

	// User unmutes, admin should receive the track
	userVoiceTrack := th.newVoiceTrack()
	err = th.userClient.Unmute(userVoiceTrack)
	require.NoError(t, err)
	go th.voiceTrackWriter(userVoiceTrack, userCloseCh)

	select {
	case <-userUnmutedCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for user unmuted event")
	}

	select {
	case <-adminTrackCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for admin client to receive track")
	}

	userMutedCh := make(chan struct{})
	err = th.adminClient.On(WSCallUserMuted, func(ctx any) error {
		sessionID := ctx.(string)
		if sessionID == th.userClient.originalConnID {
			close(userMutedCh)
		}
		return nil
	})
	require.NoError(t, err)
	err = th.userClient.Mute()
	require.NoError(t, err)
	select {
	case <-userMutedCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for user muted event")
	}

	userTrackCh := make(chan struct{})
	err = th.userClient.On(RTCTrackEvent, func(_ any) error {
		close(userTrackCh)
		return nil
	})
	require.NoError(t, err)

	adminUnmutedCh := make(chan struct{})
	err = th.userClient.On(WSCallUserUnmuted, func(ctx any) error {
		sessionID := ctx.(string)
		if sessionID == th.adminClient.originalConnID {
			close(adminUnmutedCh)
		}
		return nil
	})
	require.NoError(t, err)

	// Admin unmutes, user should receive the track
	adminVoiceTrack := th.newVoiceTrack()
	err = th.adminClient.Unmute(adminVoiceTrack)
	require.NoError(t, err)
	go th.voiceTrackWriter(adminVoiceTrack, adminCloseCh)

	select {
	case <-adminUnmutedCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for admin unmuted event")
	}

	select {
	case <-userTrackCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for user client to receive track")
	}

	adminMutedCh := make(chan struct{})
	err = th.userClient.On(WSCallUserMuted, func(ctx any) error {
		sessionID := ctx.(string)
		if sessionID == th.adminClient.originalConnID {
			close(adminMutedCh)
		}
		return nil
	})
	require.NoError(t, err)
	err = th.adminClient.Mute()
	require.NoError(t, err)
	select {
	case <-adminMutedCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for admin muted event")
	}

	// Teardown
	err = th.userClient.On(CloseEvent, func(_ any) error {
		close(userCloseCh)
		return nil
	})
	require.NoError(t, err)

	err = th.adminClient.On(CloseEvent, func(_ any) error {
		close(adminCloseCh)
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
	case <-userCloseCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for close event")
	}

	select {
	case <-adminCloseCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for close event")
	}
}

func TestAPIRaiseLowerHand(t *testing.T) {
	th := setupTestHelper(t, "calls0")

	// Setup
	userConnectCh := make(chan struct{})
	err := th.userClient.On(RTCConnectEvent, func(_ any) error {
		close(userConnectCh)
		return nil
	})
	require.NoError(t, err)

	adminConnectCh := make(chan struct{})
	err = th.adminClient.On(RTCConnectEvent, func(_ any) error {
		close(adminConnectCh)
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
	case <-userConnectCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for user connect event")
	}

	select {
	case <-adminConnectCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for admin connect event")
	}

	userRaisedHandCh := make(chan struct{})
	userLoweredHandCh := make(chan struct{})
	adminRaisedHandCh := make(chan struct{})
	adminLoweredHandCh := make(chan struct{})

	err = th.userClient.On(WSCallUserRaisedHand, func(ctx any) error {
		sessionID := ctx.(string)
		if sessionID == th.adminClient.originalConnID {
			close(adminRaisedHandCh)
		}
		return nil
	})
	require.NoError(t, err)
	err = th.userClient.On(WSCallUserLoweredHand, func(ctx any) error {
		sessionID := ctx.(string)
		if sessionID == th.adminClient.originalConnID {
			close(adminLoweredHandCh)
		}
		return nil
	})
	require.NoError(t, err)

	err = th.adminClient.On(WSCallUserRaisedHand, func(ctx any) error {
		sessionID := ctx.(string)
		if sessionID == th.userClient.originalConnID {
			close(userRaisedHandCh)
		}
		return nil
	})
	require.NoError(t, err)
	err = th.adminClient.On(WSCallUserLoweredHand, func(ctx any) error {
		sessionID := ctx.(string)
		if sessionID == th.userClient.originalConnID {
			close(userLoweredHandCh)
		}
		return nil
	})
	require.NoError(t, err)

	err = th.userClient.RaiseHand()
	require.NoError(t, err)
	select {
	case <-userRaisedHandCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for user raised hand event")
	}

	err = th.userClient.LowerHand()
	require.NoError(t, err)
	select {
	case <-userLoweredHandCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for user lowered hand event")
	}

	err = th.adminClient.RaiseHand()
	require.NoError(t, err)
	select {
	case <-adminRaisedHandCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for admin raised hand event")
	}

	err = th.adminClient.LowerHand()
	require.NoError(t, err)
	select {
	case <-adminLoweredHandCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for admin lowered hand event")
	}

	// Teardown

	userCloseCh := make(chan struct{})
	adminCloseCh := make(chan struct{})

	err = th.userClient.On(CloseEvent, func(_ any) error {
		close(userCloseCh)
		return nil
	})
	require.NoError(t, err)

	err = th.adminClient.On(CloseEvent, func(_ any) error {
		close(adminCloseCh)
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
	case <-userCloseCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for close event")
	}

	select {
	case <-adminCloseCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for close event")
	}
}

func TestAPIScreenShare(t *testing.T) {
	th := setupTestHelper(t, "calls0")

	// Setup
	userConnectCh := make(chan struct{})
	err := th.userClient.On(RTCConnectEvent, func(_ any) error {
		close(userConnectCh)
		return nil
	})
	require.NoError(t, err)

	adminConnectCh := make(chan struct{})
	err = th.adminClient.On(RTCConnectEvent, func(_ any) error {
		close(adminConnectCh)
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
	case <-userConnectCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for user connect event")
	}

	select {
	case <-adminConnectCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for admin connect event")
	}

	userCloseCh := make(chan struct{})
	adminCloseCh := make(chan struct{})

	// Test logic

	// User screen shares, admin should receive the track
	userScreenTrack := th.newScreenTrack()
	_, err = th.userClient.StartScreenShare([]webrtc.TrackLocal{userScreenTrack})
	require.NoError(t, err)
	go th.screenTrackWriter(userScreenTrack, userCloseCh)

	screenTrackCh := make(chan struct{})
	err = th.adminClient.On(RTCTrackEvent, func(_ any) error {
		close(screenTrackCh)
		return nil
	})
	require.NoError(t, err)

	userScreenOnCh := make(chan struct{})
	err = th.adminClient.On(WSCallScreenOn, func(ctx any) error {
		sessionID := ctx.(string)
		if sessionID == th.userClient.originalConnID {
			close(userScreenOnCh)
		}
		return nil
	})
	require.NoError(t, err)

	select {
	case <-userScreenOnCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for user screen on event")
	}

	select {
	case <-screenTrackCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for screen track")
	}

	userScreenOffCh := make(chan struct{})
	err = th.adminClient.On(WSCallScreenOff, func(ctx any) error {
		sessionID := ctx.(string)
		if sessionID == th.userClient.originalConnID {
			close(userScreenOffCh)
		}
		return nil
	})
	require.NoError(t, err)

	err = th.userClient.StopScreenShare()
	require.NoError(t, err)

	select {
	case <-userScreenOffCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for user screen off event")
	}

	// Teardown

	err = th.userClient.On(CloseEvent, func(_ any) error {
		close(userCloseCh)
		return nil
	})
	require.NoError(t, err)

	err = th.adminClient.On(CloseEvent, func(_ any) error {
		close(adminCloseCh)
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
	case <-userCloseCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for close event")
	}

	select {
	case <-adminCloseCh:
	case <-time.After(waitTimeout):
		require.Fail(t, "timed out waiting for close event")
	}
}