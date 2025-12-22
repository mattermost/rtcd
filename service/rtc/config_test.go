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

	t.Run("invalid ICEAddressUDP", func(t *testing.T) {
		var cfg ServerConfig
		cfg.ICEAddressUDP = "not_an_address"
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid ICEAddressUDP value: invalid ICEAddress value: not_an_address is not a valid IP address", err.Error())

		cfg.ICEAddressUDP = "127.0.0.0.1"
		err = cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid ICEAddressUDP value: invalid ICEAddress value: 127.0.0.0.1 is not a valid IP address", err.Error())
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

	t.Run("invalid ICEPortTCP", func(t *testing.T) {
		var cfg ServerConfig
		cfg.ICEPortUDP = 8443
		cfg.ICEPortTCP = 22
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid ICEPortTCP value: 22 is not in allowed range [80, 49151]", err.Error())
		cfg.ICEPortTCP = 65000
		err = cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid ICEPortTCP value: 65000 is not in allowed range [80, 49151]", err.Error())
	})

	t.Run("invalid TURNCredentialsExpirationMinutes", func(t *testing.T) {
		var cfg ServerConfig
		cfg.ICEPortUDP = 8443
		cfg.ICEPortTCP = 8443
		cfg.TURNConfig.StaticAuthSecret = "secret"
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid TURNConfig: invalid CredentialsExpirationMinutes value: should be a positive number", err.Error())

		cfg.TURNConfig.CredentialsExpirationMinutes = 20000
		err = cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid TURNConfig: invalid CredentialsExpirationMinutes value: should be less than 1 week", err.Error())
	})

	t.Run("invalid ICEHostPortOverride", func(t *testing.T) {
		t.Run("single port", func(t *testing.T) {
			var cfg ServerConfig
			cfg.ICEPortUDP = 8443
			cfg.ICEPortTCP = 8443
			cfg.ICEHostPortOverride = "45"
			err := cfg.IsValid()
			require.Error(t, err)
			require.Equal(t, "invalid ICEHostPortOverride value: 45 is not in allowed range [80, 49151]", err.Error())
			cfg.ICEHostPortOverride = "65000"
			err = cfg.IsValid()
			require.Error(t, err)
			require.Equal(t, "invalid ICEHostPortOverride value: 65000 is not in allowed range [80, 49151]", err.Error())
		})

		t.Run("mapping", func(t *testing.T) {
			var cfg ServerConfig
			cfg.ICEPortUDP = 8443
			cfg.ICEPortTCP = 8443
			cfg.ICEHostPortOverride = "127.0.0.1,8443"
			err := cfg.IsValid()
			require.Error(t, err)
			require.Equal(t, "invalid ICEHostPortOverride value: failed to parse mapping: invalid map pairing syntax", err.Error())
		})
	})

	t.Run("invalid UDPSocketsCount", func(t *testing.T) {
		var cfg ServerConfig
		cfg.ICEPortUDP = 8443
		cfg.ICEPortTCP = 8443
		cfg.UDPSocketsCount = 0
		err := cfg.IsValid()
		require.EqualError(t, err, "invalid UDPSocketsCount value: should be greater than 0")
	})

	t.Run("invalid NACKBufferSize too small", func(t *testing.T) {
		var cfg ServerConfig
		cfg.ICEPortUDP = 8443
		cfg.ICEPortTCP = 8443
		cfg.UDPSocketsCount = 1
		cfg.NACKBufferSize = 16
		err := cfg.IsValid()
		require.EqualError(t, err, "invalid NACKBufferSize value: should be at least 32")
	})

	t.Run("invalid NACKBufferSize too large", func(t *testing.T) {
		var cfg ServerConfig
		cfg.ICEPortUDP = 8443
		cfg.ICEPortTCP = 8443
		cfg.UDPSocketsCount = 1
		cfg.NACKBufferSize = 16384
		err := cfg.IsValid()
		require.EqualError(t, err, "invalid NACKBufferSize value: should not exceed 8192")
	})

	t.Run("invalid NACKBufferSize not power of 2", func(t *testing.T) {
		var cfg ServerConfig
		cfg.ICEPortUDP = 8443
		cfg.ICEPortTCP = 8443
		cfg.UDPSocketsCount = 1
		cfg.NACKBufferSize = 100
		err := cfg.IsValid()
		require.EqualError(t, err, "invalid NACKBufferSize value: must be a power of 2 (32, 64, 128, 256, 512, 1024, 2048, 4096, 8192)")
	})

	t.Run("valid", func(t *testing.T) {
		var cfg ServerConfig
		cfg.ICEAddressUDP = "127.0.0.1"
		cfg.ICEPortUDP = 8443
		cfg.ICEPortTCP = 8443
		cfg.TURNConfig.CredentialsExpirationMinutes = 1440
		cfg.UDPSocketsCount = 1
		cfg.NACKBufferSize = 256
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
				URLs: []string{"stuns:stun1.localhost", "stun:stun2.localhost"},
			},
		}
		url := servers.getSTUN()
		require.Equal(t, "stuns:stun1.localhost", url)
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

	t.Run("valid, secured", func(t *testing.T) {
		cfg := ICEServerConfig{
			URLs: []string{
				"turns:localhost:3478",
			},
		}
		err := cfg.IsValid()
		require.NoError(t, err)
	})
}

func TestICEHostPortOverrideParseMap(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var override *ICEHostPortOverride
		m, err := override.ParseMap()
		require.EqualError(t, err, "should not be nil")
		require.Nil(t, m)
	})

	t.Run("empty", func(t *testing.T) {
		var override ICEHostPortOverride
		m, err := override.ParseMap()
		require.NoError(t, err)
		require.Nil(t, m)
	})

	t.Run("duplicate addresses", func(t *testing.T) {
		override := ICEHostPortOverride("127.0.0.1/8444,127.0.0.1/8445")
		m, err := override.ParseMap()
		require.EqualError(t, err, "duplicate mapping found for 127.0.0.1")
		require.Nil(t, m)
	})

	t.Run("duplicate ports", func(t *testing.T) {
		override := ICEHostPortOverride("127.0.0.1/8444,127.0.0.2/8444")
		m, err := override.ParseMap()
		require.EqualError(t, err, "duplicate port found for 8444")
		require.Nil(t, m)
	})

	t.Run("valid mapping", func(t *testing.T) {
		override := ICEHostPortOverride("127.0.0.1/8443,127.0.0.2/8445,127.0.0.3/8444")
		m, err := override.ParseMap()
		require.NoError(t, err)
		require.Equal(t, map[string]int{
			"127.0.0.1": 8443,
			"127.0.0.2": 8445,
			"127.0.0.3": 8444,
		}, m)
	})
}

