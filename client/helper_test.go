// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mattermost/rtcd/service/random"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/stretchr/testify/require"
)

type TestHelper struct {
	tb             testing.TB
	apiURL         string
	adminAPIClient *model.Client4
	userAPIClient  *model.Client4
	adminClient    *Client
	userClient     *Client
}

func ensureUser(tb testing.TB, client *model.Client4, username, password string) {
	tb.Helper()

	apiRequestTimeout := 10 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), apiRequestTimeout)
	defer cancel()
	_, _, err := client.Login(ctx, username, password)
	if err != nil {
		_, _, err = client.CreateUser(ctx, &model.User{
			Username: username,
			Password: password,
			Email:    fmt.Sprintf("%s@example.com", username),
		})
		require.Nil(tb, err)
		_, _, err = client.Login(ctx, username, password)
		require.Nil(tb, err)
	}
}

func setupTestHelper(tb testing.TB, channelID string) *TestHelper {
	tb.Helper()
	var err error

	th := &TestHelper{
		tb: tb,
	}

	if channelID == "" {
		channelID = random.NewID()
	}

	th.apiURL = "http://localhost:8065"

	adminName := "sysadmin"
	adminPass := "Sys@dmin-sample1"

	userName := "calls-user0"
	userPass := "U$er-sample1"

	th.adminAPIClient = model.NewAPIv4Client(th.apiURL)
	th.userAPIClient = model.NewAPIv4Client(th.apiURL)

	ensureUser(tb, th.adminAPIClient, adminName, adminPass)
	ensureUser(tb, th.userAPIClient, userName, userPass)

	th.adminClient, err = New(Config{
		SiteURL:   th.apiURL,
		AuthToken: th.adminAPIClient.AuthToken,
		ChannelID: channelID,
	})
	require.NoError(tb, err)
	require.NotNil(tb, th.adminClient)

	th.userClient, err = New(Config{
		SiteURL:   th.apiURL,
		AuthToken: th.userAPIClient.AuthToken,
		ChannelID: channelID,
	})
	require.NoError(tb, err)
	require.NotNil(tb, th.userClient)

	return th
}
