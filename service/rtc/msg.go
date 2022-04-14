// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"encoding/json"
	"fmt"

	"github.com/pion/webrtc/v3"
)

type MessageType int

const (
	ICEMessage MessageType = iota + 1
	SDPMessage
	MuteMessage
	UnmuteMessage
	ScreenOnMessage
	ScreenOffMessage
)

type Message struct {
	GroupID   string      `msgpack:"group_id"`
	SessionID string      `msgpack:"session_id"`
	Type      MessageType `msgpack:"type"`
	Data      []byte      `msgpack:"data,omitempty"`
}

func (m *Message) IsValid() error {
	if m.SessionID == "" {
		return fmt.Errorf("invalid SessionID value: should not be empty")
	}
	if m.Type == 0 {
		return fmt.Errorf("invalid Type value")
	}

	return nil
}

func newMessage(s *session, msgType MessageType, data []byte) Message {
	return Message{
		GroupID:   s.cfg.GroupID,
		SessionID: s.cfg.SessionID,
		Type:      ICEMessage,
		Data:      data,
	}
}

func newICEMessage(s *session, c *webrtc.ICECandidate) (Message, error) {
	data := make(map[string]interface{})
	data["type"] = "candidate"
	data["candidate"] = c.ToJSON()
	js, err := json.Marshal(data)
	if err != nil {
		return Message{}, err
	}
	return newMessage(s, ICEMessage, js), nil
}
