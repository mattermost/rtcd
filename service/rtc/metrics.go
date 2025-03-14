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
	ObserveRTPTracksWrite(groupID, trackType string, dur float64)
	ObserveRTCConnectionTime(groupID string, dur float64)
	ObserveRTCDataChannelOpenTime(groupID string, dur float64)
	ObserveRTCSignalingLockGrabTime(groupID string, dur float64)
	ObserveRTCSignalingLockLockedTime(groupID string, dur float64)

	// Client metrics
	ObserveRTCClientLossRate(groupID string, val float64)
	ObserveRTCClientRTT(groupID string, val float64)
	ObserveRTCClientJitter(groupID string, val float64)
}
