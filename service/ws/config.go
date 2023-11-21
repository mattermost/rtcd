// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

import (
	"fmt"
	"strings"
	"time"
)

type ServerConfig struct {
	// ReadBufferSize specifies the size of the internal buffer
	// used to read from a ws connection.
	ReadBufferSize int
	// WriteBufferSize specifies the size of the internal buffer
	// used to wirte to a ws connection.
	WriteBufferSize int
	// PingInterval specifies the interval at which the server should send ping
	// messages to its connections. If the client doesn't respond in 2*PingInterval
	// the server will consider the client as disconnected and drop the connection.
	PingInterval time.Duration
}

func (c ServerConfig) IsValid() error {
	if c.ReadBufferSize <= 0 {
		return fmt.Errorf("invalid ReadBufferSize value: should be greater than zero")
	}
	if c.WriteBufferSize <= 0 {
		return fmt.Errorf("invalid WriteBufferSize value: should be greater than zero")
	}
	if c.PingInterval < time.Second {
		return fmt.Errorf("invalid PingInterval value: should be at least 1 second")
	}

	return nil
}

type ClientAuthType int

const (
	BasicClientAuthType ClientAuthType = iota + 1
	BearerClientAuthType
)

type ClientConfig struct {
	// URL specifies the WebSocket URL to connect to.
	// Should start with either `ws://` or `wss://`.
	URL string
	// ConnID specifies the id of the connection to be used in case of
	// reconnection. Should be left empty on initial connect.
	ConnID string
	// AuthToken specifies the token to be used to authenticate
	// the connection.
	AuthToken string
	// AuthType specifies the type of HTTP authentication to use when connecting.
	AuthType ClientAuthType
}

func (c ClientConfig) IsValid() error {
	if c.URL == "" {
		return fmt.Errorf("invalid URL value: should not be empty")
	}

	if !strings.HasPrefix(c.URL, "ws://") && !strings.HasPrefix(c.URL, "wss://") {
		return fmt.Errorf(`invalid URL value: should start with "ws://" or "wss://"`)
	}

	if c.ConnID != "" && len(c.ConnID) != 26 {
		return fmt.Errorf("invalid ConnID value: should be 26 characters long")
	}

	if c.AuthType != BasicClientAuthType && c.AuthType != BearerClientAuthType {
		return fmt.Errorf("invalid AuthType value")
	}

	return nil
}
