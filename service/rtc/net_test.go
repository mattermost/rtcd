// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"net/netip"
	"os"
	"testing"

	"github.com/mattermost/mattermost/server/public/shared/mlog"

	"github.com/stretchr/testify/require"
)

func TestGetSystemIPs(t *testing.T) {
	log, err := mlog.NewLogger()
	require.NoError(t, err)
	defer func() {
		err := log.Shutdown()
		require.NoError(t, err)
	}()

	t.Run("ipv4", func(t *testing.T) {
		ips, err := getSystemIPs(log, false)
		require.NoError(t, err)
		require.NotEmpty(t, ips)

		for _, ip := range ips {
			require.True(t, ip.Is4())
		}
	})

	t.Run("dual stack", func(t *testing.T) {
		// Skipping this test in CI since IPv6 is not yet supported by Github actions.
		if os.Getenv("CI") != "" {
			t.Skip()
		}

		ips, err := getSystemIPs(log, true)
		require.NoError(t, err)
		require.NotEmpty(t, ips)

		var hasIPv4 bool
		var hasIPv6 bool
		for _, ip := range ips {
			if ip.Is4() {
				hasIPv4 = true
			}
			if ip.Is6() {
				hasIPv6 = true
			}
		}

		require.True(t, hasIPv4)
		require.True(t, hasIPv6)
	})
}

func TestCreateUDPConnsForAddr(t *testing.T) {
	log, err := mlog.NewLogger()
	require.NoError(t, err)
	defer func() {
		err := log.Shutdown()
		require.NoError(t, err)
	}()

	t.Run("IPv4", func(t *testing.T) {
		ips, err := getSystemIPs(log, false)
		require.NoError(t, err)
		require.NotEmpty(t, ips)

		for _, ip := range ips {
			conns, err := createUDPConnsForAddr(log, "udp4", netip.AddrPortFrom(ip, 30443).String())
			require.NoError(t, err)
			require.Len(t, conns, getUDPListeningSocketsCount())
			for _, conn := range conns {
				require.NoError(t, conn.Close())
			}
		}
	})

	t.Run("dual stack", func(t *testing.T) {
		// Skipping this test in CI since IPv6 is not yet supported by Github actions.
		if os.Getenv("CI") != "" {
			t.Skip()
		}

		ips, err := getSystemIPs(log, false)
		require.NoError(t, err)
		require.NotEmpty(t, ips)

		for _, ip := range ips {
			conns, err := createUDPConnsForAddr(log, "udp", netip.AddrPortFrom(ip, 30443).String())
			require.NoError(t, err)
			require.Len(t, conns, getUDPListeningSocketsCount())
			for _, conn := range conns {
				require.NoError(t, conn.Close())
			}
		}
	})
}
