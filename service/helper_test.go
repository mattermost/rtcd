// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"net"
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
}

func SetupTestHelper(tb testing.TB) *TestHelper {
	var th TestHelper
	var err error

	th.cfg = Config{
		API: api.Config{
			ListenAddress: ":0",
		},
	}

	th.log, err = mlog.NewLogger()
	require.NoError(th.tb, err)

	th.srvc, err = New(th.cfg, th.log)
	require.NoError(th.tb, err)

	err = th.srvc.Start()
	require.NoError(th.tb, err)

	_, port, err := net.SplitHostPort(th.srvc.apiServer.Addr())
	require.NoError(th.tb, err)
	th.apiURL = "http://localhost:" + port

	return &th
}

func (th *TestHelper) Teardown() {
	err := th.log.Shutdown()
	require.NoError(th.tb, err)

	err = th.srvc.Stop()
	require.NoError(th.tb, err)
}
