// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"context"
	"net"
)

type ClientOption func(c *Client) error
type ClientReconnectCb func(c *Client, attempt int) error
type DialContextFn func(ctx context.Context, network, addr string) (net.Conn, error)

// WithClientReconnectCb lets the caller set an optional callback to be called prior to
// performing a WebSocket reconnection.
func WithClientReconnectCb(cb ClientReconnectCb) ClientOption {
	return func(c *Client) error {
		c.reconnectCb = cb
		return nil
	}
}

// WithDialFunc lets the caller set an optional dialing function to setup the
// HTTP/WebSocket connections used by the client.
func WithDialFunc(dialFn DialContextFn) ClientOption {
	return func(c *Client) error {
		c.dialFn = dialFn
		return nil
	}
}
