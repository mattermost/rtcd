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

	t.Run("invalid TURNCredentialsExpirationMinutes", func(t *testing.T) {
		var cfg ServerConfig
		cfg.ICEPortUDP = 8443
		cfg.TURNStaticAuthSecret = "secret"
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid TURNCredentialsExpirationMinutes value: should be a positive number", err.Error())
	})

	t.Run("valid", func(t *testing.T) {
		var cfg ServerConfig
		cfg.ICEPortUDP = 8443
		cfg.TURNCredentialsExpirationMinutes = 1440
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

func TestGetSTUN(t *testing.T) {
	t.Run("empty struct", func(t *testing.T) {
		var servers ICEServers
		url := servers.getSTUN()
		require.Empty(t, url)
	})

	t.Run("no STUN", func(t *testing.T) {
		servers := ICEServers{
			ICEServerConfig{
				URLs: []string{"turn:localhost"},
			},
			ICEServerConfig{
				URLs: []string{"turn:localhost"},
			},
		}
		url := servers.getSTUN()
		require.Empty(t, url)
	})

	t.Run("single STUN", func(t *testing.T) {
		servers := ICEServers{
			ICEServerConfig{
				URLs: []string{"turn:localhost"},
			},
			ICEServerConfig{
				URLs: []string{"stun:localhost"},
			},
		}
		url := servers.getSTUN()
		require.Equal(t, "stun:localhost", url)
	})

	t.Run("multiple STUN", func(t *testing.T) {
		servers := ICEServers{
			ICEServerConfig{
				URLs: []string{"turn:localhost"},
			},
			ICEServerConfig{
				URLs: []string{"stun:stun1.localhost", "stun:stun2.localhost"},
			},
		}
		url := servers.getSTUN()
		require.Equal(t, "stun:stun1.localhost", url)
	})
}

func TestICEServerConfigIsValid(t *testing.T) {
	t.Run("empty struct", func(t *testing.T) {
		var cfg ICEServerConfig
		err := cfg.IsValid()
		require.Error(t, err)
	})

	t.Run("empty URLS", func(t *testing.T) {
		var cfg ICEServerConfig
		cfg.URLs = []string{}
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid empty URLs", err.Error())
	})

	t.Run("empty URL", func(t *testing.T) {
		cfg := ICEServerConfig{
			URLs: []string{
				"",
			},
		}
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid empty URL", err.Error())
	})

	t.Run("invalid server", func(t *testing.T) {
		cfg := ICEServerConfig{
			URLs: []string{
				"localhost:3478",
			},
		}
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "URL is not a valid STUN/TURN server", err.Error())
	})

	t.Run("partially valid", func(t *testing.T) {
		cfg := ICEServerConfig{
			URLs: []string{
				"turn:turn1.localhost:3478",
				"turn2.localhost:3478",
			},
		}
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "URL is not a valid STUN/TURN server", err.Error())
	})

	t.Run("valid", func(t *testing.T) {
		cfg := ICEServerConfig{
			URLs: []string{
				"stun:localhost:3478",
			},
		}
		err := cfg.IsValid()
		require.NoError(t, err)
	})
}
