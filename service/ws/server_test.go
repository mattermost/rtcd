// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

import (
	"net"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/mattermost/rtcd/service/random"

	"github.com/gorilla/websocket"
	"github.com/mattermost/mattermost-server/v6/shared/mlog"
	"github.com/stretchr/testify/require"
)

func setupClient(t *testing.T, serverAddr string, opts ...ClientOption) (*Client, func()) {
	t.Helper()

	_, port, err := net.SplitHostPort(serverAddr)
	require.NoError(t, err)
	u := url.URL{Scheme: "ws", Host: "localhost:" + port, Path: "/ws"}

	cfg := ClientConfig{
		URL: u.String(),
	}
	c, err := NewClient(cfg, opts...)
	require.NoError(t, err)
	require.NotNil(t, c)

	closeCh := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case err := <-c.ErrorCh():
			require.NoError(t, err)
		case <-closeCh:
			return
		}
	}()

	closeClient := func() {
		close(closeCh)
		wg.Wait()
		err := c.Close()
		require.NoError(t, err)
	}

	return c, closeClient
}

func setupServer(t *testing.T, opts ...ServerOption) (*Server, string, func()) {
	t.Helper()

	log, err := mlog.NewLogger()
	require.NoError(t, err)
	require.NotNil(t, log)

	cfg := ServerConfig{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		PingInterval:    time.Second,
	}

	s, err := NewServer(cfg, log, opts...)
	require.NoError(t, err)
	require.NotNil(t, s)

	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	require.NotNil(t, listener)
	go func() {
		_ = http.Serve(listener, s)
	}()

	return s, listener.Addr().String(), func() {
		s.Close()
		listener.Close()
		err := log.Shutdown()
		require.NoError(t, err)
	}
}

func TestNewServer(t *testing.T) {
	log, err := mlog.NewLogger()
	require.NoError(t, err)
	defer func() {
		err := log.Shutdown()
		require.NoError(t, err)
	}()

	t.Run("empty config", func(t *testing.T) {
		s, err := NewServer(ServerConfig{}, log)
		require.Error(t, err)
		require.Nil(t, s)
	})

	t.Run("valid config", func(t *testing.T) {
		cfg := ServerConfig{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			PingInterval:    time.Second,
		}
		s, err := NewServer(cfg, log)
		require.NoError(t, err)
		require.NotNil(t, s)
	})
}

func TestServeHTTP(t *testing.T) {
	upgradeRan := false

	authCb := func(w http.ResponseWriter, r *http.Request) (string, int, error) {
		upgradeRan = true
		return "", 0, nil
	}

	_, addr, shutdown := setupServer(t, WithAuthCb(authCb))
	defer shutdown()

	_, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)

	u := url.URL{Scheme: "ws", Host: "localhost:" + port, Path: "/ws"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.NoError(t, err)
	defer c.Close()

	require.True(t, upgradeRan)

	err = c.WriteMessage(websocket.TextMessage, []byte("some data"))
	require.NoError(t, err)
}

func TestAddRemoveConn(t *testing.T) {
	s, _, shutdown := setupServer(t)
	defer shutdown()
	defer func() {
		// cleanup
		s.mut.Lock()
		defer s.mut.Unlock()
		for id := range s.conns {
			delete(s.conns, id)
		}
	}()

	require.Empty(t, s.conns)

	t.Run("nil", func(t *testing.T) {
		ok := s.addConn(nil)
		require.False(t, ok)
		require.Empty(t, s.conns)
	})

	t.Run("duplicate", func(t *testing.T) {
		conn := newConn(random.NewID(), random.NewID(), &websocket.Conn{})
		ok := s.addConn(conn)
		require.True(t, ok)
		require.Len(t, s.conns, 1)
		require.NotNil(t, s.conns[conn.id])
		require.Equal(t, conn, s.conns[conn.id])

		ok = s.addConn(conn)
		require.False(t, ok)
		require.Len(t, s.conns, 1)
	})

	t.Run("multiple", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			conn := newConn(random.NewID(), random.NewID(), &websocket.Conn{})
			ok := s.addConn(conn)
			require.True(t, ok)
			require.Len(t, s.conns, i+2)
			require.NotNil(t, s.conns[conn.id])
			require.Equal(t, conn, s.conns[conn.id])
		}
	})
}

