// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"github.com/vmihailenco/msgpack/v5"
)

type ClientMessage struct {
	Type string                 `msgpack:"type"`
	Data map[string]interface{} `msgpack:"data"`
}

const (
	ClientMessageJoin      = "join"
	ClientMessageSDP       = "sdp"
	ClientMessageICE       = "ice"
	ClientMessageMute      = "mute"
	ClientMessageUnmute    = "unmute"
	ClientMessageScreenOn  = "screen_on"
	ClientMessageScreenOff = "screen_off"
)

func NewClientMessage(msgType string, data map[string]interface{}) *ClientMessage {
	return &ClientMessage{
		Type: msgType,
		Data: data,
	}
}

func (m *ClientMessage) Pack() ([]byte, error) {
	return msgpack.Marshal(m)
}

func (m *ClientMessage) Unpack(data []byte) error {
	return msgpack.Unmarshal(data, &m)
}
