// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

import (
	"github.com/gorilla/websocket"
)

const (
	connMaxReadBytes = 1024 * 1024 // 1MB
)

type conn struct {
	id       string
	clientID string
	ws       *websocket.Conn
	closeCh  chan struct{}
}

func newConn(id, clientID string, ws *websocket.Conn) *conn {
	return &conn{
		id:       id,
		clientID: clientID,
		ws:       ws,
		closeCh:  make(chan struct{}),
	}
}

func (c *conn) close() error {
	return c.ws.Close()
}

func (s *Server) addConn(c *conn) bool {
	if c == nil {
		return false
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	if _, ok := s.conns[c.id]; ok {
		return false
	}
	s.conns[c.id] = c
	return true
}

func (s *Server) removeConn(connID string) bool {
	s.mut.Lock()
	defer s.mut.Unlock()
	if _, ok := s.conns[connID]; !ok {
		return false
	}
	delete(s.conns, connID)
	return true
}

func (s *Server) getConn(connID string) *conn {
	s.mut.RLock()
	defer s.mut.RUnlock()

	if connID != "" {
		c := s.conns[connID]
		return c
	}

	return nil
}

func (s *Server) getConns() []*conn {
	s.mut.RLock()
	defer s.mut.RUnlock()
	var i int
	conns := make([]*conn, len(s.conns))
	for _, conn := range s.conns {
		conns[i] = conn
		i++
	}
	return conns
}
