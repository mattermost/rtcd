// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

const (
	sendChSize    = 256
	receiveChSize = 256
)

type UpgradeCb func(connID string, w http.ResponseWriter, r *http.Request) error

type Server struct {
	cfg       Config
	log       *mlog.Logger
	conns     map[string]*conn
	upgradeCb UpgradeCb
	mut       sync.RWMutex
	sendCh    chan Message
	receiveCh chan Message
}

func NewServer(cfg Config, log *mlog.Logger, opts ...Option) (*Server, error) {
	if err := cfg.IsValid(); err != nil {
		return nil, fmt.Errorf("failed to validate config: %w", err)
	}
	s := &Server{
		cfg:       cfg,
		log:       log,
		conns:     make(map[string]*conn),
		sendCh:    make(chan Message, sendChSize),
		receiveCh: make(chan Message, receiveChSize),
	}

	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	go s.connWriter()

	return s, nil
}

func (s *Server) SendCh() chan<- Message {
	return s.sendCh
}

func (s *Server) ReceiveCh() <-chan Message {
	return s.receiveCh
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	connID := newID()

	sendOpenMsg := func() {
		s.receiveCh <- newOpenMessage(connID)
	}
	sendCloseMsg := func() {
		s.receiveCh <- newCloseMessage(connID)
	}

	if s.upgradeCb != nil {
		if err := s.upgradeCb(connID, w, r); err != nil {
			s.log.Error("upgradeCb failed", mlog.Err(err))
			return
		}
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  s.cfg.ReadBufferSize,
		WriteBufferSize: s.cfg.WriteBufferSize,
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Error("failed to upgrade connection", mlog.Err(err))
		sendCloseMsg()
		return
	}
	ws.SetReadLimit(connMaxReadBytes)

	conn := newConn(connID, ws)
	defer conn.Close()
	defer close(conn.closeCh)
	s.addConn(conn)

	sendOpenMsg()

	defer s.removeConn(conn.id)
	defer sendCloseMsg()

	for {
		mt, data, err := ws.ReadMessage()
		if err != nil {
			s.log.Error("ws read failed", mlog.Err(err))
			break
		}

		var msgType MessageType
		if mt == websocket.TextMessage {
			msgType = TextMessage
		} else if mt == websocket.BinaryMessage {
			msgType = BinaryMessage
		}
		s.receiveCh <- Message{
			ConnID: connID,
			Type:   msgType,
			Data:   data,
		}
	}
}

func (s *Server) Close() {
	conns := s.getConns()
	for _, conn := range conns {
		if err := conn.Close(); err != nil {
			s.log.Error("failed to close ws conn", mlog.Err(err))
		}
		<-conn.closeCh
	}
	close(s.receiveCh)
	close(s.sendCh)
}

func (s *Server) connWriter() {
	for msg := range s.sendCh {
		conn := s.getConn(msg.ConnID)
		if conn == nil {
			s.log.Error("failed to get conn for sending", mlog.String("connID", msg.ConnID))
			continue
		}

		var msgType int
		if msg.Type == TextMessage {
			msgType = websocket.TextMessage
		} else if msg.Type == BinaryMessage {
			msgType = websocket.BinaryMessage
		} else if msg.Type == CloseMessage {
			msgType = websocket.CloseMessage
		}

		if err := conn.ws.WriteMessage(msgType, msg.Data); err != nil {
			s.log.Error("failed to write message", mlog.String("connID", msg.ConnID), mlog.Err(err))
		}
	}
}
