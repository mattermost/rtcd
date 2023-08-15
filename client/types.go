// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

const pluginID = "com.mattermost.calls"

type CallJoinMessage struct {
	ChannelID string `json:"channelID"`
	ContextID string `json:"contextID"`
}

type CallReconnectMessage struct {
	ChannelID      string `json:"channelID"`
	OriginalConnID string `json:"originalConnID"`
	PrevConnID     string `json:"prevConnID"`
}

const (
	TrackTypeVoice  = "voice"
	TrackTypeScreen = "screen"
)
