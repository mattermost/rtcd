// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

type Option func(s *Server) error

func WithUpgradeCb(cb UpgradeCb) Option {
	return func(s *Server) error {
		s.upgradeCb = cb
		return nil
	}
}
