// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"fmt"
	"strings"
)

// ParseTrackID returns the track type and session ID for the given
// track ID.
func ParseTrackID(trackID string) (string, string, error) {
	fields := strings.Split(trackID, "_")
	if len(fields) < 3 {
		return "", "", fmt.Errorf("invalid trackID %q", trackID)
	}

	return fields[0], fields[1], nil
}
