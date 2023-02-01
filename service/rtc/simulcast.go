// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/pion/interceptor/pkg/cc"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

const (
	SimulcastLevelHigh    string = "h"
	SimulcastLevelLow            = "l"
	SimulcastLevelDefault        = SimulcastLevelLow
	levelChangeBackoff           = 2 * time.Second
)

var simulcastRates = []int{2_500_000, 500_000}
var simulcastLevels = []string{SimulcastLevelHigh, SimulcastLevelLow}

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

func getSimulcastLevel(downRate, upRate int) string {
	if upRate > 0 && downRate > int(float32(upRate)*0.9) {
		return SimulcastLevelHigh
	}

	return SimulcastLevelLow
}

func getSimulcastLevelForRate(rate int) string {
	if rate >= simulcastRates[0] {
		return SimulcastLevelHigh
	}

	return SimulcastLevelLow
}

func (s *session) initBWEstimator(bwEstimator cc.BandwidthEstimator) {
	s.mut.Lock()
	defer s.mut.Unlock()

	// TODO: consider removing both limiter and log statement once testing phase is over.
	limiter := rate.NewLimiter(1, 1)
	var lastLevelChangeAt atomic.Value
	lastLevelChangeAt.Store(time.Now())
	bwEstimator.OnTargetBitrateChange(func(rate int) {
		stats := bwEstimator.GetStats()
		lossRate, _ := stats["lossTargetBitrate"].(int)
		delayRate, _ := stats["delayTargetBitrate"].(int)

		if limiter.Allow() {
			s.log.Debug("sender bwe", mlog.String("sessionID", s.cfg.SessionID), mlog.Int("delayRate", delayRate), mlog.Any("lossRate", lossRate))
		}

		if time.Since(lastLevelChangeAt.Load().(time.Time)) < levelChangeBackoff {
			s.log.Debug("skipping bitrate check")
			return
		}

		if ok, newRate := s.handleSenderBitrateChange(rate); ok {
			lastLevelChangeAt.Store(time.Now())
			s.log.Debug("setting new target rate", mlog.Int("rate", newRate))
			bwEstimator.SetTargetBitrate(newRate)
		}
	})
	s.bwEstimator = bwEstimator
}

func (s *session) handleSenderBitrateChange(rate int) (bool, int) {
	screenSession := s.call.getScreenSession()
	if screenSession == nil {
		return false, 0
	}

	s.mut.RLock()
	sender := s.screenTrackSender
	s.mut.RUnlock()

	if sender == nil {
		// nothing to do if the session is not receiving a screen track
		return false, 0
	}

	track := sender.Track()

	if track == nil {
		s.log.Error("track should not be nil", mlog.String("sessionID", s.cfg.SessionID))
		return false, 0
	}

	currLevel := track.RID()
	if currLevel == "" {
		// not a simulcast track
		return false, 0
	}

	rm := screenSession.getRateMonitor(currLevel)
	if rm == nil {
		s.log.Warn("rate monitor should not be nil")
		return false, 0
	}

	s.log.Debug("rates", mlog.Int("down", rate), mlog.Int("up", rm.GetRate()*1000))
	newLevel := getSimulcastLevel(rate, rm.GetRate()*1000)
	if newLevel == currLevel {
		// no level change, nothing to do
		return false, 0
	}

	screenTrack := screenSession.getOutScreenTrack(newLevel)
	if screenTrack == nil {
		// if the desired track is not available we keep the current one
		return false, 0
	}

	rm = screenSession.getRateMonitor(newLevel)
	if rm == nil {
		s.log.Warn("rate monitor should not be nil")
		return false, 0
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

	return true, rm.GetRate() * 1000
}
