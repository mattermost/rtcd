// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"fmt"
	"sync"

	"github.com/pion/webrtc/v4"
	"golang.org/x/time/rate"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/mattermost/rtcd/service/rtc/dc"
)

type call struct {
	id            string
	sessions      map[string]*session
	screenSession *session
	pliLimiters   map[webrtc.SSRC]*rate.Limiter
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
		sdpOfferInCh:       make(chan offerMessage, signalChSize),
		sdpAnswerInCh:      make(chan webrtc.SessionDescription, signalChSize),
		dcSDPCh:            make(chan Message, signalChSize),
		dcOutCh:            make(chan dcMessage, signalChSize),
		dcOpenCh:           make(chan struct{}, 1),
		signalingLock:      dc.NewLock(),
		closeCh:            make(chan struct{}),
		closeCb:            closeCb,
		doneCh:             make(chan struct{}),
		tracksCh:           make(chan trackActionContext, tracksChSize),
		outScreenTracks:    make(map[string][]*webrtc.TrackLocalStaticRTP),
		remoteScreenTracks: make(map[string]*webrtc.TrackRemote),
		screenRateMonitors: make(map[string]*RateMonitor),
		outVideoTracks:     make(map[string][]*webrtc.TrackLocalStaticRTP),
		remoteVideoTracks:  make(map[string]*webrtc.TrackRemote),
		videoRateMonitors:  make(map[string]*RateMonitor),
		log:                log,
		call:               c,
		rxTracks:           make(map[string]webrtc.TrackLocal),
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
	for _, session := range c.sessions {
		cb(session)
	}
}

func (c *call) clearScreenState(screenSession *session) error {
	c.mut.Lock()
	defer c.mut.Unlock()

	if screenSession == nil {
		return fmt.Errorf("screenSession should not be nil")
	}

	if c.screenSession == nil {
		return fmt.Errorf("call.screenSession should not be nil")
	}

	if c.screenSession != screenSession {
		return fmt.Errorf("screenSession mismatch, call.screenSession=%s, screenSession=%s",
			c.screenSession.cfg.SessionID, screenSession.cfg.SessionID)
	}

	for _, s := range c.sessions {
		s.mut.Lock()
		if s == c.screenSession {
			s.clearScreenState()
			c.screenSession = nil
		} else if s.screenTrackSender != nil {
			select {
			case s.tracksCh <- trackActionContext{action: trackActionRemove, localTrack: s.screenTrackSender.Track()}:
			default:
				s.log.Error("failed to send screen track: channel is full", mlog.String("sessionID", s.cfg.SessionID))
			}
			s.screenTrackSender = nil
		}
		s.mut.Unlock()
	}

	return nil
}

// handleSessionClose cleans up resources such as senders or receivers for the
// closing session.
// NOTE: this is expected to always be called under lock (call.mut).
func (c *call) handleSessionClose(us *session) {
	us.log.Debug("handleSessionClose", mlog.String("sessionID", us.cfg.SessionID))

	us.mut.Lock()
	defer us.mut.Unlock()

	cleanUp := func(sessionID string, sender *webrtc.RTPSender, track webrtc.TrackLocal) {
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

	// If the session getting closed was screen sharing we need to do some extra
	// cleanup.
	if us == c.screenSession {
		c.screenSession = nil
		for _, ss := range c.sessions {
			if ss.cfg.SessionID == us.cfg.SessionID {
				continue
			}
			ss.mut.Lock()
			ss.screenTrackSender = nil
			ss.mut.Unlock()
		}
	}

	// First we cleanup any track the closing session may have been receiving and stop
	// the associated senders.
	for _, sender := range us.rtcConn.GetSenders() {
		if track := sender.Track(); track != nil {
			us.log.Debug("cleaning up out track on receiver",
				mlog.String("sessionID", us.cfg.SessionID),
				mlog.String("trackID", track.ID()),
			)
			cleanUp(us.cfg.SessionID, sender, track)
		}
	}
	for _, track := range us.rxTracks {
		c.metrics.DecRTPTracks(us.cfg.GroupID, "out", getTrackKind(track.Kind()))
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
	for _, tracks := range us.outScreenTracks {
		for _, track := range tracks {
			outTracks[track.ID()] = true
		}
	}
	for _, tracks := range us.outVideoTracks {
		for _, track := range tracks {
			outTracks[track.ID()] = true
		}
	}

	// Nothing left to do if the closing session wasn't sending anything.
	if len(outTracks) == 0 {
		us.log.Debug("no out tracks to cleanup, returning",
			mlog.String("sessionID", us.cfg.SessionID))
		return
	}

	// We finally go ahead and cleanup any tracks that the closing session may
	// have been sending to other connected sessions.
	for _, ss := range c.sessions {
		if ss.cfg.SessionID == us.cfg.SessionID {
			continue
		}

		ss.mut.Lock()
		for _, sender := range ss.rtcConn.GetSenders() {
			if track := sender.Track(); track != nil && outTracks[track.ID()] {
				us.log.Debug("cleaning up out track on sender",
					mlog.String("senderID", us.cfg.SessionID),
					mlog.String("sessionID", ss.cfg.SessionID),
					mlog.String("trackID", track.ID()),
				)
				// If it's a screen sharing track we should remove it as we normally would when
				// sharing ends.
				if track.Kind() == webrtc.RTPCodecTypeVideo {
					select {
					case ss.tracksCh <- trackActionContext{action: trackActionRemove, localTrack: track}:
					default:
						ss.log.Error("failed to send screen track: channel is full", mlog.String("sessionID", ss.cfg.SessionID))
					}
				} else {
					cleanUp(ss.cfg.SessionID, sender, track)
				}
			}
		}
		ss.mut.Unlock()
	}
}
