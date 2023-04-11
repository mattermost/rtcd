// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"errors"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	receiveMTU = 1460
)

type multiConn struct {
	conns        []net.PacketConn
	addr         net.Addr
	readResultCh chan readResult
	closeCh      chan struct{}
	bufPool      *sync.Pool
	counter      uint64
	wg           sync.WaitGroup
}

type readResult struct {
	n    int
	addr net.Addr
	err  error
	buf  []byte
}

func newMultiConn(conns []net.PacketConn) (*multiConn, error) {
	if len(conns) == 0 {
		return nil, errors.New("conns should not be empty")
	}
	for _, conn := range conns {
		if conn == nil {
			return nil, errors.New("invalid nil conn")
		}
	}
	var mc multiConn
	mc.conns = conns
	mc.addr = conns[0].LocalAddr()
	mc.readResultCh = make(chan readResult, len(conns)*2)
	mc.closeCh = make(chan struct{})
	mc.bufPool = &sync.Pool{
		New: func() interface{} {
			return make([]byte, receiveMTU)
		},
	}
	mc.wg.Add(len(conns))
	for _, conn := range conns {
		go mc.reader(conn)
	}
	return &mc, nil
}

func (mc *multiConn) reader(conn net.PacketConn) {
	defer mc.wg.Done()
	var res readResult
	for {
		res.buf = mc.bufPool.Get().([]byte)
		res.n, res.addr, res.err = conn.ReadFrom(res.buf)
		select {
		case mc.readResultCh <- res:
		case <-mc.closeCh:
			return
		}
		if os.IsTimeout(res.err) {
			continue
		} else if res.err != nil {
			break
		}
	}
}

func (mc *multiConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	res := <-mc.readResultCh
	copy(p, res.buf[:res.n])
	mc.bufPool.Put(res.buf)
	return res.n, res.addr, res.err
}

func (mc *multiConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	// Simple round-robin to equally distribute the writes among the connections.
	idx := (atomic.AddUint64(&mc.counter, 1) - 1) % uint64(len(mc.conns))
	return mc.conns[idx].WriteTo(p, addr)
}

func (mc *multiConn) Close() error {
	var err error
	close(mc.closeCh)
	for _, conn := range mc.conns {
		err = conn.Close()
	}
	mc.wg.Wait()
	close(mc.readResultCh)
	return err
}

func (mc *multiConn) LocalAddr() net.Addr {
	return mc.addr
}

func (mc *multiConn) SetDeadline(t time.Time) error {
	var err error
	for _, conn := range mc.conns {
		err = conn.SetDeadline(t)
	}
	return err
}

func (mc *multiConn) SetReadDeadline(t time.Time) error {
	var err error
	for _, conn := range mc.conns {
		err = conn.SetReadDeadline(t)
	}
	return err
}

func (mc *multiConn) SetWriteDeadline(t time.Time) error {
	var err error
	for _, conn := range mc.conns {
		err = conn.SetWriteDeadline(t)
	}
	return err
}
