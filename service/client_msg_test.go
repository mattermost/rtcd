// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"testing"

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

	t.Run("with type", func(t *testing.T) {
		msg := NewClientMessage(ClientMessageSDP, nil)
		data, err := msg.Pack()
		require.NoError(t, err)
		msg2 := NewClientMessage(ClientMessageSDP, nil)
		err = msg2.Unpack(data)
		require.NoError(t, err)
		require.Equal(t, msg, msg2)
		require.Equal(t, ClientMessageSDP, msg2.Type)
	})

	t.Run("with type and data", func(t *testing.T) {
		msgData := map[string]interface{}{}
		msgData["sdp"] = `{"some": "data"}`
		msg := NewClientMessage(ClientMessageSDP, msgData)
		data, err := msg.Pack()
		require.NoError(t, err)
		msg2 := NewClientMessage(ClientMessageSDP, msgData)
		err = msg2.Unpack(data)
		require.NoError(t, err)
		require.Equal(t, msg, msg2)
		require.Equal(t, ClientMessageSDP, msg2.Type)
		require.Equal(t, msgData, msg2.Data)
	})
}
