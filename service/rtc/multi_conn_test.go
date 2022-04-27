// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"context"
	"net"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/stretchr/testify/require"
)

func TestNewMultiConn(t *testing.T) {
	t.Run("error - nil conns", func(t *testing.T) {
		mc, err := newMultiConn(nil)
		require.Error(t, err)
		require.Equal(t, "conns should not be empty", err.Error())
		require.Nil(t, mc)

	})

	t.Run("error - empty conns", func(t *testing.T) {
		mc, err := newMultiConn([]net.PacketConn{})
		require.Error(t, err)
		require.Equal(t, "conns should not be empty", err.Error())
		require.Nil(t, mc)
	})

	t.Run("error - nil conn", func(t *testing.T) {
		mc, err := newMultiConn([]net.PacketConn{nil})
		require.Error(t, err)
		require.Equal(t, "invalid nil conn", err.Error())
		require.Nil(t, mc)
	})

	t.Run("success", func(t *testing.T) {
		var listenConfig net.ListenConfig
		conn1, err := listenConfig.ListenPacket(context.Background(), "udp4", ":0")
		require.NoError(t, err)
		require.NotNil(t, conn1)
		mc, err := newMultiConn([]net.PacketConn{conn1})
		require.NoError(t, err)
		require.NotNil(t, mc)
		err = mc.Close()
		require.NoError(t, err)
		err = conn1.Close()
		require.Error(t, err)
		require.Contains(t, err.Error(), "use of closed network connection")
	})
}

func TestMultiConnReadWrite(t *testing.T) {
	listenConfig := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			return c.Control(func(fd uintptr) {
				err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
				require.NoError(t, err)
				err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
				require.NoError(t, err)
			})
		},
	}

	conn1, err := listenConfig.ListenPacket(context.Background(), "udp4", ":0")
	require.NoError(t, err)
	require.NotNil(t, conn1)
	conn2, err := listenConfig.ListenPacket(context.Background(), "udp4", conn1.LocalAddr().String())
	require.NoError(t, err)
	require.NotNil(t, conn2)
	require.Equal(t, conn1.LocalAddr(), conn2.LocalAddr())

	mc, err := newMultiConn([]net.PacketConn{conn1, conn2})
	require.NoError(t, err)
	require.NotNil(t, mc)
	defer mc.Close()

	conn1Data := []byte("conn1 data")
	_, err = conn1.WriteTo(conn1Data, mc.LocalAddr())
	require.NoError(t, err)
	receivedData := make([]byte, receiveMTU)
	read, _, err := mc.ReadFrom(receivedData)
	require.NoError(t, err)
	require.Equal(t, conn1Data, receivedData[:read])

	conn2Data := []byte("conn2 data")
	_, err = conn1.WriteTo(conn2Data, mc.LocalAddr())
	require.NoError(t, err)
	read, _, err = mc.ReadFrom(receivedData)
	require.NoError(t, err)
	require.Equal(t, conn2Data, receivedData[:read])

	require.Equal(t, uint64(0), mc.counter)
	_, err = mc.WriteTo(conn1Data, conn1.LocalAddr())
	require.NoError(t, err)
	require.Equal(t, uint64(1), mc.counter)
	_, err = mc.WriteTo(conn2Data, conn2.LocalAddr())
	require.NoError(t, err)
	require.Equal(t, uint64(2), mc.counter)
}
