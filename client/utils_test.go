// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTrackID(t *testing.T) {
	tcs := []struct {
		name      string
		trackType string
		sessionID string
		trackID   string
		err       string
	}{
		{
			name: "empty input",
			err:  "invalid trackID \"\"",
		},
		{
			name:    "invalid input",
			err:     "invalid trackID \"not_valid\"",
			trackID: "not_valid",
		},
		{
			name:      "valid",
			trackID:   "voice_sessiona_random",
			trackType: "voice",
			sessionID: "sessiona",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			trackType, sessionID, err := ParseTrackID(tc.trackID)
			if tc.err != "" {
				require.EqualError(t, err, tc.err)
			}
			require.Equal(t, tc.trackType, trackType)
			require.Equal(t, tc.sessionID, sessionID)
		})
	}
}
