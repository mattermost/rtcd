// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServerConfigIsValid(t *testing.T) {
	t.Run("empty struct", func(t *testing.T) {
		var cfg ServerConfig
		err := cfg.IsValid()
		require.Error(t, err)
	})

	t.Run("invalid ICEPortUDP", func(t *testing.T) {
		var cfg ServerConfig
		cfg.ICEPortUDP = 22
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid ICEPortUDP value: 22 is not in allowed range [80, 49151]", err.Error())
		cfg.ICEPortUDP = 65000
		err = cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid ICEPortUDP value: 65000 is not in allowed range [80, 49151]", err.Error())
	})

	t.Run("valid", func(t *testing.T) {
		var cfg ServerConfig
		cfg.ICEPortUDP = 8443
		err := cfg.IsValid()
		require.NoError(t, err)
	})
}

func TestSessionConfigIsValid(t *testing.T) {
	t.Run("empty struct", func(t *testing.T) {
		var cfg SessionConfig
		err := cfg.IsValid()
		require.Error(t, err)
	})

	t.Run("invalid GroupID", func(t *testing.T) {
		var cfg SessionConfig
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid GroupID value: should not be empty", err.Error())
	})

	t.Run("invalid CallID", func(t *testing.T) {
		var cfg SessionConfig
		cfg.GroupID = "groupID"
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid CallID value: should not be empty", err.Error())
	})

	t.Run("invalid UserID", func(t *testing.T) {
		var cfg SessionConfig
		cfg.GroupID = "groupID"
		cfg.CallID = "callID"
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid UserID value: should not be empty", err.Error())
	})

	t.Run("invalid ConnID", func(t *testing.T) {
		var cfg SessionConfig
		cfg.GroupID = "groupID"
		cfg.CallID = "callID"
		cfg.UserID = "userID"
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid SessionID value: should not be empty", err.Error())
	})

	t.Run("valid", func(t *testing.T) {
		var cfg SessionConfig
		cfg.GroupID = "groupID"
		cfg.CallID = "callID"
		cfg.UserID = "userID"
		cfg.SessionID = "sessionID"
		err := cfg.IsValid()
		require.NoError(t, err)
	})
}
