// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

type ServerOption func(s *Server) error
type ClientOption func(c *Client) error

// WithAuthCb lets the caller set an optional callback to be called prior to
// performing the WebSocket upgrade.
func WithAuthCb(cb AuthCb) ServerOption {
	return func(s *Server) error {
		s.authCb = cb
		return nil
	}
}
