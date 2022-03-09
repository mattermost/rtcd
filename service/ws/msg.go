// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

type MessageType int

const (
	TextMessage MessageType = iota + 1
	BinaryMessage
	OpenMessage
	CloseMessage
)

type Message struct {
	ConnID string
	Type   MessageType
	Data   []byte
}

func newOpenMessage(connID string) Message {
	return Message{
		ConnID: connID,
		Type:   OpenMessage,
	}
}

func newCloseMessage(connID string) Message {
	return Message{
		ConnID: connID,
		Type:   CloseMessage,
	}
}
