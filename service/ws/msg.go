// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

// MessageType defines the type of message sent to or received from a ws
// connection.
type MessageType int

const (
	TextMessage MessageType = iota + 1
	BinaryMessage
	OpenMessage
	CloseMessage
)

// Message defines the data to be sent to or received from a ws connection.
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
