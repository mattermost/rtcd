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
	ClientID string
	ConnID   string
	Type     MessageType
	Data     []byte
}

func newOpenMessage(connID, clientID string) Message {
	return Message{
		ClientID: clientID,
		ConnID:   connID,
		Type:     OpenMessage,
	}
}

func newCloseMessage(connID, clientID string) Message {
	return Message{
		ClientID: clientID,
		ConnID:   connID,
		Type:     CloseMessage,
	}
}
