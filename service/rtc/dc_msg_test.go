// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pion/webrtc/v3"
)

func TestEncodeDCMessage(t *testing.T) {
	t.Run("ping", func(t *testing.T) {
		dcMsg, err := encodeDCMessage(DCMessageTypePing, nil)
		require.NoError(t, err)

		mt, payload, err := decodeDCMessage(dcMsg)
		require.NoError(t, err)
		require.Equal(t, DCMessageTypePing, mt)
		require.Nil(t, payload)
	})

	t.Run("pong", func(t *testing.T) {
		dcMsg, err := encodeDCMessage(DCMessageTypePong, nil)
		require.NoError(t, err)

		mt, payload, err := decodeDCMessage(dcMsg)
		require.NoError(t, err)
		require.Equal(t, DCMessageTypePong, mt)
		require.Nil(t, payload)
	})

	t.Run("sdp", func(t *testing.T) {
		var sdp webrtc.SessionDescription
		sdp.Type = webrtc.SDPTypeOffer
		sdp.SDP = "sdp"

		sdpData, err := json.Marshal(sdp)
		require.NoError(t, err)

		dcMsg, err := encodeDCMessage(DCMessageTypeSDP, sdpData)
		require.NoError(t, err)

		mt, payload, err := decodeDCMessage(dcMsg)
		require.NoError(t, err)
		require.Equal(t, DCMessageTypeSDP, mt)

		var decodedSDP webrtc.SessionDescription
		err = json.Unmarshal(payload.([]byte), &decodedSDP)
		require.NoError(t, err)
		require.Equal(t, sdp, decodedSDP)
	})
}
