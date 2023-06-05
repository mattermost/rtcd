// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateAddrsPairs(t *testing.T) {
	t.Run("nil/empty inputs", func(t *testing.T) {
		pairs, err := generateAddrsPairs(nil, nil, "", false)
		require.NoError(t, err)
		require.Empty(t, pairs)

		pairs, err = generateAddrsPairs([]netip.Addr{}, map[netip.Addr]string{}, "", false)
		require.NoError(t, err)
		require.Empty(t, pairs)
	})

	t.Run("no public addresses", func(t *testing.T) {
		pairs, err := generateAddrsPairs([]netip.Addr{
			netip.MustParseAddr("127.0.0.1"),
			netip.MustParseAddr("10.1.1.1"),
		}, map[netip.Addr]string{
			netip.MustParseAddr("127.0.0.1"): "",
			netip.MustParseAddr("10.1.1.1"):  "",
		}, "", false)
		require.NoError(t, err)
		require.Equal(t, []string{"127.0.0.1/127.0.0.1", "10.1.1.1/10.1.1.1"}, pairs)
	})

	t.Run("full NAT mapping", func(t *testing.T) {
		pairs, err := generateAddrsPairs([]netip.Addr{
			netip.MustParseAddr("127.0.0.1"),
			netip.MustParseAddr("10.1.1.1"),
		}, map[netip.Addr]string{}, "1.1.1.1/127.0.0.1,1.1.1.1/10.1.1.1", false)
		require.NoError(t, err)
		require.Equal(t, []string{"1.1.1.1/127.0.0.1", "1.1.1.1/10.1.1.1"}, pairs)
	})

	t.Run("no public addresses with override", func(t *testing.T) {
		pairs, err := generateAddrsPairs([]netip.Addr{
			netip.MustParseAddr("127.0.0.1"),
			netip.MustParseAddr("10.1.1.1"),
		}, map[netip.Addr]string{
			netip.MustParseAddr("127.0.0.1"): "",
			netip.MustParseAddr("10.1.1.1"):  "",
		}, "1.1.1.1", false)
		require.NoError(t, err)
		require.Equal(t, []string{"127.0.0.1/127.0.0.1", "1.1.1.1/10.1.1.1"}, pairs)
	})

	t.Run("single public address for multiple local addrs, no override", func(t *testing.T) {
		pairs, err := generateAddrsPairs([]netip.Addr{
			netip.MustParseAddr("127.0.0.1"),
			netip.MustParseAddr("10.1.1.1"),
		}, map[netip.Addr]string{
			netip.MustParseAddr("127.0.0.1"): "",
			netip.MustParseAddr("10.1.1.1"):  "1.1.1.1",
		}, "", false)
		require.NoError(t, err)
		require.Equal(t, []string{"127.0.0.1/127.0.0.1", "1.1.1.1/10.1.1.1"}, pairs)
	})

	t.Run("single local/public address map, no override", func(t *testing.T) {
		pairs, err := generateAddrsPairs([]netip.Addr{
			netip.MustParseAddr("127.0.0.1"),
			netip.MustParseAddr("10.1.1.1"),
		}, map[netip.Addr]string{
			netip.MustParseAddr("127.0.0.1"): "",
			netip.MustParseAddr("10.1.1.1"):  "1.1.1.1",
		}, "", false)
		require.NoError(t, err)
		require.Equal(t, []string{"127.0.0.1/127.0.0.1", "1.1.1.1/10.1.1.1"}, pairs)
	})

	t.Run("multiple public addresses, no override", func(t *testing.T) {
		pairs, err := generateAddrsPairs([]netip.Addr{
			netip.MustParseAddr("127.0.0.1"),
			netip.MustParseAddr("10.1.1.1"),
		}, map[netip.Addr]string{
			netip.MustParseAddr("127.0.0.1"): "1.1.1.1",
			netip.MustParseAddr("10.1.1.1"):  "1.1.1.2",
		}, "", false)
		require.NoError(t, err)
		require.Equal(t, []string{"1.1.1.1/127.0.0.1", "1.1.1.2/10.1.1.1"}, pairs)
	})

	// This is not a case that would happen in the application because the
	// override would prevent us from finding public IPs.
	t.Run("multiple public addresses, with override", func(t *testing.T) {
		pairs, err := generateAddrsPairs([]netip.Addr{
			netip.MustParseAddr("127.0.0.1"),
			netip.MustParseAddr("10.1.1.1"),
		}, map[netip.Addr]string{
			netip.MustParseAddr("127.0.0.1"): "1.1.1.1",
			netip.MustParseAddr("10.1.1.1"):  "1.1.1.2",
		}, "8.8.8.8", false)
		require.NoError(t, err)
		require.Equal(t, []string{"127.0.0.1/127.0.0.1", "8.8.8.8/10.1.1.1"}, pairs)
	})
}
