// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"bytes"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"

	"github.com/mattermost/rtcd/service/ws"

	"github.com/mattermost/mattermost/server/public/model"
)

var idRE = regexp.MustCompile(`^[a-z0-9]{26}$`)

const (
	mmWebSocketAPIPath = "/api/v4/websocket"
)

type Config struct {
	SiteURL   string
	AuthToken string

	wsURL string
}

func (c *Config) Parse() error {
	if c.SiteURL == "" {
		return fmt.Errorf("invalid SiteURL value: should not be empty")
	}
	c.SiteURL = strings.TrimRight(strings.TrimSpace(c.SiteURL), "/")
	u, err := url.Parse(c.SiteURL)
	if err != nil {
		return fmt.Errorf("failed to parse SiteURL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid SiteURL scheme %q", u.Scheme)
	}

	if u.Scheme == "http" {
		u.Scheme = "ws"
		u.Path += mmWebSocketAPIPath
	} else {
		u.Scheme = "wss"
		u.Path += mmWebSocketAPIPath
	}
	c.wsURL = u.String()

	if c.AuthToken == "" {
		return fmt.Errorf("invalid AuthToken value: should not be empty")
	}

	return nil
}

type EventHandler func() error
type EventType int

const (
	ConnectEvent = iota + 1
	DisconnectEvent
	CloseEvent
)

type Client struct {
	cfg            Config
	ws             *ws.Client
	originalConnID string
	currentConnID  string

	handlers map[EventType]EventHandler
}

type Option func(c *Client) error

// New initializes and returns a new Calls client.
func New(cfg Config, opts ...Option) (*Client, error) {
	if err := cfg.Parse(); err != nil {
		return nil, fmt.Errorf("failed to validate config: %w", err)
	}

	c := &Client{
		cfg:      cfg,
		handlers: make(map[EventType]EventHandler),
	}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	return c, nil
}

// Connect connects to a call in the given channel.
func (c *Client) Connect(channelID string) error {
	if !idRE.MatchString(channelID) {
		return fmt.Errorf("invalid channelID")
	}

	ws, err := ws.NewClient(ws.ClientConfig{
		URL:       c.cfg.wsURL,
		AuthToken: c.cfg.AuthToken,
		AuthType:  ws.BearerClientAuthType,
	})
	if err != nil {
		return fmt.Errorf("failed to create websocket client: %w", err)
	}
	c.ws = ws

	go c.wsReader()

	return nil
}

// Close permanently disconnects the client.
func (c *Client) Close() error {
	if err := c.ws.Close(); err != nil {
		return fmt.Errorf("failed to close ws: %w", err)
	}

	if err := c.emit(CloseEvent); err != nil {
		return fmt.Errorf("close event handler failed: %w", err)
	}

	c.handlers = nil

	return nil
}

// On is used to subscribe to any events fired by the client.
func (c *Client) On(eventType EventType, h EventHandler) {
	c.handlers[eventType] = h
}

func (c *Client) emit(eventType EventType) error {
	if h := c.handlers[eventType]; h != nil {
		if err := h(); err != nil {
			return err
		}
	}
	return nil
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

				if err := c.emit(ConnectEvent); err != nil {
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
	for {
		select {
		case msg, ok := <-c.ws.ReceiveCh():
			if !ok {
				// disconnect
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
