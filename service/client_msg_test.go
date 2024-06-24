// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"testing"

	"github.com/mattermost/rtcd/service/rtc"

	"github.com/stretchr/testify/require"
)

func TestClientMessage(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		msg := NewClientMessage("", nil)
		data, err := msg.Pack()
		require.NoError(t, err)
		msg2 := NewClientMessage("", nil)
		err = msg2.Unpack(data)
		require.NoError(t, err)
		require.Equal(t, msg, msg2)
	})

	t.Run("with join type", func(t *testing.T) {
		msgData := map[string]any{
			"connID": "conn_id",
		}
		msg := NewClientMessage(ClientMessageJoin, msgData)
		data, err := msg.Pack()
		require.NoError(t, err)
		msg2 := NewClientMessage(ClientMessageJoin, msgData)
		err = msg2.Unpack(data)
		require.NoError(t, err)
		require.Equal(t, msg, msg2)
		require.Equal(t, ClientMessageJoin, msg2.Type)
	})

	t.Run("with leave type", func(t *testing.T) {
		msgData := map[string]string{
			"sessionID": "session_id",
		}
		msg := NewClientMessage(ClientMessageLeave, msgData)
		data, err := msg.Pack()
		require.NoError(t, err)
		msg2 := NewClientMessage(ClientMessageLeave, msgData)
		err = msg2.Unpack(data)
		require.NoError(t, err)
		require.Equal(t, msg, msg2)
		require.Equal(t, ClientMessageLeave, msg2.Type)
	})

	t.Run("with rtc type", func(t *testing.T) {
		rtcMsg := rtc.Message{
			SessionID: "session_id",
			GroupID:   "group_id",
			Type:      rtc.SDPMessage,
			Data:      []byte(`sdp data`),
		}
		msg := NewClientMessage(ClientMessageRTC, rtcMsg)
		data, err := msg.Pack()
		require.NoError(t, err)
		msg2 := &ClientMessage{}
		err = msg2.Unpack(data)
		require.NoError(t, err)
		require.Equal(t, msg, msg2)
		require.Equal(t, ClientMessageRTC, msg2.Type)
		require.Equal(t, rtcMsg, msg2.Data)
	})
}
