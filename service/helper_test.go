// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"net"
	"os"
	"testing"

	"github.com/mattermost/rtcd/logger"
	"github.com/mattermost/rtcd/service/api"
	"github.com/mattermost/rtcd/service/rtc"

	"github.com/stretchr/testify/require"
)

type TestHelper struct {
	srvc        *Service
	adminClient *Client
	cfg         Config
	tb          testing.TB
	apiURL      string
	dbDir       string
}

func SetupTestHelper(tb testing.TB) *TestHelper {
	tb.Helper()
	var err error

	dbDir, err := os.MkdirTemp("", "db")
	require.NoError(tb, err)

	th := &TestHelper{
		cfg: Config{
			API: APIConfig{
				HTTP: api.Config{
					ListenAddress: ":0",
				},
				Admin: AdminConfig{
					Enable:    true,
					SecretKey: "admin_secret_key",
				},
			},
			RTC: rtc.ServerConfig{
				ICEPortUDP: 30443,
			},
			Store: StoreConfig{
				DataSource: dbDir,
			},
			Logger: logger.Config{
				EnableConsole: true,
				ConsoleLevel:  "ERROR",
			},
		},
		tb:    tb,
		dbDir: dbDir,
	}

	th.srvc, err = New(th.cfg)
	require.NoError(th.tb, err)
	require.NotNil(th.tb, th.srvc)

	err = th.srvc.Start()
	require.NoError(th.tb, err)

	_, port, err := net.SplitHostPort(th.srvc.apiServer.Addr())
	require.NoError(th.tb, err)
	th.apiURL = "http://localhost:" + port

	th.adminClient, err = NewClient(ClientConfig{
		URL:     th.apiURL,
		AuthKey: th.srvc.cfg.API.Admin.SecretKey,
	})
	require.NoError(th.tb, err)
	require.NotNil(th.tb, th.adminClient)

	return th
}

func (th *TestHelper) Teardown() {
	err := th.srvc.Stop()
	require.NoError(th.tb, err)

	err = os.RemoveAll(th.dbDir)
	require.NoError(th.tb, err)

	err = th.adminClient.Close()
	require.NoError(th.tb, err)
}
