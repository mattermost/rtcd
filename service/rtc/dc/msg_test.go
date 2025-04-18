// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package dc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pion/webrtc/v4"
)

func TestEncodeMessage(t *testing.T) {
	t.Run("ping", func(t *testing.T) {
		dcMsg, err := EncodeMessage(MessageTypePing, nil)
		require.NoError(t, err)

		mt, payload, err := DecodeMessage(dcMsg)
		require.NoError(t, err)
		require.Equal(t, MessageTypePing, mt)
		require.Nil(t, payload)
	})

	t.Run("pong", func(t *testing.T) {
		dcMsg, err := EncodeMessage(MessageTypePong, nil)
		require.NoError(t, err)

		mt, payload, err := DecodeMessage(dcMsg)
		require.NoError(t, err)
		require.Equal(t, MessageTypePong, mt)
		require.Nil(t, payload)
	})

	t.Run("sdp", func(t *testing.T) {
		var sdp webrtc.SessionDescription
		sdp.Type = webrtc.SDPTypeOffer
		sdp.SDP = "sdp"

		sdpData, err := json.Marshal(sdp)
		require.NoError(t, err)

		dcMsg, err := EncodeMessage(MessageTypeSDP, sdpData)
		require.NoError(t, err)

		mt, payload, err := DecodeMessage(dcMsg)
		require.NoError(t, err)
		require.Equal(t, MessageTypeSDP, mt)

		var decodedSDP webrtc.SessionDescription
		err = json.Unmarshal(payload.([]byte), &decodedSDP)
		require.NoError(t, err)
		require.Equal(t, sdp, decodedSDP)
	})

	t.Run("mediamap", func(t *testing.T) {
		mediaMap := MediaMap{
			"1": TrackInfo{
				Type:     "voice",
				SenderID: "sessionA",
				MimeType: webrtc.MimeTypeVP8,
			},
			"2": TrackInfo{
				Type:     "screen",
				SenderID: "sessionB",
				MimeType: webrtc.MimeTypeAV1,
			},
		}

		dcMsg, err := EncodeMessage(MessageTypeMediaMap, mediaMap)
		require.NoError(t, err)

		mt, payload, err := DecodeMessage(dcMsg)
		require.NoError(t, err)
		require.Equal(t, MessageTypeMediaMap, mt)

		decodedMediaMap, ok := payload.(MediaMap)
		require.True(t, ok, "payload should be of type MediaMap")
		require.Equal(t, mediaMap, decodedMediaMap)

		// Verify individual entries
		require.Equal(t, "voice", decodedMediaMap["1"].Type)
		require.Equal(t, "sessionA", decodedMediaMap["1"].SenderID)
		require.Equal(t, webrtc.MimeTypeVP8, decodedMediaMap["1"].MimeType)
		require.Equal(t, "screen", decodedMediaMap["2"].Type)
		require.Equal(t, "sessionB", decodedMediaMap["2"].SenderID)
		require.Equal(t, webrtc.MimeTypeAV1, decodedMediaMap["2"].MimeType)
	})

	t.Run("codecsupportmap", func(t *testing.T) {
		codecSupportMap := CodecSupportMap{
			webrtc.MimeTypeAV1: CodecSupportFull,
			webrtc.MimeTypeVP8: CodecSupportPartial,
		}

		dcMsg, err := EncodeMessage(MessageTypeCodecSupportMap, codecSupportMap)
		require.NoError(t, err)

		mt, payload, err := DecodeMessage(dcMsg)
		require.NoError(t, err)
		require.Equal(t, MessageTypeCodecSupportMap, mt)

		decodedCodecSupportMap, ok := payload.(CodecSupportMap)
		require.True(t, ok, "payload should be of type CodecSupportMap")
		require.Equal(t, codecSupportMap, decodedCodecSupportMap)

		// Verify individual entries
		require.Equal(t, CodecSupportFull, decodedCodecSupportMap[webrtc.MimeTypeAV1])
		require.Equal(t, CodecSupportPartial, decodedCodecSupportMap[webrtc.MimeTypeVP8])
	})
}