func TestSessionConfigFromMap(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		var cfg *SessionConfig
		err := cfg.FromMap(map[string]any{})
		require.EqualError(t, err, "invalid nil config")
	})

	t.Run("nil map", func(t *testing.T) {
		var cfg SessionConfig
		err := cfg.FromMap(nil)
		require.EqualError(t, err, "invalid nil map")
	})

	t.Run("missing props", func(t *testing.T) {
		var cfg SessionConfig
		err := cfg.FromMap(map[string]any{
			"callID":    "callID",
			"sessionID": "sessionID",
			"groupID":   "groupID",
			"userID":    "userID",
		})
		require.NoError(t, err)
		require.Equal(t, SessionConfig{
			GroupID:   "groupID",
			SessionID: "sessionID",
			UserID:    "userID",
			CallID:    "callID",
			Props: SessionProps{
				"channelID":   nil,
				"av1Support":  nil,
				"dcSignaling": nil,
			},
		}, cfg)
	})

	t.Run("complete", func(t *testing.T) {
		var cfg SessionConfig
		err := cfg.FromMap(map[string]any{
			"callID":      "callID",
			"sessionID":   "sessionID",
			"groupID":     "groupID",
			"userID":      "userID",
			"channelID":   "channelID",
			"av1Support":  true,
			"dcSignaling": true,
		})
		require.NoError(t, err)
		require.NoError(t, cfg.IsValid())
		require.Equal(t, SessionConfig{
			GroupID:   "groupID",
			SessionID: "sessionID",
			UserID:    "userID",
			CallID:    "callID",
			Props: SessionProps{
				"channelID":   "channelID",
				"av1Support":  true,
				"dcSignaling": true,
			},
		}, cfg)
	})
}

