// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGenTURNCredentials(t *testing.T) {
	t.Run("empty username", func(t *testing.T) {
		ts := time.Now().Add(30 * time.Minute).Unix()
		username, password, err := genTURNCredentials("", "secret", ts)
		require.EqualError(t, err, "username should not be empty")
		require.Empty(t, username)
		require.Empty(t, password)
	})

	t.Run("empty secret", func(t *testing.T) {
		ts := time.Now().Add(30 * time.Minute).Unix()
		username, password, err := genTURNCredentials("username", "", ts)
		require.EqualError(t, err, "secret should not be empty")
		require.Empty(t, username)
		require.Empty(t, password)
	})

	t.Run("invalid timestamp", func(t *testing.T) {
		username, password, err := genTURNCredentials("username", "secret", 0)
		require.EqualError(t, err, "expirationTS should be a positive number")
		require.Empty(t, username)
		require.Empty(t, password)
	})

	t.Run("expiration > 1 week", func(t *testing.T) {
		ts := time.Now().Add(20000 * time.Minute).Unix()
		username, password, err := genTURNCredentials("username", "secret", ts)
		require.EqualError(t, err, "expirationTS cannot be more than a week into the future")
		require.Empty(t, username)
		require.Empty(t, password)
	})

	t.Run("valid", func(t *testing.T) {
		ts := time.Now().Add(30 * time.Minute).Unix()
		username, password, err := genTURNCredentials("username", "secret", ts)
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("%d:username", ts), username)
		require.NotEmpty(t, password)
	})
}

func TestGenTURNConfigs(t *testing.T) {
	t.Run("no servers", func(t *testing.T) {
		configs, err := GenTURNConfigs(nil, "", "", 0)
		require.NoError(t, err)
		require.Empty(t, configs)

		configs, err = GenTURNConfigs(ICEServers{}, "", "", 0)
		require.NoError(t, err)
		require.Empty(t, configs)
	})

	t.Run("static credentials", func(t *testing.T) {
		servers := ICEServers{
			ICEServerConfig{
				URLs:       []string{"turn:turn1.example.com:3478"},
				Username:   "username",
				Credential: "password",
			},
		}
		configs, err := GenTURNConfigs(servers, "", "", 0)
		require.NoError(t, err)
		require.Empty(t, configs)
	})

	t.Run("turn servers", func(t *testing.T) {
		servers := ICEServers{
			ICEServerConfig{
				URLs: []string{"turn:turn1.example.com:3478"},
			},
			ICEServerConfig{
				URLs: []string{"turn:turn2.example.com:3478"},
			},
		}
		configs, err := GenTURNConfigs(servers, "username", "secret", 1440)
		require.NoError(t, err)
		require.Len(t, configs, 2)
		require.Len(t, configs[0].URLs, 1)
		require.Len(t, configs[1].URLs, 1)
		require.Equal(t, "turn:turn1.example.com:3478", configs[0].URLs[0])
		require.NotEmpty(t, configs[0].Username)
		require.NotEmpty(t, configs[0].Credential)
		require.Equal(t, "turn:turn2.example.com:3478", configs[1].URLs[0])
		require.NotEmpty(t, configs[1].Username)
		require.NotEmpty(t, configs[1].Credential)
	})
}
