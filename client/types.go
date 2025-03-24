// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

const pluginID = "com.mattermost.calls"

type CallJoinMessage struct {
	ChannelID   string `json:"channelID"`
	JobID       string `json:"jobID"`
	AV1Support  bool   `json:"av1Support"`
	DCSignaling bool   `json:"dcSignaling"`
}

type CallReconnectMessage struct {
	ChannelID      string `json:"channelID"`
	OriginalConnID string `json:"originalConnID"`
	PrevConnID     string `json:"prevConnID"`
}

type CallJobState struct {
	Type    string `json:"type"`
	InitAt  int64  `json:"init_at"`
	StartAt int64  `json:"start_at"`
	EndAt   int64  `json:"end_at"`
	Err     string `json:"err,omitempty"`
}

func (cjs *CallJobState) FromMap(m map[string]any) {
	jobType, _ := m["type"].(string)
	initAt, _ := m["init_at"].(float64)
	startAt, _ := m["start_at"].(float64)
	endAt, _ := m["end_at"].(float64)
	err, _ := m["err"].(string)

	cjs.Type = jobType
	cjs.InitAt = int64(initAt)
	cjs.StartAt = int64(startAt)
	cjs.EndAt = int64(endAt)
	cjs.Err = err
}

const (
	TrackTypeVoice  = "voice"
	TrackTypeScreen = "screen"
	TrackTypeVideo  = "video"
)
