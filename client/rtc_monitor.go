// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"log/slog"
	"time"

	"github.com/pion/interceptor/pkg/stats"
	"github.com/pion/webrtc/v3"
)

type rtcMonitor struct {
	log         *slog.Logger
	pc          *webrtc.PeerConnection
	statsGetter stats.Getter
	interval    time.Duration

	lastSndStats map[webrtc.SSRC]*stats.Stats
	lastRcvStats map[webrtc.SSRC]*stats.Stats

	statsCh chan rtcStats
	stopCh  chan struct{}
	doneCh  chan struct{}
}

type rtcStats struct {
	lossRate float64
	jitter   float64
}

func newRTCMonitor(log *slog.Logger, pc *webrtc.PeerConnection, sg stats.Getter, intv time.Duration) *rtcMonitor {
	return &rtcMonitor{
		log:          log,
		pc:           pc,
		statsGetter:  sg,
		interval:     intv,
		statsCh:      make(chan rtcStats, 1),
		stopCh:       make(chan struct{}),
		doneCh:       make(chan struct{}),
		lastSndStats: make(map[webrtc.SSRC]*stats.Stats),
		lastRcvStats: make(map[webrtc.SSRC]*stats.Stats),
	}
}

func (m *rtcMonitor) gatherStats() (map[webrtc.SSRC]*stats.Stats, map[webrtc.SSRC]*stats.Stats) {
	sndStats := make(map[webrtc.SSRC]*stats.Stats)
	for _, snd := range m.pc.GetSenders() {
		if snd == nil {
			continue
		}
		for i, enc := range snd.GetParameters().Encodings {
			// For simplicity we only consider audio streams.
			// This lets us more easily make assumptions on the clock rate.
			if snd.GetParameters().Codecs[i].MimeType != webrtc.MimeTypeOpus {
				continue
			}

			stats := m.statsGetter.Get(uint32(enc.SSRC))
			if stats != nil {
				sndStats[enc.SSRC] = stats
			}
		}
	}

	rcvStats := make(map[webrtc.SSRC]*stats.Stats)
	for _, rcv := range m.pc.GetReceivers() {
		if rcv == nil {
			continue
		}

		track := rcv.Track()

		if track == nil {
			continue
		}

		// For simplicity we only consider audio streams.
		// This lets us more easily make assumptions on the clock rate.
		if track.Codec().MimeType != webrtc.MimeTypeOpus {
			continue
		}

		stats := m.statsGetter.Get(uint32(track.SSRC()))
		if stats != nil {
			rcvStats[track.SSRC()] = stats
		}
	}

	return sndStats, rcvStats
}

func (m *rtcMonitor) getAvgSenderStats(stats map[webrtc.SSRC]*stats.Stats) (avgLossRate, avgJitter, statsCount float64) {
	var totalJitter, totalLossRate float64

	for ssrc, s := range stats {
		if prevStats := m.lastSndStats[ssrc]; prevStats == nil || s.OutboundRTPStreamStats.PacketsSent == prevStats.OutboundRTPStreamStats.PacketsSent {
			continue
		}

		totalLossRate += s.RemoteInboundRTPStreamStats.FractionLost
		totalJitter += s.RemoteInboundRTPStreamStats.Jitter
		statsCount++
	}

	if statsCount > 0 {
		avgJitter = (totalJitter / statsCount)
		avgLossRate = totalLossRate / statsCount
	}

	return
}

func (m *rtcMonitor) getAvgReceiverStats(stats map[webrtc.SSRC]*stats.Stats) (avgLossRate, avgJitter, statsCount float64) {
	var totalJitter, totalLost, totalReceived float64

	for ssrc, s := range stats {
		prevStats := m.lastRcvStats[ssrc]
		if prevStats == nil || s.InboundRTPStreamStats.PacketsReceived == prevStats.InboundRTPStreamStats.PacketsReceived {
			continue
		}

		receivedDiff := s.InboundRTPStreamStats.PacketsReceived - prevStats.InboundRTPStreamStats.PacketsReceived
		potentiallyLost := int64(s.RemoteOutboundRTPStreamStats.PacketsSent) - int64(s.InboundRTPStreamStats.PacketsReceived)
		prevPotentiallyLost := int64(prevStats.RemoteOutboundRTPStreamStats.PacketsSent) - int64(prevStats.InboundRTPStreamStats.PacketsReceived)
		var lostDiff int64
		if prevPotentiallyLost >= 0 && potentiallyLost > prevPotentiallyLost {
			lostDiff = potentiallyLost - prevPotentiallyLost
		}
		totalLost += float64(lostDiff)
		totalReceived += float64(receivedDiff)
		totalJitter += s.InboundRTPStreamStats.Jitter

		statsCount++
	}

	if statsCount > 0 {
		avgJitter = (totalJitter / statsCount)
		avgLossRate = totalLost / totalReceived
	}

	return
}

func (m *rtcMonitor) processStats(sndStats, rcvStats map[webrtc.SSRC]*stats.Stats) {
	defer func() {
		// cache stats for the next iteration
		m.lastSndStats = sndStats
		m.lastRcvStats = rcvStats
	}()

	sndLossRate, sndJitter, sndCnt := m.getAvgSenderStats(sndStats)
	rcvLossRate, rcvJitter, rcvCnt := m.getAvgReceiverStats(rcvStats)

	// nothing to do if we didn't process any stats
	if sndCnt == 0 && rcvCnt == 0 {
		return
	}

	select {
	case m.statsCh <- rtcStats{lossRate: max(sndLossRate, rcvLossRate), jitter: max(sndJitter, rcvJitter)}:
	default:
		m.log.Error("failed to send stats: channel is full")
	}
}

func (m *rtcMonitor) Start() {
	m.log.Debug("starting rtc monitor")
	go func() {
		defer close(m.doneCh)
		ticker := time.NewTicker(m.interval)

		for {
			select {
			case <-ticker.C:
				sndStats, rcvStats := m.gatherStats()
				m.processStats(sndStats, rcvStats)
			case <-m.stopCh:
				return
			}
		}
	}()
}

func (m *rtcMonitor) Stop() {
	m.log.Debug("stopping rtc monitor")
	close(m.stopCh)
	<-m.doneCh
	close(m.statsCh)
}

func (m *rtcMonitor) StatsCh() <-chan rtcStats {
	return m.statsCh
}
