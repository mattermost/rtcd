// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/mattermost/rtcd/service/random"

	"github.com/gorilla/websocket"
	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

const (
	sendChSize    = 256
	receiveChSize = 256
	writeWaitTime = 10 * time.Second
)

type AuthCb func(w http.ResponseWriter, r *http.Request) (string, error)

type Server struct {
	cfg       ServerConfig
	log       mlog.LoggerIFace
	conns     map[string]*conn
	authCb    AuthCb
	mut       sync.RWMutex
	sendCh    chan Message
	receiveCh chan Message
}

// NewServer initializes and returns a new WebSocket server.
func NewServer(cfg ServerConfig, log mlog.LoggerIFace, opts ...ServerOption) (*Server, error) {
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

// SendCh returns a channel that can be used to send messages to ws connections.
func (s *Server) SendCh() chan<- Message {
	return s.sendCh
}

// ReceiveCh returns a channel that can be used to receive messages from ws connections.
func (s *Server) ReceiveCh() <-chan Message {
	return s.receiveCh
}

// Close stops the websocket server and closes all the ws connections.
// Must be called once all senders are done and cannot be called more than once.
func (s *Server) Close() {
	conns := s.getConns()
	for _, conn := range conns {
		if err := conn.close(); err != nil {
			s.log.Error("failed to close ws conn", mlog.Err(err))
		}
		<-conn.closeCh
	}
	close(s.receiveCh)
	close(s.sendCh)
}

// ServeHTTP makes the WebSocket server implement http.Handler so that it can
// be passed to a RegisterHandler method.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	connID := random.NewID()

	sendOpenMsg := func() {
		s.receiveCh <- newOpenMessage(connID)
	}
	sendCloseMsg := func() {
		s.receiveCh <- newCloseMessage(connID)
	}

	var err error
	var clientID string
	if s.authCb != nil {
		clientID, err = s.authCb(w, r)
		if err != nil {
			s.log.Error("authCb failed", mlog.Err(err))
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

	conn := newConn(connID, clientID, ws)
	defer conn.close()
	defer close(conn.closeCh)
	s.addConn(conn)

	sendOpenMsg()

	defer s.removeConn(conn.id)
	defer sendCloseMsg()

	ws.SetReadLimit(connMaxReadBytes)
	if err := ws.SetReadDeadline(time.Now().Add(2 * s.cfg.PingInterval)); err != nil {
		s.log.Error("failed to set read deadline", mlog.Err(err))
		return
	}
	ws.SetPongHandler(func(appData string) error {
		return ws.SetReadDeadline(time.Now().Add(2 * s.cfg.PingInterval))
	})

	for {
		mt, data, err := ws.ReadMessage()
		if err != nil {
			s.log.Error("ws read failed", mlog.Err(err))
			break
		}

		var msgType MessageType
		switch mt {
		case websocket.TextMessage:
			msgType = TextMessage
		case websocket.BinaryMessage:
			msgType = BinaryMessage
		default:
			s.log.Error("unexpected ws message", mlog.Int("type", mt))
			continue
		}

		s.receiveCh <- Message{
			ConnID: connID,
			Type:   msgType,
			Data:   data,
		}
	}
}

func (s *Server) connWriter() {
	pingTicker := time.NewTicker(s.cfg.PingInterval)
	defer pingTicker.Stop()

	for {
		select {
		case msg, ok := <-s.sendCh:
			if !ok {
				return
			}
			conn := s.getConn(msg.ConnID)
			if conn == nil {
				s.log.Error("failed to get conn for sending", mlog.String("connID", msg.ConnID))
				continue
			}

			var msgType int
			switch msg.Type {
			case TextMessage:
				msgType = websocket.TextMessage
			case BinaryMessage:
				msgType = websocket.BinaryMessage
			case CloseMessage:
				msgType = websocket.CloseMessage
			default:
				s.log.Error("unexpected ws message", mlog.Int("type", int(msg.Type)))
				continue
			}

			if err := conn.ws.SetWriteDeadline(time.Now().Add(writeWaitTime)); err != nil {
				s.log.Error("failed to set write deadline", mlog.String("connID", msg.ConnID), mlog.Err(err))
			}
			if err := conn.ws.WriteMessage(msgType, msg.Data); err != nil {
				s.log.Error("failed to write message", mlog.String("connID", msg.ConnID), mlog.Err(err))
			}
		case <-pingTicker.C:
			conns := s.getConns()
			for _, conn := range conns {
				if err := conn.ws.SetWriteDeadline(time.Now().Add(writeWaitTime)); err != nil {
					s.log.Error("failed to set write deadline", mlog.String("connID", conn.id), mlog.Err(err))
				}
				if err := conn.ws.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
					s.log.Error("failed to write ping message", mlog.String("connID", conn.id), mlog.Err(err))
				}
			}
		}
	}
}