func TestSessionProps(t *testing.T) {
	t.Run("empty props", func(t *testing.T) {
		cfg := SessionConfig{
			Props: SessionProps{},
		}
		require.Empty(t, cfg.Props.ChannelID())
		require.False(t, cfg.Props.AV1Support())
	})

	t.Run("complete props", func(t *testing.T) {
		cfg := SessionConfig{
			Props: SessionProps{
				"channelID":  "channelID",
				"av1Support": true,
			},
		}
		require.Equal(t, "channelID", cfg.Props.ChannelID())
		require.True(t, cfg.Props.AV1Support())
	})
}

func TestICEAddressParse(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var addr ICEAddress
		addrs := addr.Parse()
		require.Nil(t, addrs)
	})

	t.Run("single address", func(t *testing.T) {
		addr := ICEAddress("127.0.0.1")
		addrs := addr.Parse()
		require.Equal(t, []string{"127.0.0.1"}, addrs)
	})

	t.Run("multiple addresses", func(t *testing.T) {
		addr := ICEAddress("127.0.0.1,192.168.1.1")
		addrs := addr.Parse()
		require.Equal(t, []string{"127.0.0.1", "192.168.1.1"}, addrs)
	})

	t.Run("multiple addresses with spaces", func(t *testing.T) {
		addr := ICEAddress("127.0.0.1 , 192.168.1.1   ,  10.0.0.1")
		addrs := addr.Parse()
		require.Equal(t, []string{"127.0.0.1", "192.168.1.1", "10.0.0.1"}, addrs)
	})

	t.Run("single address with spaces", func(t *testing.T) {
		addr := ICEAddress("  127.0.0.1  ")
		addrs := addr.Parse()
		require.Equal(t, []string{"127.0.0.1"}, addrs)
	})

	t.Run("IPv6 addresses", func(t *testing.T) {
		addr := ICEAddress("::1,2001:db8::1")
		addrs := addr.Parse()
		require.Equal(t, []string{"::1", "2001:db8::1"}, addrs)
	})
}

func TestICEAddressIsValid(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var addr ICEAddress
		err := addr.IsValid()
		require.NoError(t, err)
	})

	t.Run("valid single address", func(t *testing.T) {
		addr := ICEAddress("127.0.0.1")
		err := addr.IsValid()
		require.NoError(t, err)
	})

	t.Run("valid multiple addresses", func(t *testing.T) {
		addr := ICEAddress("127.0.0.1,192.168.45.45")
		err := addr.IsValid()
		require.NoError(t, err)
	})

	t.Run("valid multiple addresses, with extra spaces", func(t *testing.T) {
		addr := ICEAddress("127.0.0.1 , 192.168.45.45   ,  10.8.8.8")
		err := addr.IsValid()
		require.NoError(t, err)
	})

	t.Run("invalid address", func(t *testing.T) {
		addr := ICEAddress("127.0.0.256")
		err := addr.IsValid()
		require.EqualError(t, err, "invalid ICEAddress value: 127.0.0.256 is not a valid IP address")
	})

	t.Run("invalid address in list", func(t *testing.T) {
		addr := ICEAddress("127.0.0.1,192.168.45.45,256.45.45.45")
		err := addr.IsValid()
		require.EqualError(t, err, "invalid ICEAddress value: 256.45.45.45 is not a valid IP address")
	})
}
