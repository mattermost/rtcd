// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"testing"

	"github.com/mattermost/rtcd/service/rtc/dc"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/require"
)

func TestGetCodecSupportMap(t *testing.T) {
	tests := []struct {
		name            string
		setupSessions   func(c *call)
		expectedSupport dc.CodecSupportLevel
	}{
		{
			name:            "no sessions",
			setupSessions:   func(_ *call) {},
			expectedSupport: dc.CodecSupportNone,
		},
		{
			name: "all sessions support AV1",
			setupSessions: func(c *call) {
				// Add three sessions that all support AV1
				c.sessions["session1"] = &session{cfg: SessionConfig{Props: SessionProps{"av1Support": true}}}
				c.sessions["session2"] = &session{cfg: SessionConfig{Props: SessionProps{"av1Support": true}}}
				c.sessions["session3"] = &session{cfg: SessionConfig{Props: SessionProps{"av1Support": true}}}
			},
			expectedSupport: dc.CodecSupportFull,
		},
		{
			name: "some sessions support AV1",
			setupSessions: func(c *call) {
				// Add three sessions where only some support AV1
				c.sessions["session1"] = &session{cfg: SessionConfig{Props: SessionProps{"av1Support": true}}}
				c.sessions["session2"] = &session{cfg: SessionConfig{Props: SessionProps{"av1Support": false}}}
				c.sessions["session3"] = &session{cfg: SessionConfig{Props: SessionProps{"av1Support": true}}}
			},
			expectedSupport: dc.CodecSupportPartial,
		},
		{
			name: "no sessions support AV1",
			setupSessions: func(c *call) {
				// Add three sessions where none support AV1
				c.sessions["session1"] = &session{cfg: SessionConfig{Props: SessionProps{"av1Support": false}}}
				c.sessions["session2"] = &session{cfg: SessionConfig{Props: SessionProps{"av1Support": false}}}
				c.sessions["session3"] = &session{cfg: SessionConfig{Props: SessionProps{"av1Support": false}}}
			},
			expectedSupport: dc.CodecSupportNone,
		},
		{
			name: "sessions with nil props",
			setupSessions: func(c *call) {
				// Add sessions with nil props (should default to no AV1 support)
				c.sessions["session1"] = &session{cfg: SessionConfig{}}
				c.sessions["session2"] = &session{cfg: SessionConfig{}}
			},
			expectedSupport: dc.CodecSupportNone,
		},
		{
			name: "mixed props configuration",
			setupSessions: func(c *call) {
				// Add sessions with mixed prop configurations
				c.sessions["session1"] = &session{cfg: SessionConfig{Props: SessionProps{"av1Support": true}}}
				c.sessions["session2"] = &session{cfg: SessionConfig{}} // nil props
				c.sessions["session3"] = &session{cfg: SessionConfig{Props: SessionProps{"av1Support": false}}}
			},
			expectedSupport: dc.CodecSupportPartial,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &call{
				sessions: make(map[string]*session),
			}

			// Setup the test case
			tt.setupSessions(c)

			// Get the codec support map
			supportMap := c.getCodecSupportMap()

			// Check the full codec support map
			expectedMap := dc.CodecSupportMap{
				webrtc.MimeTypeAV1: tt.expectedSupport,
			}
			require.Equal(t, expectedMap, supportMap)
		})
	}
}
