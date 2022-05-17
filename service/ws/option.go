// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

import (
	"context"
	"net"
)

type ServerOption func(s *Server) error
type ClientOption func(c *Client) error
type DialContextFn func(ctx context.Context, network, addr string) (net.Conn, error)

// WithAuthCb lets the caller set an optional callback to be called prior to
// performing the WebSocket upgrade.
func WithAuthCb(cb AuthCb) ServerOption {
	return func(s *Server) error {
		s.authCb = cb
		return nil
	}
}

// WithDialFunc lets the caller set an optional dialing function to setup the
// TCP connection needed by the client.
func WithDialFunc(dialFn DialContextFn) ClientOption {
	return func(c *Client) error {
		c.dialFn = dialFn
		return nil
	}
}
