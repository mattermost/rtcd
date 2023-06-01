// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"sync"

	"github.com/pion/webrtc/v3"

	"github.com/mattermost/mattermost-server/server/public/shared/mlog"
)

type call struct {
	id            string
	sessions      map[string]*session
	screenSession *session
	metrics       Metrics

	mut sync.RWMutex
}

func (c *call) getSession(sessionID string) *session {
	c.mut.RLock()
	defer c.mut.RUnlock()
	return c.sessions[sessionID]
}

func (c *call) addSession(cfg SessionConfig, rtcConn *webrtc.PeerConnection, closeCb func() error, log mlog.LoggerIFace) (*session, bool) {
	c.mut.Lock()
	defer c.mut.Unlock()
	if s := c.sessions[cfg.SessionID]; s != nil {
		return s, false
	}

	s := &session{
		cfg:                cfg,
		rtcConn:            rtcConn,
		iceInCh:            make(chan []byte, signalChSize*2),
		sdpOfferInCh:       make(chan webrtc.SessionDescription, signalChSize),
		sdpAnswerInCh:      make(chan webrtc.SessionDescription, signalChSize),
		closeCh:            make(chan struct{}),
		closeCb:            closeCb,
		tracksCh:           make(chan trackActionContext, tracksChSize),
		outScreenTracks:    make(map[string]*webrtc.TrackLocalStaticRTP),
		remoteScreenTracks: make(map[string]*webrtc.TrackRemote),
		screenRateMonitors: make(map[string]*RateMonitor),
		log:                log,
		call:               c,
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
	defer c.mut.RUnlock()
	for _, session := range c.sessions {
		cb(session)
	}
}

func (c *call) clearScreenState(screenSession *session) {
	c.mut.Lock()
	defer c.mut.Unlock()

	if screenSession == nil || c.screenSession != screenSession {
		return
	}

	for _, s := range c.sessions {
		s.mut.Lock()
		if s == c.screenSession {
			s.clearScreenState()
			c.screenSession = nil
		} else if s.screenTrackSender != nil {
			select {
			case s.tracksCh <- trackActionContext{action: trackActionRemove, track: s.screenTrackSender.Track()}:
			default:
				s.log.Error("failed to send screen track: channel is full", mlog.String("sessionID", s.cfg.SessionID))
			}
			s.screenTrackSender = nil
		}
		s.mut.Unlock()
	}
}

// handleSessionClose cleans up resources such as senders or receivers for the
// closing session.
// NOTE: this is expected to always be called under lock (call.mut).
func (c *call) handleSessionClose(us *session) {
	us.mut.RLock()
	defer us.mut.RUnlock()

	cleanUp := func(sessionID string, sender *webrtc.RTPSender, track webrtc.TrackLocal) {
		c.metrics.DecRTPTracks(us.cfg.GroupID, us.cfg.CallID, "out", getTrackType(track.Kind()))

		if err := sender.ReplaceTrack(nil); err != nil {
			us.log.Error("failed to replace track on sender",
				mlog.String("sessionID", sessionID),
				mlog.String("trackID", track.ID()))
		}

		if err := sender.Stop(); err != nil {
			us.log.Error("failed to stop sender for track",
				mlog.String("sessionID", sessionID),
				mlog.String("trackID", track.ID()))
		}
	}

	// First we cleanup any track the closing session may have been receiving and stop
	// the associated senders.
	for _, sender := range us.rtcConn.GetSenders() {
		if track := sender.Track(); track != nil {
			cleanUp(us.cfg.SessionID, sender, track)
		}
	}

	// We check whether the closing session was also sending any track
	// (e.g. voice, screen).
	outTracks := map[string]bool{}
	if us.outVoiceTrack != nil {
		outTracks[us.outVoiceTrack.ID()] = true
	}
	if us.outScreenAudioTrack != nil {
		outTracks[us.outScreenAudioTrack.ID()] = true
	}
	for _, track := range us.outScreenTracks {
		outTracks[track.ID()] = true
	}

	// Nothing left to do if the closing session wasn't sending anything.
	if len(outTracks) == 0 {
		return
	}

	// We finally go ahead and cleanup any tracks that the closing session may
	// have been sending to other connected sessions.
	for _, ss := range c.sessions {
		if ss.cfg.SessionID == us.cfg.SessionID {
			continue
		}

		ss.mut.RLock()
		for _, sender := range ss.rtcConn.GetSenders() {
			if track := sender.Track(); track != nil && outTracks[track.ID()] {
				cleanUp(ss.cfg.SessionID, sender, track)
			}
		}
		ss.mut.RUnlock()
	}
}
