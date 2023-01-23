// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"golang.org/x/time/rate"

	"github.com/pion/interceptor/pkg/cc"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

const (
	SimulcastLevelHigh    string = "h"
	SimulcastLevelMedium         = "m"
	SimulcastLevelLow            = "l"
	SimulcastLevelDefault        = SimulcastLevelMedium
)

var simulcastRates = []int{2_000_000, 1_000_000, 500_000}
var simulcastLevels = []string{SimulcastLevelHigh, SimulcastLevelMedium, SimulcastLevelLow}

func getRateForSimulcastLevel(level string) int {
	var rate int
	for idx, lvl := range simulcastLevels {
		if lvl == level {
			rate = simulcastRates[idx]
			break
		}
	}
	return rate
}

func getSimulcastLevelForRate(rate int) string {
	level := SimulcastLevelLow
	for idx, r := range simulcastRates {
		if rate >= r {
			level = simulcastLevels[idx]
			break
		}
	}
	return level
}

func (s *session) initBWEstimator(bwEstimator cc.BandwidthEstimator) {
	s.mut.Lock()
	defer s.mut.Unlock()

	// TODO: consider removing both limiter and log statement once testing phase is over.
	limiter := rate.NewLimiter(1, 1)
	bwEstimator.OnTargetBitrateChange(func(rate int) {
		stats := bwEstimator.GetStats()
		lossRate, _ := stats["delayTargetBitrate"].(int)
		delayRate, _ := stats["lossTargetBitrate"].(int)

		if limiter.Allow() {
			s.log.Debug("sender bwe", mlog.String("sessionID", s.cfg.SessionID), mlog.Int("delayRate", delayRate), mlog.Any("lossRate", lossRate))
		}

		s.handleSenderBitrateChange(rate)
	})
	s.bwEstimator = bwEstimator
}

func (s *session) handleSenderBitrateChange(rate int) {
	screenSession := s.call.getScreenSession()
	if screenSession == nil {
		return
	}

	s.mut.RLock()
	sender := s.screenTrackSender
	s.mut.RUnlock()

	if sender == nil {
		// nothing to do if the session is not receiving a screen track
		return
	}

	track := sender.Track()

	if track == nil {
		s.log.Error("track should not be nil", mlog.String("sessionID", s.cfg.SessionID))
		return
	}

	currLevel := track.RID()
	if currLevel == "" {
		// not a simulcast track
		return
	}

	newLevel := getSimulcastLevelForRate(rate)
	if newLevel == currLevel {
		// no level change, nothing to do
		return
	}

	screenTrack := screenSession.getOutScreenTrack(newLevel)
	if screenTrack == nil {
		// if the desired track is not available we keep the current one
		return
	}

	s.log.Debug("switching simulcast level",
		mlog.String("sessionID", s.cfg.SessionID),
		mlog.String("currLevel", currLevel),
		mlog.String("newLevel", newLevel),
		mlog.Int("rate", rate),
	)

	select {
	case s.tracksCh <- trackActionContext{action: trackActionRemove, track: track}:
	default:
		s.log.Error("failed to send screen track: channel is full", mlog.String("sessionID", s.cfg.SessionID))
	}

	select {
	case s.tracksCh <- trackActionContext{action: trackActionAdd, track: screenTrack}:
	default:
		s.log.Error("failed to send screen track: channel is full", mlog.String("sessionID", s.cfg.SessionID))
	}
}
