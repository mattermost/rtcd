// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mattermost/rtcd/service/random"

	"github.com/gorilla/websocket"
)

const (
	WSConnClosed int32 = iota
	WSConnOpen
	WSConnClosing
)

type Client struct {
	cfg           ClientConfig
	conn          *conn
	sendCh        chan Message
	receiveCh     chan Message
	errorCh       chan error
	flushCh       chan struct{}
	flushDoneCh   chan struct{}
	wg            sync.WaitGroup
	connState     int32
	dialFn        DialContextFn
	pingHandlerFn func(msg string) error
}

// NewClient initializes and returns a new WebSocket client.
func NewClient(cfg ClientConfig, opts ...ClientOption) (*Client, error) {
	if err := cfg.IsValid(); err != nil {
		return nil, fmt.Errorf("failed to validate config: %w", err)
	}

	c := &Client{
		cfg:         cfg,
		sendCh:      make(chan Message, sendChSize),
		receiveCh:   make(chan Message, ReceiveChSize),
		errorCh:     make(chan error, 32),
		flushCh:     make(chan struct{}, 1),
		flushDoneCh: make(chan struct{}),
	}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	header := http.Header{}
	if cfg.AuthType == BearerClientAuthType {
		header.Set("Authorization", "Bearer "+cfg.AuthToken)
	} else {
		header.Set("Authorization", "Basic "+cfg.AuthToken)
	}

	dialer := *websocket.DefaultDialer
	if c.dialFn != nil {
		dialer.NetDialContext = c.dialFn
	}
	ws, _, err := dialer.Dial(cfg.URL, header)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %w", err)
	}

	if c.pingHandlerFn != nil {
		ws.SetPingHandler(c.pingHandlerFn)
	}

	connID := cfg.ConnID
	if connID == "" {
		connID = random.NewID()
	}
	c.conn = newConn(connID, "", ws)

	c.wg.Add(2)
	go c.connReader()
	go c.connWriter()
	c.setConnState(WSConnOpen)

	return c, nil
}

func (c *Client) connReader() {
	defer func() {
		close(c.receiveCh)
		c.wg.Done()
		close(c.conn.closeCh)
		c.wg.Wait()
		close(c.errorCh)
		c.setConnState(WSConnClosed)
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
	defer func() {
		close(c.flushDoneCh)
		c.wg.Done()
	}()

	sendMsg := func(msg Message) {
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
	}

	for {
		select {
		case msg := <-c.sendCh:
			sendMsg(msg)
		case <-c.flushCh:
			log.Printf("flushing %d queued messages", len(c.sendCh))
			for i := 0; i < len(c.sendCh); i++ {
				sendMsg(<-c.sendCh)
			}
			return
		case <-c.conn.closeCh:
			return
		}
	}
}

func (c *Client) sendError(err error) {
	if c.GetConnState() != WSConnOpen {
		return
	}
	select {
	case c.errorCh <- err:
	default:
		log.Printf("failed to send error: channel is full: %s", err.Error())
	}
}

// SendMsg sends a WebSocket message with the specified type and data.
func (c *Client) Send(mt MessageType, data []byte) error {
	if c.GetConnState() != WSConnOpen {
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
	c.setConnState(WSConnClosing)
	if err := c.flush(); err != nil {
		return err
	}
	err := c.conn.close()
	c.wg.Wait()
	return err
}

func (c *Client) setConnState(st int32) {
	atomic.StoreInt32(&c.connState, st)
}

func (c *Client) GetConnState() int32 {
	return atomic.LoadInt32(&c.connState)
}

func (c *Client) flush() error {
	select {
	case c.flushCh <- struct{}{}:
	default:
		return fmt.Errorf("failed to send flush message")
	}
	<-c.flushDoneCh
	return nil
}
