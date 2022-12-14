// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"sync"

	"github.com/pion/webrtc/v3"
)

type call struct {
	id            string
	sessions      map[string]*session
	screenSession *session

	mut sync.RWMutex
}

func (c *call) getSession(sessionID string) *session {
	c.mut.RLock()
	defer c.mut.RUnlock()
	return c.sessions[sessionID]
}

func (c *call) addSession(cfg SessionConfig, rtcConn *webrtc.PeerConnection, closeCb func() error) (*session, bool) {
	c.mut.Lock()
	defer c.mut.Unlock()
	if s := c.sessions[cfg.SessionID]; s != nil {
		return s, false
	}

	s := &session{
		cfg:           cfg,
		rtcConn:       rtcConn,
		iceInCh:       make(chan []byte, signalChSize*2),
		sdpOfferInCh:  make(chan webrtc.SessionDescription, signalChSize),
		sdpAnswerInCh: make(chan webrtc.SessionDescription, signalChSize),
		closeCh:       make(chan struct{}),
		closeCb:       closeCb,
		tracksCh:      make(chan *webrtc.TrackLocalStaticRTP, tracksChSize),
	}

	c.sessions[cfg.SessionID] = s
	return s, true
}

func (c *call) getScreenSession() *session {
	c.mut.RLock()
	defer c.mut.RUnlock()
	return c.screenSession
}

func (c *call) setScreenSession(s *session) bool {
	c.mut.Lock()
	defer c.mut.Unlock()
	if c.screenSession == nil {
		c.screenSession = s
		return true
	}
	return false
}

func (c *call) iterSessions(cb func(s *session)) {
	c.mut.RLock()
	for _, session := range c.sessions {
		c.mut.RUnlock()
		cb(session)
		c.mut.RLock()
	}
	c.mut.RUnlock()
}
