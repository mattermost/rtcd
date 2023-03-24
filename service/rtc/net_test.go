// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"runtime"
	"testing"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"

	"github.com/stretchr/testify/require"
)

func TestGetSystemIPs(t *testing.T) {
	log, err := mlog.NewLogger()
	require.NoError(t, err)
	defer func() {
		err := log.Shutdown()
		require.NoError(t, err)
	}()

	ips, err := getSystemIPs(log)
	require.NoError(t, err)
	require.NotEmpty(t, ips)
}

func TestCreateUDPConnsForAddr(t *testing.T) {
	log, err := mlog.NewLogger()
	require.NoError(t, err)
	defer func() {
		err := log.Shutdown()
		require.NoError(t, err)
	}()

	ips, err := getSystemIPs(log)
	require.NoError(t, err)
	require.NotEmpty(t, ips)

	for _, ip := range ips {
		conns, err := createUDPConnsForAddr(log, ip+":30443")
		require.NoError(t, err)
		require.Len(t, conns, runtime.NumCPU())
		for _, conn := range conns {
			require.NoError(t, conn.Close())
		}
	}
}
