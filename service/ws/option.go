// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

type Option func(s *Server) error

// WithUpgradeCb lets the caller set an optional callback to be called prior to
// performing the websocket upgrade.
func WithUpgradeCb(cb UpgradeCb) Option {
	return func(s *Server) error {
		s.upgradeCb = cb
		return nil
	}
}
