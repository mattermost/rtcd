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
	Session SessionConfig
	Type    MessageType
	Data    []byte
}

func (m *Message) IsValid() error {
	if err := m.Session.IsValid(); err != nil {
		return err
	}
	if m.Type == 0 {
		return fmt.Errorf("invalid Type value")
	}

	return nil
}

func newMessage(s *session, msgType MessageType, data []byte) Message {
	return Message{
		Session: s.cfg,
		Type:    ICEMessage,
		Data:    data,
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
