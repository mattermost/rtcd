// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/pion/interceptor/pkg/cc"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

const (
	SimulcastLevelHigh        = "h"
	SimulcastLevelLow         = "l"
	SimulcastLevelDefault     = SimulcastLevelLow
	levelChangeInitialBackoff = 10 * time.Second
	rateTolerance             = 0.9
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

	// Allowing up to one rate change per second with a burst size of 4.
	limiter := rate.NewLimiter(1, 4)
	rateCh := make(chan int, 4)
	bwEstimator.OnTargetBitrateChange(func(rate int) {
		if !limiter.Allow() {
			return
		}

		select {
		case rateCh <- rate:
		default:
			s.log.Error("failed to send on rateCh", mlog.String("sessionID", s.cfg.SessionID))
		}
	})

	var backoff time.Duration
	var lastLevelChangeAt time.Time
	currLevel := SimulcastLevelDefault

	rateChangeHandler := func(rate int) {
		stats := bwEstimator.GetStats()
		lossRate, _ := stats["lossTargetBitrate"].(int)
		delayRate, _ := stats["delayTargetBitrate"].(int)
		averageLoss, _ := stats["averageLoss"].(float64)
		state, _ := stats["state"].(string)
		s.log.Debug("sender bwe",
			mlog.String("sessionID", s.cfg.SessionID),
			mlog.Int("delayRate", delayRate),
			mlog.Int("lossRate", lossRate),
			mlog.String("averageLoss", fmt.Sprintf("%.5f", averageLoss)),
			mlog.Float64("backoff", backoff.Seconds()),
			mlog.String("state", state),
		)

		// We want to give it some time for the rate estimation to stabilize
		// before attempting to upgrade level again.
		if time.Since(lastLevelChangeAt) < backoff && currLevel == SimulcastLevelLow {
			s.log.Debug("skipping bitrate check due to backoff", mlog.String("sessionID", s.cfg.SessionID))
			return
		}

		if changed, newRate, newLevel := s.handleSenderBitrateChange(rate, lossRate); changed {
			// Adding some exponential backoff to avoid switching levels too often
			// if either client's network conditions fluctuate too often or the client has
			// not enough bandwidth to handle the higher rate track.
			if newLevel == SimulcastLevelLow {
				if backoff == 0 {
					backoff = levelChangeInitialBackoff
				} else {
					backoff = backoff + backoff/2
				}
			}

			// We update the maximum rate for the estimator to better match the
			// actual source rate.
			bwEstimator.SetMaxBitrate(int(float64(newRate) * 1.5))

			// We update the target bitrate for the estimator to better reflect the
			// real rate of the source.
			bwEstimator.SetTargetBitrate(newRate)

			currLevel = newLevel
			lastLevelChangeAt = time.Now()
		}
	}

	updateMaxSourceRate := func() {
		s.mut.RLock()
		screenSession := s.call.getScreenSession()
		s.mut.RUnlock()
		if screenSession == nil || s == screenSession {
			return
		}

		currLevel, err := s.getSenderSimulcastLevel()
		if err != nil {
			s.log.Error("failed to get sender simulcast level", mlog.String("sessionID", s.cfg.SessionID), mlog.Err(err))
			return
		}

		sourceRate := screenSession.getSourceRate(currLevel)
		if sourceRate <= 0 {
			s.log.Debug("source rate not available yet", mlog.String("sessionID", s.cfg.SessionID))
			return
		}

		s.mut.RLock()
		s.bwEstimator.SetMaxBitrate(int(float64(sourceRate) * 1.5))
		s.mut.RUnlock()
	}

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case rate, ok := <-rateCh:
				if !ok {
					s.log.Info("rateCh was closed, returning", mlog.String("sessionID", s.cfg.SessionID))
					return
				}
				rateChangeHandler(rate)
			case <-ticker.C:
				updateMaxSourceRate()
			case <-s.closeCh:
				return
			}
		}
	}()
}

func (s *session) handleSenderBitrateChange(downRate int, lossRate int) (bool, int, string) {
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

	currSourceRate := screenSession.getSourceRate(currLevel)
	if currSourceRate <= 0 {
		s.log.Warn("current source rate not available yet", mlog.String("sessionID", s.cfg.SessionID))
		return false, 0, ""
	}

	newLevel := getSimulcastLevel(downRate, currSourceRate)
	if newLevel == currLevel {
		// no level change, nothing to do
		return false, 0, ""
	}

	// If the loss based rate estimation is greater than the source rate we avoid
	// potentially downgrading the level due to fluctuating delay rate estimation.
	if currLevel == SimulcastLevelHigh && lossRate > currSourceRate {
		s.log.Debug("skipping level downgrade, no loss", mlog.String("sessionID", s.cfg.SessionID))
		return false, 0, ""
	}

	newTrack := screenSession.getOutScreenTrack(newLevel)
	if newTrack == nil {
		// if the desired track is not available we keep the current one
		return false, 0, ""
	}

	sourceRate := screenSession.getSourceRate(newLevel)
	if sourceRate <= 0 {
		s.log.Warn("source rate not available", mlog.String("sessionID", s.cfg.SessionID))
		return false, 0, ""
	}

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
