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
	VoiceOnMessage
	VoiceOffMessage
)

type Message struct {
	GroupID   string      `msgpack:"group_id"`
	UserID    string      `msgpack:"user_id"`
	SessionID string      `msgpack:"session_id"`
	CallID    string      `msgpack:"call_id"`
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
		UserID:    s.cfg.UserID,
		SessionID: s.cfg.SessionID,
		CallID:    s.cfg.CallID,
		Type:      msgType,
		Data:      data,
	}
}

func marshalHostCandidate(c *webrtc.ICECandidate) webrtc.ICECandidateInit {
	val := c.Foundation
	if val == " " {
		val = ""
	}

	val = fmt.Sprintf("%s %d %s %d %s %d typ %s",
		val,
		c.Component,
		c.Protocol,
		c.Priority,
		c.Address,
		c.Port,
		c.Typ)

	if c.TCPType != "" {
		val += fmt.Sprintf(" tcptype %s", c.TCPType)
	}

	if c.RelatedAddress != "" && c.RelatedPort != 0 {
		val = fmt.Sprintf("%s raddr %s rport %d",
			val,
			c.RelatedAddress,
			c.RelatedPort)
	}

	return webrtc.ICECandidateInit{
		Candidate:     fmt.Sprintf("candidate:%s", val),
		SDPMid:        new(string),
		SDPMLineIndex: new(uint16),
	}
}

func newICEMessage(s *session, c *webrtc.ICECandidate) (Message, error) {
	data := make(map[string]interface{})
	data["type"] = "candidate"

	if c.Typ == webrtc.ICECandidateTypeHost && !isIPAddress(c.Address) {
		// If the address is not an IP, we assume it's a hostname (FQDN)
		// and pass it through as such.
		data["candidate"] = marshalHostCandidate(c)
	} else {
		data["candidate"] = c.ToJSON()
	}

	js, err := json.Marshal(data)
	if err != nil {
		return Message{}, err
	}
	return newMessage(s, ICEMessage, js), nil
}
