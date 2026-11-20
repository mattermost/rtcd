// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"testing"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

func TestShouldSkipTrack(t *testing.T) {
	tcs := []struct {
		name        string
		isScreen    bool
		mimeType    string
		senderAV1   bool
		receiverAV1 bool
		skip        bool
	}{
		{"VP8 camera, both AV1 capable (MM-70762)", false, webrtc.MimeTypeVP8, true, true, false},
		{"VP8 camera, receiver without AV1", false, webrtc.MimeTypeVP8, true, false, false},
		{"VP8 camera, sender without AV1", false, webrtc.MimeTypeVP8, false, true, false},
		{"AV1 camera, receiver with AV1", false, webrtc.MimeTypeAV1, true, true, false},
		{"AV1 camera, receiver without AV1", false, webrtc.MimeTypeAV1, true, false, true},
		{"VP8 screen, both AV1 capable", true, webrtc.MimeTypeVP8, true, true, true},
		{"VP8 screen, receiver without AV1", true, webrtc.MimeTypeVP8, true, false, false},
		{"VP8 screen, sender without AV1", true, webrtc.MimeTypeVP8, false, true, false},
		{"AV1 screen, receiver with AV1", true, webrtc.MimeTypeAV1, true, true, false},
		{"AV1 screen, receiver without AV1", true, webrtc.MimeTypeAV1, true, false, true},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			skip, reason := shouldSkipTrack(tc.isScreen, tc.mimeType, tc.senderAV1, tc.receiverAV1)
			require.Equal(t, tc.skip, skip)
			if skip {
				require.NotEmpty(t, reason)
			} else {
				require.Empty(t, reason)
			}
		})
	}
}
