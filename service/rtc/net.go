// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"runtime"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

const (
	udpSocketBufferSize      = 1024 * 1024 * 16 // 16MB
	tcpConnReadBufferLength  = 64
	tcpSocketWriteBufferSize = 1024 * 1024 * 4 // 4MB
)

// getSystemIPs returns a list of all the available local addresses.
func getSystemIPs(log mlog.LoggerIFace, dualStack bool) ([]netip.Addr, error) {
	var ips []netip.Addr

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get system interfaces: %w", err)
	}

	for _, iface := range interfaces {
		// filter out inactive interfaces
		if iface.Flags&net.FlagUp == 0 {
			log.Info("skipping inactive interface", mlog.String("interface", iface.Name))
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			log.Warn("failed to get addresses for interface", mlog.String("interface", iface.Name))
			continue
		}

		for _, addr := range addrs {
			prefix, err := netip.ParsePrefix(addr.String())
			if err != nil {
				log.Warn("failed to parse prefix", mlog.Err(err), mlog.String("prefix", prefix.String()))
				continue
			}

			ip := prefix.Addr()

			if !dualStack && ip.Is6() {
				log.Debug("ignoring IPv6 address: dual stack support is disabled by config", mlog.String("addr", ip.String()))
				continue
			}

			if ip.Is6() && !ip.IsGlobalUnicast() {
				log.Debug("ignoring non global IPv6 address", mlog.String("addr", ip.String()))
				continue
			}

			ips = append(ips, ip)
		}
	}

	return ips, nil
}

func createUDPConnsForAddr(log mlog.LoggerIFace, network, listenAddress string) ([]net.PacketConn, error) {
	var conns []net.PacketConn

	for i := 0; i < runtime.NumCPU(); i++ {
		listenConfig := net.ListenConfig{
			Control: func(network, address string, c syscall.RawConn) error {
				return c.Control(func(fd uintptr) {
					err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1)
					if err != nil {
						log.Error("failed to set reuseaddr option", mlog.Err(err))
						return
					}
					err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
					if err != nil {
						log.Error("failed to set reuseport option", mlog.Err(err))
						return
					}
				})
			},
		}

		udpConn, err := listenConfig.ListenPacket(context.Background(), network, listenAddress)
		if err != nil {
			return nil, fmt.Errorf("failed to listen on udp: %w", err)
		}

		log.Info(fmt.Sprintf("rtc: server is listening on udp %s", listenAddress))

		if err := udpConn.(*net.UDPConn).SetWriteBuffer(udpSocketBufferSize); err != nil {
			log.Warn("rtc: failed to set udp send buffer", mlog.Err(err))
		}

		if err := udpConn.(*net.UDPConn).SetReadBuffer(udpSocketBufferSize); err != nil {
			log.Warn("rtc: failed to set udp receive buffer", mlog.Err(err))
		}

		connFile, err := udpConn.(*net.UDPConn).File()
		if err != nil {
			return nil, fmt.Errorf("failed to get udp conn file: %w", err)
		}
		defer connFile.Close()

		sysConn, err := connFile.SyscallConn()
		if err != nil {
			return nil, fmt.Errorf("failed to get syscall conn: %w", err)
		}
		err = sysConn.Control(func(fd uintptr) {
			writeBufSize, err := syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF)
			if err != nil {
				log.Error("failed to get buffer size", mlog.Err(err))
				return
			}
			readBufSize, err := syscall.GetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF)
			if err != nil {
				log.Error("failed to get buffer size", mlog.Err(err))
				return
			}
			log.Debug("rtc: udp buffers", mlog.Int("writeBufSize", writeBufSize), mlog.Int("readBufSize", readBufSize))
		})
		if err != nil {
			return nil, fmt.Errorf("Control call failed: %w", err)
		}

		conns = append(conns, udpConn)
	}

	return conns, nil
}

func resolveHost(host, network string, timeout time.Duration) (string, error) {
	var ip string
	r := net.Resolver{}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	addrs, err := r.LookupIP(ctx, network, host)
	if err != nil {
		return ip, fmt.Errorf("failed to resolve host %q: %w", host, err)
	}
	if len(addrs) > 0 {
		ip = addrs[0].String()
	}
	return ip, err
}

func areAddressesSameStack(addrA, addrB netip.Addr) bool {
	return (addrA.Is4() && addrB.Is4()) || (addrA.Is6() && addrB.Is6())
}
