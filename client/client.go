// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/mattermost/rtcd/service/ws"
)

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

// Connect connects a call in the configured channel.
func (c *Client) Connect() error {
	c.mut.Lock()
	defer c.mut.Unlock()

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
