// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"fmt"

	"github.com/mattermost/rtcd/service/rtc"

	"github.com/vmihailenco/msgpack/v5"
)

type ClientMessage struct {
	Type string `msgpack:"type"`
	Data any    `msgpack:"data,omitempty"`
}

const (
	ClientMessageJoin      = "join"
	ClientMessageLeave     = "leave"
	ClientMessageRTC       = "rtc"
	ClientMessageHello     = "hello"
	ClientMessageReconnect = "reconnect"
	ClientMessageClose     = "close"
	ClientMessageVAD       = "vad"
)

var _ msgpack.CustomEncoder = (*ClientMessage)(nil)

func (cm *ClientMessage) EncodeMsgpack(enc *msgpack.Encoder) error {
	return enc.EncodeMulti(cm.Type, cm.Data)
}

var _ msgpack.CustomDecoder = (*ClientMessage)(nil)

func (cm *ClientMessage) DecodeMsgpack(dec *msgpack.Decoder) error {
	msgType, err := dec.DecodeString()
	if err != nil {
		return fmt.Errorf("failed to decode msg.Type: %w", err)
	}
	cm.Type = msgType

	switch cm.Type {
	case ClientMessageJoin:
		data, err := dec.DecodeMap()
		if err != nil {
			return fmt.Errorf("failed to decode msg.Data: %w", err)
		}
		cm.Data = data
	case ClientMessageLeave, ClientMessageHello, ClientMessageReconnect, ClientMessageClose:
		data, err := dec.DecodeTypedMap()
		if err != nil {
			return fmt.Errorf("failed to decode msg.Data: %w", err)
		}
		cm.Data = data
	case ClientMessageRTC, ClientMessageVAD:
		var rtcMsg rtc.Message
		if err = dec.Decode(&rtcMsg); err != nil {
			return fmt.Errorf("failed to decode rtc.Message: %w", err)
		}
		cm.Data = rtcMsg
	default:
		data, err := dec.DecodeInterface()
		if err != nil {
			return fmt.Errorf("failed to decode msg.Data: %w", err)
		}
		cm.Data = data
	}

	return nil
}

func NewClientMessage(msgType string, data interface{}) *ClientMessage {
	return &ClientMessage{
		Type: msgType,
		Data: data,
	}
}

func NewPackedClientMessage(msgType string, data interface{}) ([]byte, error) {
	cm := NewClientMessage(msgType, data)
	packed, err := cm.Pack()
	if err != nil {
		return nil, fmt.Errorf("failed to pack client message: %s", err)
	}
	return packed, nil
}

func (cm *ClientMessage) Pack() ([]byte, error) {
	return msgpack.Marshal(&cm)
}

func (cm *ClientMessage) Unpack(data []byte) error {
	return msgpack.Unmarshal(data, &cm)
}
