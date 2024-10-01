// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package dc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pion/webrtc/v3"
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
}
