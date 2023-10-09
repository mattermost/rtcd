// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/mattermost/rtcd/service/ws"

	"github.com/mattermost/mattermost/server/public/model"

	"github.com/vmihailenco/msgpack/v5"
)

const (
	mmWebSocketAPIPath = "/api/v4/websocket"
	wsEvPrefix         = "custom_" + pluginID + "_"

	wsMinReconnectRetryInterval    = time.Second
	wsReconnectRetryIntervalJitter = 500 * time.Millisecond
)

const (
	wsEventJoin               = wsEvPrefix + "join"
	wsEventLeave              = wsEvPrefix + "leave"
	wsEventReconnect          = wsEvPrefix + "reconnect"
	wsEventSignal             = wsEvPrefix + "signal"
	wsEventICE                = wsEvPrefix + "ice"
	wsEventSDP                = wsEvPrefix + "sdp"
	wsEventError              = wsEvPrefix + "error"
	wsEventUserLeft           = wsEvPrefix + "user_left"
	wsEventCallEnd            = wsEvPrefix + "call_end"
	wsEventCallRecordingState = wsEvPrefix + "call_recording_state"
)

var (
	wsReconnectionTimeout = 30 * time.Second
	errCallEnded          = errors.New("call ended")
)

func (c *Client) wsSend(ev string, msg any, binary bool) error {
	c.mut.Lock()
	defer c.mut.Unlock()

	var err error
	var data []byte
	var msgType ws.MessageType

	req := map[string]any{
		"action": ev,
		"seq":    c.wsClientSeqNo,
		"data":   msg,
	}

	if binary {
		msgType = ws.BinaryMessage
		data, err = msgpack.Marshal(req)
	} else {
		msgType = ws.TextMessage
		data, err = json.Marshal(req)
	}

	if err != nil {
		return fmt.Errorf("failed to marshal ws message (%s): %w", ev, err)
	}

	c.wsClientSeqNo++
	if err := c.ws.Send(msgType, data); err != nil {
		return fmt.Errorf("failed to send ws message (%s): %w", ev, err)
	}

	return nil
}

func (c *Client) handleWSEventHello(ev *model.WebSocketEvent) (isReconnect bool, err error) {
	connID, ok := ev.GetData()["connection_id"].(string)
	if !ok || connID == "" {
		return false, fmt.Errorf("missing or invalid connection ID")
	}

	if connID != c.currentConnID {
		log.Printf("new connection id from server")
	}

	if c.originalConnID == "" {
		log.Printf("initial ws connection")
		c.originalConnID = connID
	} else {
		log.Printf("ws reconnected successfully")
		c.wsLastDisconnect = time.Time{}
		c.wsReconnectInterval = 0
		isReconnect = true
	}

	c.currentConnID = connID

	c.emit(WSConnectEvent, nil)

	return
}

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

		msgConnID := ev.GetBroadcast().ConnectionId
		if msgConnID == "" {
			msgConnID, _ = ev.GetData()["connID"].(string)
		}

		if msgConnID != "" && msgConnID != c.currentConnID && msgConnID != c.originalConnID {
			// ignoring any messages not specifically meant for us.
			return nil
		}

		if c.originalConnID == "" && ev.EventType() != model.WebsocketEventHello {
			return fmt.Errorf("ws message received while waiting for hello")
		}

		switch ev.EventType() {
		case model.WebsocketEventHello:
			isReconnect, err := c.handleWSEventHello(ev)
			if err != nil {
				return fmt.Errorf("failed to handle hello event: %w", err)
			}
			if !isReconnect {
				if err := c.joinCall(); err != nil {
					return fmt.Errorf("failed to join call: %w", err)
				}
			}
		case wsEventJoin:
			c.emit(WSCallJoinEvent, nil)
			if err := c.initRTCSession(); err != nil {
				return fmt.Errorf("failed to init RTC session: %w", err)
			}
		case wsEventSignal:
			if err := c.handleWSEventSignal(ev.GetData()); err != nil {
				return fmt.Errorf("failed to handle signal event: %w", err)
			}
		case wsEventError:
			errMsg, _ := ev.GetData()["data"].(string)
			err := fmt.Errorf("ws error: %s", errMsg)
			c.emit(ErrorEvent, err)
			return err
		case wsEventUserLeft:
			sessionID, _ := ev.GetData()["session_id"].(string)
			if sessionID == "" {
				return fmt.Errorf("missing session_id from user_left event")
			}
			c.mut.Lock()
			if rx := c.receivers[sessionID]; rx != nil {
				log.Printf("stopping receiver for disconnected session %q", sessionID)
				if err := rx.Stop(); err != nil {
					log.Printf("failed to stop receiver for session %q: %s", sessionID, err)
				}
				delete(c.receivers, sessionID)
			}
			c.mut.Unlock()
		case wsEventCallEnd:
			channelID := ev.GetBroadcast().ChannelId
			if channelID == "" {
				channelID, _ = ev.GetData()["channelID"].(string)
			}
			if channelID == c.cfg.ChannelID {
				log.Printf("received call end event, closing client")
				return errCallEnded
			}
		case wsEventCallRecordingState:
			data, ok := ev.GetData()["recState"].(map[string]any)
			if !ok {
				return fmt.Errorf("invalid recording state")
			}
			var recState CallJobState
			recState.FromMap(data)
			c.emit(WSCallRecordingState, recState)
		default:
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

	if c.originalConnID != "" {
		if err := c.reconnectCall(); err != nil {
			return fmt.Errorf("reconnectCall failed: %w", err)
		}
	}

	return nil
}

func (c *Client) wsReader() {
	defer func() {
		if err := c.leaveCall(); err != nil {
			log.Printf(err.Error())
		}
		close(c.wsDoneCh)
	}()

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
				if errors.Is(err, errCallEnded) {
					c.close()
					return
				}
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
