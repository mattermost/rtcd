// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"testing"

	"github.com/pion/webrtc/v3"
	"github.com/stretchr/testify/require"
)

func TestNewICEMessage(t *testing.T) {
	t.Run("host candidate - ip address", func(t *testing.T) {
		msg, err := newICEMessage(&session{
			cfg: SessionConfig{
				SessionID: "sessionID",
				UserID:    "userID",
				CallID:    "callID",
				GroupID:   "groupID",
			},
		}, &webrtc.ICECandidate{
			Address:    "1.1.1.1",
			Port:       8443,
			Priority:   45,
			Typ:        webrtc.ICECandidateTypeHost,
			Protocol:   webrtc.ICEProtocolUDP,
			Foundation: "2145320272",
		})
		require.NoError(t, err)
		require.Equal(t, Message{
			SessionID: "sessionID",
			UserID:    "userID",
			CallID:    "callID",
			GroupID:   "groupID",
			Type:      ICEMessage,
			Data:      []byte(`{"candidate":{"candidate":"candidate:2145320272 0 udp 45 1.1.1.1 8443 typ host","sdpMid":"","sdpMLineIndex":0,"usernameFragment":null},"type":"candidate"}`),
		}, msg)
	})

	t.Run("host candidate - fqdn", func(t *testing.T) {
		msg, err := newICEMessage(&session{
			cfg: SessionConfig{
				SessionID: "sessionID",
				UserID:    "userID",
				CallID:    "callID",
				GroupID:   "groupID",
			},
		}, &webrtc.ICECandidate{
			Address:    "example.tld",
			Port:       8443,
			Priority:   45,
			Typ:        webrtc.ICECandidateTypeHost,
			Protocol:   webrtc.ICEProtocolUDP,
			Foundation: "2145320272",
		})
		require.NoError(t, err)
		require.Equal(t, Message{
			SessionID: "sessionID",
			UserID:    "userID",
			CallID:    "callID",
			GroupID:   "groupID",
			Type:      ICEMessage,
			Data:      []byte(`{"candidate":{"candidate":"candidate:2145320272 0 udp 45 example.tld 8443 typ host","sdpMid":"","sdpMLineIndex":0,"usernameFragment":null},"type":"candidate"}`),
		}, msg)
	})
}
