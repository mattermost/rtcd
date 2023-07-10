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
	"sync"
	"sync/atomic"

	"github.com/mattermost/rtcd/service/ws"

	"github.com/mattermost/mattermost/server/public/model"
)

var idRE = regexp.MustCompile(`^[a-z0-9]{26}$`)

const (
	mmWebSocketAPIPath = "/api/v4/websocket"
)

type Config struct {
	// SiteURL is the URL of the Mattermost installation to connect to.
	SiteURL string
	// AuthToken is a valid user session authentication token.
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
	WSConnectEvent = iota + 1
	WSDisconnectEvent
	CloseEvent
	RTCConnectEvent
	RTCDisconnectEvent
)

const (
	clientStateNew int32 = iota
	clientStateInit
	clientStateClosing
	clientStateClosed
)

// Client is a Golang implementation of a client for Mattermost Calls.
type Client struct {
	cfg            Config
	ws             *ws.Client
	originalConnID string
	currentConnID  string

	handlers map[EventType]EventHandler
	wsDoneCh chan struct{}
	state    int32

	mut sync.RWMutex
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
		wsDoneCh: make(chan struct{}),
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
	c.mut.Lock()
	defer c.mut.Unlock()

	if !idRE.MatchString(channelID) {
		return fmt.Errorf("invalid channelID")
	}

	if !atomic.CompareAndSwapInt32(&c.state, clientStateNew, clientStateInit) {
		return fmt.Errorf("ws client is already initialized")
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
	c.mut.RLock()
	if !atomic.CompareAndSwapInt32(&c.state, clientStateInit, clientStateClosing) {
		c.mut.RUnlock()
		return fmt.Errorf("client is not initialized")
	}
	c.mut.RUnlock()

	if err := c.ws.Close(); err != nil {
		return fmt.Errorf("failed to close ws: %w", err)
	}

	<-c.wsDoneCh

	atomic.StoreInt32(&c.state, clientStateClosed)

	if err := c.emit(CloseEvent); err != nil {
		return fmt.Errorf("close event handler failed: %w", err)
	}

	return nil
}

// On is used to subscribe to any events fired by the client.
// Note: there can only be one subscriber per event type.
func (c *Client) On(eventType EventType, h EventHandler) {
	c.mut.Lock()
	defer c.mut.Unlock()
	c.handlers[eventType] = h
}

func (c *Client) emit(eventType EventType) error {
	c.mut.RLock()
	handler := c.handlers[eventType]
	c.mut.RUnlock()
	if handler != nil {
		if err := handler(); err != nil {
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