func TestRemoveConn(t *testing.T) {
	s, _, shutdown := setupServer(t)
	defer shutdown()

	require.Empty(t, s.conns)

	t.Run("missing", func(t *testing.T) {
		ok := s.removeConn("")
		require.False(t, ok)
		ok = s.removeConn(random.NewID())
		require.False(t, ok)
	})

	t.Run("remove", func(t *testing.T) {
		conn := newConn(random.NewID(), random.NewID(), &websocket.Conn{})
		ok := s.addConn(conn)
		require.True(t, ok)
		require.Len(t, s.conns, 1)
		require.NotNil(t, s.conns[conn.id])
		require.Equal(t, conn, s.conns[conn.id])

		ok = s.removeConn(conn.id)
		require.True(t, ok)
		require.Empty(t, s.conns)

		ok = s.removeConn(conn.id)
		require.False(t, ok)
		require.Empty(t, s.conns)
	})
}

func TestGetConn(t *testing.T) {
	s, _, shutdown := setupServer(t)
	defer shutdown()

	require.Empty(t, s.conns)

	t.Run("missing", func(t *testing.T) {
		c := s.getConn(random.NewID())
		require.Nil(t, c)
	})

	t.Run("added", func(t *testing.T) {
		conn := newConn(random.NewID(), random.NewID(), &websocket.Conn{})
		ok := s.addConn(conn)
		require.True(t, ok)
		require.Len(t, s.conns, 1)
		require.NotNil(t, s.conns[conn.id])
		require.Equal(t, conn, s.conns[conn.id])

		c := s.getConn(conn.id)
		require.NotNil(t, c)
		require.Equal(t, conn, c)

		ok = s.removeConn(c.id)
		require.True(t, ok)
		require.Empty(t, s.conns)
	})

	t.Run("removed", func(t *testing.T) {
		c := s.getConn(random.NewID())
		require.Nil(t, c)

		conn := newConn(random.NewID(), random.NewID(), &websocket.Conn{})
		ok := s.addConn(conn)
		require.True(t, ok)
		require.Len(t, s.conns, 1)
		require.NotNil(t, s.conns[conn.id])
		require.Equal(t, conn, s.conns[conn.id])

		ok = s.removeConn(conn.id)
		require.True(t, ok)
		require.Empty(t, s.conns)

		c = s.getConn(random.NewID())
		require.Nil(t, c)
	})
}

func TestGetConns(t *testing.T) {
	s, _, shutdown := setupServer(t)
	defer shutdown()
	defer func() {
		// cleanup
		s.mut.Lock()
		defer s.mut.Unlock()
		for id := range s.conns {
			delete(s.conns, id)
		}
	}()

	conns := s.getConns()
	require.Empty(t, conns)

	conn1 := newConn(random.NewID(), random.NewID(), &websocket.Conn{})
	ok := s.addConn(conn1)
	require.True(t, ok)
	require.Len(t, s.conns, 1)
	require.NotNil(t, s.conns[conn1.id])
	require.Equal(t, conn1, s.conns[conn1.id])

	conn2 := newConn(random.NewID(), random.NewID(), &websocket.Conn{})
	ok = s.addConn(conn2)
	require.True(t, ok)
	require.Len(t, s.conns, 2)
	require.NotNil(t, s.conns[conn2.id])
	require.Equal(t, conn2, s.conns[conn2.id])

	conn3 := newConn(random.NewID(), random.NewID(), &websocket.Conn{})
	ok = s.addConn(conn3)
	require.True(t, ok)
	require.Len(t, s.conns, 3)
	require.NotNil(t, s.conns[conn3.id])
	require.Equal(t, conn3, s.conns[conn3.id])

	conns = s.getConns()
	require.Equal(t, len(s.conns), len(conns))
	require.ElementsMatch(t, []*conn{conn1, conn2, conn3}, conns)
}

func TestWithAuthCb(t *testing.T) {
	s, _, shutdown := setupServer(t)
	defer shutdown()
	require.Nil(t, s.authCb)

	authCb := func(w http.ResponseWriter, r *http.Request) (string, int, error) {
		return "", 0, nil
	}

	s2, _, shutdown2 := setupServer(t, WithAuthCb(authCb))
	defer shutdown2()
	require.NotNil(t, s2.authCb)
}

