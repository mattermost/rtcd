// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

type Metrics interface {
	IncRTCSessions(groupID string, callID string)
	DecRTCSessions(groupID string, callID string)
	IncRTCConnState(state string)
	IncRTCErrors(groupID string, errType string)
	IncRTPTracks(groupID string, callID, direction, trackType string)
	DecRTPTracks(groupID string, callID, direction, trackType string)
}
