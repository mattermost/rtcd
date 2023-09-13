// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

const pluginID = "com.mattermost.calls"

type CallJoinMessage struct {
	ChannelID string `json:"channelID"`
	JobID     string `json:"jobID"`
}

type CallReconnectMessage struct {
	ChannelID      string `json:"channelID"`
	OriginalConnID string `json:"originalConnID"`
	PrevConnID     string `json:"prevConnID"`
}

type CallJobState struct {
	InitAt  int64  `json:"init_at"`
	StartAt int64  `json:"start_at"`
	EndAt   int64  `json:"end_at"`
	Err     string `json:"err,omitempty"`
}

func (cjs *CallJobState) FromMap(m map[string]any) {
	cjs.InitAt, _ = m["init_at"].(int64)
	cjs.StartAt, _ = m["start_at"].(int64)
	cjs.EndAt, _ = m["end_at"].(int64)
	cjs.Err, _ = m["err"].(string)
}

const (
	TrackTypeVoice  = "voice"
	TrackTypeScreen = "screen"
)
