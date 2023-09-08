// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

type Metrics interface {
	IncRTCSessions(groupID string)
	DecRTCSessions(groupID string)
	IncRTCConnState(state string)
	IncRTCErrors(groupID string, errType string)
	IncRTPTracks(groupID string, direction, trackType string)
	DecRTPTracks(groupID string, direction, trackType string)
}
