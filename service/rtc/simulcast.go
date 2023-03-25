// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"time"

	"github.com/pion/interceptor/pkg/cc"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

const (
	SimulcastLevelHigh        = "h"
	SimulcastLevelLow         = "l"
	SimulcastLevelDefault     = SimulcastLevelLow
	levelChangeInitialBackoff = 10 * time.Second
	rateTolerance             = 0.9
	lossThreshold             = 0.01
)

var simulcastRates = map[string]int{
	SimulcastLevelHigh: 2_500_000,
	SimulcastLevelLow:  500_000,
}

var simulcastRateMonitorSampleSizes = map[string]time.Duration{
	SimulcastLevelHigh: 2 * time.Second,
	SimulcastLevelLow:  5 * time.Second,
}

func getRateForSimulcastLevel(level string) int {
	return simulcastRates[level]
}

func getSimulcastLevel(downRate, sourceRate int) string {
	if sourceRate > 0 && downRate > int(float32(sourceRate)*rateTolerance) {
		return SimulcastLevelHigh
	}

	return SimulcastLevelLow
}

func getSimulcastLevelForRate(rate int) string {
	if rate >= int(float32(simulcastRates[SimulcastLevelHigh])*rateTolerance) {
		return SimulcastLevelHigh
	}

	return SimulcastLevelLow
}

func (s *session) initBWEstimator(bwEstimator cc.BandwidthEstimator) {
	s.mut.Lock()
	s.bwEstimator = bwEstimator
	s.mut.Unlock()

	rateCh := make(chan int, 1)
	bwEstimator.OnTargetBitrateChange(func(rate int) {
		select {
		case rateCh <- rate:
		default:
			s.log.Error("failed to send on rateCh", mlog.String("sessionID", s.cfg.SessionID))
		}
	})

	var lastRate int
	var backoff time.Duration
	var lastLevelChangeAt time.Time
	currLevel := SimulcastLevelDefault

	rateChangeHandler := func(rate int) {
		rateDiff := (rate - lastRate)
		lastRate = rate

		stats := bwEstimator.GetStats()
		lossRate, _ := stats["lossTargetBitrate"].(int)
		delayRate, _ := stats["delayTargetBitrate"].(int)
		averageLoss, _ := stats["averageLoss"].(float64)
		state, _ := stats["state"].(string)
		s.log.Debug("sender bwe",
			mlog.String("sessionID", s.cfg.SessionID),
			mlog.Int("delayRate", delayRate),
			mlog.Int("lossRate", lossRate),
			mlog.Float64("averageLoss", averageLoss),
			mlog.Float64("backoff", backoff.Seconds()),
			mlog.String("state", state),
			mlog.Int("rateDiff", rateDiff),
		)

		// We want to give it some time for the rate estimation to stabilize when
		// switching up levels. Unless there was some decrease in estimated rate
		// in which case we want to react as quickly as possible.
		if time.Since(lastLevelChangeAt) < backoff && (currLevel == SimulcastLevelLow || rateDiff >= 0) {
			s.log.Debug("skipping bitrate check", mlog.String("sessionID", s.cfg.SessionID))
			return
		}

		// If there's minimal loss we avoid potentially downgrading the level due
		// to fluctuating rate estimatiob.
		if averageLoss < lossThreshold && currLevel == SimulcastLevelHigh {
			s.log.Debug("skipping bitrate check at highest level, no loss", mlog.String("sessionID", s.cfg.SessionID))
			return
		}

		if changed, newRate, newLevel := s.handleSenderBitrateChange(rate); changed {
			lastLevelChangeAt = time.Now()
			currLevel = newLevel

			// Adding some exponential backoff to avoid switching levels like crazy
			// if either client's bandwidth fluctuates too often or the client has
			// not enough to handle the higher rate track.
			if backoff == 0 {
				backoff = levelChangeInitialBackoff
			} else {
				backoff = backoff + backoff/2
			}

			// We update the target bitrate for the estimator to better reflect the
			// real rate of the source.
			// TODO: consider tweaking maximum bitrate as well to avoid potentially
			// plateauing on a higher than real value.
			bwEstimator.SetTargetBitrate(newRate)
		}
	}

	go func() {
		for {
			select {
			case rate := <-rateCh:
				rateChangeHandler(rate)
			case <-s.closeCh:
				return
			}
		}
	}()
}

func (s *session) handleSenderBitrateChange(downRate int) (bool, int, string) {
	screenSession := s.call.getScreenSession()
	if screenSession == nil {
		return false, 0, ""
	}

	s.mut.RLock()
	sender := s.screenTrackSender
	s.mut.RUnlock()

	if sender == nil {
		// nothing to do if the session is not receiving a screen track
		return false, 0, ""
	}

	currTrack := sender.Track()

	if currTrack == nil {
		s.log.Error("track should not be nil", mlog.String("sessionID", s.cfg.SessionID))
		return false, 0, ""
	}

	currLevel := currTrack.RID()
	if currLevel == "" {
		// not a simulcast track
		return false, 0, ""
	}

	rm := screenSession.getRateMonitor(currLevel)
	if rm == nil {
		s.log.Warn("rate monitor should not be nil")
		return false, 0, ""
	}

	currSourceRate, _ := rm.GetRate()
	if currSourceRate <= 0 {
		s.log.Warn("current source rate not available yet")
		return false, 0, ""
	}

	newLevel := getSimulcastLevel(downRate, currSourceRate)
	if newLevel == currLevel {
		// no level change, nothing to do
		return false, 0, ""
	}

	newTrack := screenSession.getOutScreenTrack(newLevel)
	if newTrack == nil {
		// if the desired track is not available we keep the current one
		return false, 0, ""
	}

	rm = screenSession.getRateMonitor(newLevel)
	if rm == nil {
		s.log.Warn("rate monitor should not be nil")
		return false, 0, ""
	}
	sourceRate, _ := rm.GetRate()

	s.log.Debug("switching simulcast level",
		mlog.String("sessionID", s.cfg.SessionID),
		mlog.String("currLevel", currLevel),
		mlog.String("newLevel", newLevel),
		mlog.Int("downRate", downRate),
		mlog.Int("currSourceRate", currSourceRate),
		mlog.Int("newSourceRate", sourceRate),
	)

	select {
	case s.tracksCh <- trackActionContext{action: trackActionRemove, track: currTrack}:
	default:
		s.log.Error("failed to send screen track: channel is full", mlog.String("sessionID", s.cfg.SessionID))
		return false, 0, ""
	}

	select {
	case s.tracksCh <- trackActionContext{action: trackActionAdd, track: newTrack}:
	default:
		s.log.Error("failed to send screen track: channel is full", mlog.String("sessionID", s.cfg.SessionID))
		return false, 0, ""
	}

	return true, sourceRate, newLevel
}
