// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"net"
	"os"
	"testing"

	"github.com/mattermost/rtcd/service/api"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
	"github.com/stretchr/testify/require"
)

type TestHelper struct {
	srvc   *Service
	log    *mlog.Logger
	cfg    Config
	tb     testing.TB
	apiURL string
	dbDir  string
}

func SetupTestHelper(tb testing.TB) *TestHelper {
	tb.Helper()
	var err error

	dbDir, err := os.MkdirTemp("", "db")
	require.NoError(tb, err)

	th := &TestHelper{
		cfg: Config{
			API: api.Config{
				ListenAddress: ":0",
			},
			Store: StoreConfig{
				DataSource: dbDir,
			},
		},
		tb:    tb,
		dbDir: dbDir,
	}

	th.log, err = mlog.NewLogger()
	require.NoError(th.tb, err)

	th.srvc, err = New(th.cfg, th.log)
	require.NoError(th.tb, err)
	require.NotNil(th.tb, th.srvc)

	err = th.srvc.Start()
	require.NoError(th.tb, err)

	_, port, err := net.SplitHostPort(th.srvc.apiServer.Addr())
	require.NoError(th.tb, err)
	th.apiURL = "http://localhost:" + port

	return th
}

func (th *TestHelper) Teardown() {
	err := th.log.Shutdown()
	require.NoError(th.tb, err)

	err = th.srvc.Stop()
	require.NoError(th.tb, err)

	err = os.RemoveAll(th.dbDir)
	require.NoError(th.tb, err)
}
