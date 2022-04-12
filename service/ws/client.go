// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mattermost/rtcd/service/random"

	"github.com/gorilla/websocket"
)

const (
	wsConnClosed int32 = iota
	wsConnOpen
	wsConnClosing
)

type Client struct {
	cfg       ClientConfig
	conn      *conn
	sendCh    chan Message
	receiveCh chan Message
	errorCh   chan error
	wg        sync.WaitGroup
	connState int32
}

// NewClient initializes and returns a new WebSocket client.
func NewClient(cfg ClientConfig, opts ...ClientOption) (*Client, error) {
	if err := cfg.IsValid(); err != nil {
		return nil, fmt.Errorf("failed to validate config: %w", err)
	}

	header := http.Header{
		"Authorization": []string{"Basic " + cfg.AuthToken},
	}

	ws, _, err := websocket.DefaultDialer.Dial(cfg.URL, header)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %w", err)
	}

	connID := cfg.ConnID
	if connID == "" {
		connID = random.NewID()
	}
	conn := newConn(connID, "", ws)

	c := &Client{
		cfg:       cfg,
		conn:      conn,
		sendCh:    make(chan Message, sendChSize),
		receiveCh: make(chan Message, receiveChSize),
		errorCh:   make(chan error),
	}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	c.wg.Add(2)
	go c.connReader()
	go c.connWriter()
	c.setConnState(wsConnOpen)

	return c, nil
}

func (c *Client) connReader() {
	defer func() {
		close(c.receiveCh)
		c.wg.Done()
		close(c.conn.closeCh)
		c.wg.Wait()
		close(c.errorCh)
		c.setConnState(wsConnClosed)
	}()

	c.conn.ws.SetReadLimit(connMaxReadBytes)

	for {
		mt, data, err := c.conn.ws.ReadMessage()
		if err != nil {
			c.sendError(fmt.Errorf("failed to read message: %w", err))
			return
		}

		var msgType MessageType
		switch mt {
		case websocket.TextMessage:
			msgType = TextMessage
		case websocket.BinaryMessage:
			msgType = BinaryMessage
		default:
			c.sendError(fmt.Errorf("unexpected message type: %d", msgType))
			continue
		}

		c.receiveCh <- Message{
			Type: msgType,
			Data: data,
		}
	}
}

func (c *Client) connWriter() {
	defer c.wg.Done()

	for {
		select {
		case msg := <-c.sendCh:
			msgType := websocket.TextMessage
			if msg.Type == BinaryMessage {
				msgType = websocket.BinaryMessage
			}
			if err := c.conn.ws.SetWriteDeadline(time.Now().Add(writeWaitTime)); err != nil {
				c.sendError(fmt.Errorf("failed to set write deadline: %w", err))
			}
			if err := c.conn.ws.WriteMessage(msgType, msg.Data); err != nil {
				c.sendError(fmt.Errorf("failed to write message: %w", err))
			}
		case <-c.conn.closeCh:
			return
		}
	}
}

func (c *Client) sendError(err error) {
	if c.getConnState() != wsConnOpen {
		return
	}
	select {
	case c.errorCh <- err:
	default:
	}
}

// SendMsg sends a WebSocket message with the specified type and data.
func (c *Client) Send(mt MessageType, data []byte) error {
	if c.getConnState() != wsConnOpen {
		return fmt.Errorf("failed to send message: connection is closed")
	}

	msg := Message{
		Type: mt,
		Data: data,
	}
	select {
	case c.sendCh <- msg:
	default:
		return fmt.Errorf("failed to send message: channel is full")
	}
	return nil
}

// ReceiveCh returns a channel that should be used to receive messages from the
// underlying ws connection.
func (c *Client) ReceiveCh() <-chan Message {
	return c.receiveCh
}

// ErrorCh returns a channel that is used to receive client errors
// asynchronously.
func (c *Client) ErrorCh() <-chan error {
	return c.errorCh
}

// Close closes the underlying WebSocket connection.
func (c *Client) Close() error {
	c.setConnState(wsConnClosing)
	err := c.conn.close()
	c.wg.Wait()
	return err
}

func (c *Client) setConnState(st int32) {
	atomic.StoreInt32(&c.connState, st)
}

func (c *Client) getConnState() int32 {
	return atomic.LoadInt32(&c.connState)
}
