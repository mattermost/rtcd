// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

type Metrics interface {
	IncRTCSessions(groupID string, callID string)
	DecRTCSessions(groupID string, callID string)
	IncRTCConnState(state string)
	IncRTPPackets(direction, trackType string)
	AddRTPPacketBytes(direction, trackType string, value int)
}
