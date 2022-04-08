// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"sync"
)

// group defines a collection of calls that belongs to the same group.
type group struct {
	id    string
	calls map[string]*call

	mut sync.RWMutex
}

func (g *group) getCall(callID string) *call {
	g.mut.RLock()
	defer g.mut.RUnlock()
	return g.calls[callID]
}

func (s *Server) getGroup(groupID string) *group {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.groups[groupID]
}