func TestReceiveMessages(t *testing.T) {
	s, addr, shutdown := setupServer(t)
	defer shutdown()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		c, closeClient := setupClient(t, addr)
		defer closeClient()
		err := c.conn.ws.WriteMessage(websocket.TextMessage, []byte("conn1 data"))
		require.NoError(t, err)
	}()

	go func() {
		defer wg.Done()
		c, closeClient := setupClient(t, addr)
		defer closeClient()
		err := c.conn.ws.WriteMessage(websocket.TextMessage, []byte("conn2 data"))
		require.NoError(t, err)
	}()

	wg.Wait()

	var msgs []Message
	msgs = append(msgs, <-s.ReceiveCh())
	msgs = append(msgs, <-s.ReceiveCh())
	msgs = append(msgs, <-s.ReceiveCh())
	msgs = append(msgs, <-s.ReceiveCh())

	require.Len(t, msgs, 4)
}

func TestRaceReceiveClose(t *testing.T) {
	server, addr, shutdown := setupServer(t)

	var wg sync.WaitGroup
	wg.Add(3)

	setupClient := func(t *testing.T, addr string) *Client {
		_, port, err := net.SplitHostPort(addr)
		require.NoError(t, err)
		u := url.URL{Scheme: "ws", Host: "localhost:" + port, Path: "/ws"}

		cfg := ClientConfig{
			URL: u.String(),
		}
		c, err := NewClient(cfg)
		require.NoError(t, err)
		require.NotNil(t, c)

		return c
	}

	c1 := setupClient(t, addr)
	go func() {
		defer wg.Done()
		defer c1.Close()
		for i := 0; i < 100; i++ {
			_ = c1.Send(TextMessage, []byte("conn data"))
		}
	}()

	c2 := setupClient(t, addr)
	go func() {
		defer wg.Done()
		defer c2.Close()
		for i := 0; i < 100; i++ {
			_ = c2.Send(TextMessage, []byte("conn data"))
		}
	}()

	go func() {
		defer wg.Done()
		server.Close()
		shutdown()
	}()

	wg.Wait()
}

func TestRaceConnectClose(t *testing.T) {
	_, addr, shutdown := setupServer(t)
	setupClient := func(t *testing.T, addr string) *Client {
		_, port, err := net.SplitHostPort(addr)
		require.NoError(t, err)
		u := url.URL{Scheme: "ws", Host: "localhost:" + port, Path: "/ws"}

		cfg := ClientConfig{
			URL: u.String(),
		}
		c, _ := NewClient(cfg)
		return c
	}

	var wg sync.WaitGroup
	wg.Add(11)

	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			c := setupClient(t, addr)
			if c != nil {
				c.Close()
			}
		}()
	}

	go func() {
		defer wg.Done()
		shutdown()
	}()

	wg.Wait()
}

func TestSendMessages(t *testing.T) {
	s, addr, shutdown := setupServer(t)
	defer shutdown()

	var wg sync.WaitGroup
	wg.Add(2)

	c, closeClient := setupClient(t, addr)

	openMsg := <-s.ReceiveCh()
	require.Equal(t, OpenMessage, openMsg.Type)

	conns := s.getConns()
	require.NotEmpty(t, conns)
	require.NotEmpty(t, conns[0].id)

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			s.sendCh <- Message{
				ConnID: conns[0].id,
				Data:   []byte("some data"),
				Type:   TextMessage,
			}
		}
	}()

	go func() {
		defer wg.Done()
		defer closeClient()
		var received int
		for msg := range c.ReceiveCh() {
			require.Equal(t, []byte("some data"), msg.Data)
			received++
			if received == 100 {
				break
			}
		}
		require.Equal(t, 100, received)
	}()

	wg.Wait()
}

func TestRaceSendClose(t *testing.T) {
	s, _, shutdown := setupServer(t)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = s.Send(Message{
				ConnID: "connID",
				Data:   []byte("some data"),
				Type:   TextMessage,
			})
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			s.Close()
		}
		shutdown()
	}()

	wg.Wait()

	require.EqualError(t, s.Send(Message{}), "server is closed")
}
