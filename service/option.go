// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

type ClientOption func(c *Client) error
type ClientReconnectCb func(c *Client, attempt int) error

// WithClientReconnectCb lets the caller set an optional callback to be called prior to
// performing a WebSocket reconnection.
func WithClientReconnectCb(cb ClientReconnectCb) ClientOption {
	return func(c *Client) error {
		c.reconnectCb = cb
		return nil
	}
}
