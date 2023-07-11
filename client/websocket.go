// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/mattermost/rtcd/service/ws"

	"github.com/mattermost/mattermost/server/public/model"
)

const (
	mmWebSocketAPIPath = "/api/v4/websocket"

	wsMinReconnectRetryInterval    = time.Second
	wsReconnectRetryIntervalJitter = 500 * time.Millisecond
)

var wsReconnectionTimeout = 30 * time.Second

func (c *Client) handleWSMsg(msg ws.Message) error {
	switch msg.Type {
	case ws.TextMessage:
		ev, err := model.WebSocketEventFromJSON(bytes.NewReader(msg.Data))
		if err != nil {
			return fmt.Errorf("failed to unmarshal event: %w", err)
		}
		if ev == nil {
			return fmt.Errorf("unexpected nil event")
		}
		if !ev.IsValid() {
			return fmt.Errorf("invalid event")
		}

		if ev.EventType() == model.WebsocketEventHello {
			if connID, ok := ev.GetData()["connection_id"].(string); ok && connID != "" {
				if c.originalConnID == "" {
					log.Printf("initial ws connection")
					c.originalConnID = connID
				} else {
					log.Printf("ws reconnected successfully")
					c.wsLastDisconnect = time.Time{}
					c.wsReconnectInterval = 0
				}

				if connID != c.currentConnID {
					log.Printf("new connection id from server")
				}

				c.currentConnID = connID

				c.emit(WSConnectEvent, nil)
			} else {
				return fmt.Errorf("missing or invalid connection ID")
			}
		}
	case ws.BinaryMessage:
	default:
		return fmt.Errorf("invalid ws message type %d", msg.Type)
	}

	return nil
}

func (c *Client) wsOpen() error {
	ws, err := ws.NewClient(ws.ClientConfig{
		URL:       c.cfg.wsURL,
		AuthToken: c.cfg.AuthToken,
		AuthType:  ws.BearerClientAuthType,
	})
	if err != nil {
		return fmt.Errorf("failed to create websocket client: %w", err)
	}
	c.ws = ws

	return nil
}

func (c *Client) wsReader() {
	defer close(c.wsDoneCh)

	for {
		select {
		case msg, ok := <-c.ws.ReceiveCh():
			if !ok {
				c.emit(WSDisconnectEvent, nil)

				// reconnect handler
				if c.wsLastDisconnect.IsZero() {
					c.wsLastDisconnect = time.Now()
				} else if time.Since(c.wsLastDisconnect) > wsReconnectionTimeout {
					log.Printf("ws reconnection timeout reached, closing")
					c.emit(ErrorEvent, fmt.Errorf("ws reconnection timeout reached"))
					c.close()
					return
				}

				c.wsReconnectInterval += wsMinReconnectRetryInterval + time.Duration(rand.Int63n(wsReconnectRetryIntervalJitter.Milliseconds()))*time.Millisecond
				log.Printf("ws disconnected, attemping reconnection in %v...", c.wsReconnectInterval)
				time.Sleep(c.wsReconnectInterval)
				if err := c.wsOpen(); err != nil {
					log.Printf(err.Error())
				}

				continue
			}
			if err := c.handleWSMsg(msg); err != nil {
				log.Printf("failed to handle ws message: %s", err.Error())
			}
		case err := <-c.ws.ErrorCh():
			if err != nil {
				log.Printf("ws error: %s", err.Error())
			}
		case <-c.wsCloseCh:
			return
		}
	}
}
