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
	SimulcastLevelHigh        = "h"
	SimulcastLevelLow         = "l"
	SimulcastLevelDefault     = SimulcastLevelLow
	levelChangeInitialBackoff = 4 * time.Second
	rateTolerance             = 0.9
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

func getSimulcastLevel(downRate, sourceRate int) string {
	if sourceRate > 0 && downRate > int(float32(sourceRate)*rateTolerance) {
		return SimulcastLevelHigh
	}

	return SimulcastLevelLow
}

func getSimulcastLevelForRate(rate int) string {
	if rate >= int(float32(simulcastRates[0])*rateTolerance) {
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

	var lastRate int32
	atomic.StoreInt32(&lastRate, int32(bwEstimator.GetTargetBitrate()))

	var backoff atomic.Value
	backoff.Store(time.Duration(0))

	bwEstimator.OnTargetBitrateChange(func(rate int) {
		diffRate := float32(rate) / float32(atomic.LoadInt32(&lastRate))
		defer atomic.StoreInt32(&lastRate, int32(rate))

		if limiter.Allow() {
			stats := bwEstimator.GetStats()
			lossRate, _ := stats["lossTargetBitrate"].(int)
			delayRate, _ := stats["delayTargetBitrate"].(int)
			averageLoss, _ := stats["averageLoss"].(float64)
			s.log.Debug("sender bwe",
				mlog.String("sessionID", s.cfg.SessionID),
				mlog.Int("delayRate", delayRate),
				mlog.Int("lossRate", lossRate),
				mlog.Float64("averageLoss", averageLoss),
				mlog.Float32("diffRate", diffRate),
				mlog.Float64("backoff", backoff.Load().(time.Duration).Seconds()),
			)
		}

		// We want to give it some time for the rate estimation to stabilize when
		// switching levels. Unless there was some decrease in estimated rate
		// in which case we want to react as quickly as possible.
		if time.Since(lastLevelChangeAt.Load().(time.Time)) < backoff.Load().(time.Duration) && diffRate > 1 {
			s.log.Debug("skipping bitrate check", mlog.String("sessionID", s.cfg.SessionID))
			return
		}

		if ok, newRate := s.handleSenderBitrateChange(rate); ok {
			lastLevelChangeAt.Store(time.Now())

			// Adding some exponential backoff to avoid switching levels like crazy
			// if either client's bandwidth fluctuates too often or the client has
			// not enough to handle the higher rate track.
			if b := backoff.Load().(time.Duration); b == 0 {
				backoff.Store(levelChangeInitialBackoff)
			} else {
				backoff.Store(b + b/2)
			}

			// We update the target bitrate for the estimator to better reflect the
			// real rate of the source.
			// TODO: consider tweaking maximum bitrate as well to avoid potentially
			// plateauing on a higher than real value.
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

	newLevel := getSimulcastLevel(rate, rm.GetRate())
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
	sourceRate := rm.GetRate()

	s.log.Debug("switching simulcast level",
		mlog.String("sessionID", s.cfg.SessionID),
		mlog.String("currLevel", currLevel),
		mlog.String("newLevel", newLevel),
		mlog.Int("downRate", rate),
		mlog.Int("sourceRate", sourceRate),
	)

	select {
	case s.tracksCh <- trackActionContext{action: trackActionRemove, track: track}:
	default:
		s.log.Error("failed to send screen track: channel is full", mlog.String("sessionID", s.cfg.SessionID))
		return false, 0
	}

	select {
	case s.tracksCh <- trackActionContext{action: trackActionAdd, track: screenTrack}:
	default:
		s.log.Error("failed to send screen track: channel is full", mlog.String("sessionID", s.cfg.SessionID))
		return false, 0
	}

	return true, sourceRate
}
