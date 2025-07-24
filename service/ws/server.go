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
	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

const (
	ReceiveChSize = 5000
	sendChSize    = ReceiveChSize
	writeWaitTime = 10 * time.Second
)

type AuthCb func(w http.ResponseWriter, r *http.Request) (string, int, error)

type Server struct {
	cfg       ServerConfig
	log       mlog.LoggerIFace
	conns     map[string]*conn
	authCb    AuthCb
	mut       sync.RWMutex
	sendCh    chan Message
	receiveCh chan Message
	closed    bool
}

// NewServer initializes and returns a new WebSocket server.
func NewServer(cfg ServerConfig, log mlog.LoggerIFace, opts ...ServerOption) (*Server, error) {
	if err := cfg.IsValid(); err != nil {
		return nil, fmt.Errorf("ws: failed to validate config: %w", err)
	}

	s := &Server{
		cfg:       cfg,
		log:       log,
		conns:     make(map[string]*conn),
		sendCh:    make(chan Message, sendChSize),
		receiveCh: make(chan Message, ReceiveChSize),
	}

	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, fmt.Errorf("ws: failed to apply option: %w", err)
		}
	}

	go s.connWriter()

	return s, nil
}

// SendCh queues a message to be sent through a ws connection.
func (s *Server) Send(msg Message) error {
	s.mut.RLock()
	defer s.mut.RUnlock()

	if s.closed {
		return fmt.Errorf("ws: server is closed")
	}

	select {
	case s.sendCh <- msg:
	default:
		return fmt.Errorf("ws: failed to send message, channel is full")
	}
	return nil
}

// ReceiveCh returns a channel that can be used to receive messages from ws connections.
func (s *Server) ReceiveCh() <-chan Message {
	return s.receiveCh
}

// Close stops the websocket server and closes all the ws connections.
// Must be called once all senders are done.
func (s *Server) Close() {
	s.mut.Lock()
	if s.closed {
		s.mut.Unlock()
		return
	}
	s.closed = true
	s.mut.Unlock()

	conns := s.getConns()
	for _, conn := range conns {
		if err := conn.close(); err != nil {
			s.log.Error("ws: failed to close conn", mlog.Err(err))
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

	sendOpenMsg := func(clientID string) {
		s.receiveCh <- newOpenMessage(connID, clientID)
	}
	sendCloseMsg := func(clientID string) {
		s.receiveCh <- newCloseMessage(connID, clientID)
	}

	var err error
	var clientID string
	if s.authCb != nil {
		clientID, _, err = s.authCb(w, r)
		if err != nil {
			s.log.Error("ws: auth callback failed", mlog.Err(err))
			return
		}
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  s.cfg.ReadBufferSize,
		WriteBufferSize: s.cfg.WriteBufferSize,
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Error("ws: failed to upgrade connection", mlog.Err(err))
		sendCloseMsg(clientID)
		return
	}

	conn := newConn(connID, clientID, ws)
	defer conn.close()
	defer close(conn.closeCh)
	s.addConn(conn)

	if s.isClosed() {
		return
	}

	sendOpenMsg(clientID)

	defer s.removeConn(conn.id)
	defer sendCloseMsg(clientID)

	ws.SetReadLimit(connMaxReadBytes)
	if err := ws.SetReadDeadline(time.Now().Add(2 * s.cfg.PingInterval)); err != nil {
		s.log.Error("ws: failed to set read deadline", mlog.Err(err))
		return
	}
	ws.SetPongHandler(func(_ string) error {
		return ws.SetReadDeadline(time.Now().Add(2 * s.cfg.PingInterval))
	})

	for {
		mt, data, err := ws.ReadMessage()
		if err != nil {
			s.log.Error("ws: read failed", mlog.Err(err))
			break
		}

		var msgType MessageType
		switch mt {
		case websocket.TextMessage:
			msgType = TextMessage
		case websocket.BinaryMessage:
			msgType = BinaryMessage
		default:
			s.log.Error("ws: unexpected message", mlog.Int("type", mt))
			continue
		}

		s.receiveCh <- Message{
			ConnID:   connID,
			ClientID: conn.clientID,
			Type:     msgType,
			Data:     data,
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
				s.log.Error("ws: failed to get conn for sending", mlog.String("connID", msg.ConnID), mlog.String("clientID", msg.ClientID))
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
				s.log.Error("ws: unexpected message", mlog.Int("type", int(msg.Type)))
				continue
			}

			if err := conn.ws.SetWriteDeadline(time.Now().Add(writeWaitTime)); err != nil {
				s.log.Error("ws: failed to set write deadline", mlog.String("connID", msg.ConnID), mlog.Err(err))
			}
			if err := conn.ws.WriteMessage(msgType, msg.Data); err != nil {
				s.log.Error("ws: failed to write message", mlog.String("connID", msg.ConnID), mlog.Err(err))
			}
		case <-pingTicker.C:
			conns := s.getConns()
			for _, conn := range conns {
				if err := conn.ws.SetWriteDeadline(time.Now().Add(writeWaitTime)); err != nil {
					s.log.Error("ws: failed to set write deadline", mlog.String("connID", conn.id), mlog.Err(err))
				}
				if err := conn.ws.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
					s.log.Error("ws: failed to write ping message", mlog.String("connID", conn.id), mlog.Err(err))
				}
			}
		}
	}
}

func (s *Server) isClosed() bool {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.closed
}
