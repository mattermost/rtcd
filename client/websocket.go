// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"bytes"
	"fmt"
	"log"

	"github.com/mattermost/rtcd/service/ws"

	"github.com/mattermost/mattermost/server/public/model"
)

const (
	mmWebSocketAPIPath = "/api/v4/websocket"
)

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
					c.currentConnID = connID
				}

				if connID != c.currentConnID {
					log.Printf("new connection id from server")
					c.currentConnID = connID
				}

				if err := c.emit(WSConnectEvent); err != nil {
					return fmt.Errorf("failed to emit connect event: %w", err)
				}
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

func (c *Client) wsReader() {
	defer close(c.wsDoneCh)
	for {
		select {
		case msg, ok := <-c.ws.ReceiveCh():
			if !ok {
				if err := c.emit(WSDisconnectEvent); err != nil {
					log.Printf("failed to emit disconnect event: %s", err)
				}
				return
			}
			if err := c.handleWSMsg(msg); err != nil {
				log.Printf("failed to handle ws message: %s", err.Error())
			}
		case err := <-c.ws.ErrorCh():
			if err != nil {
				log.Printf("ws error: %s", err.Error())
			}
		}
	}
}
