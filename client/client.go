// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mattermost/rtcd/service/ws"
)

type EventHandler func(ctx any) error

type EventType string

const (
	WSConnectEvent     EventType = "WSConnect"
	WSDisconnectEvent            = "WSDisconnect"
	RTCConnectEvent              = "RTCConnect"
	RTCDisconnectEvent           = "RTCDisconnect"
	CloseEvent                   = "Close"
	ErrorEvent                   = "Error"
)

const (
	clientStateNew int32 = iota
	clientStateInit
	clientStateClosing
	clientStateClosed
)

// Client is a Golang implementation of a client for Mattermost Calls.
type Client struct {
	cfg Config

	handlers map[EventType]EventHandler

	// WebSocket
	ws                  *ws.Client
	wsDoneCh            chan struct{}
	wsCloseCh           chan struct{}
	wsReconnectInterval time.Duration
	wsLastDisconnect    time.Time
	originalConnID      string
	currentConnID       string

	state int32

	mut sync.RWMutex
}

type Option func(c *Client) error

// New initializes and returns a new Calls client.
func New(cfg Config, opts ...Option) (*Client, error) {
	if err := cfg.Parse(); err != nil {
		return nil, fmt.Errorf("failed to validate config: %w", err)
	}

	c := &Client{
		cfg:       cfg,
		handlers:  make(map[EventType]EventHandler),
		wsDoneCh:  make(chan struct{}),
		wsCloseCh: make(chan struct{}),
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

	if err := c.wsOpen(); err != nil {
		return err
	}

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

	close(c.wsCloseCh)
	<-c.wsDoneCh

	if err := c.ws.Close(); err != nil {
		return fmt.Errorf("failed to close ws: %w", err)
	}

	c.close()

	return nil
}

// On is used to subscribe to any events fired by the client.
// Note: there can only be one subscriber per event type.
func (c *Client) On(eventType EventType, h EventHandler) {
	c.mut.Lock()
	defer c.mut.Unlock()
	c.handlers[eventType] = h
}

func (c *Client) emit(eventType EventType, ctx any) {
	c.mut.RLock()
	handler := c.handlers[eventType]
	c.mut.RUnlock()
	if handler != nil {
		if err := handler(ctx); err != nil {
			log.Printf("failed to emit event (%s): %s", eventType, err.Error())
		}
	}
}

func (c *Client) close() {
	atomic.StoreInt32(&c.state, clientStateClosed)
	c.emit(CloseEvent, nil)
}
