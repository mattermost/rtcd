// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/pion/interceptor/pkg/stats"
	"github.com/pion/webrtc/v3"

	"github.com/stretchr/testify/require"
)

type statsGetter struct{}

func (sg *statsGetter) Get(_ uint32) *stats.Stats {
	return nil
}

func TestRTCMonitor(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	defer pc.Close()

	var sg statsGetter
	rtcMon := newRTCMonitor(logger, pc, &sg, time.Second)
	require.NotNil(t, rtcMon)

	rtcMon.Start()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s := <-rtcMon.StatsCh()
		require.Empty(t, s)
	}()

	time.Sleep(2 * time.Second)

	rtcMon.Stop()
	wg.Wait()
}

func TestRTCMonitorProcessStats(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}))

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	defer pc.Close()

	var sg statsGetter
	rtcMon := newRTCMonitor(logger, pc, &sg, time.Second)
	require.NotNil(t, rtcMon)

	rtcMon.lastSndStats = map[webrtc.SSRC]*stats.Stats{
		45454545: {
			RemoteInboundRTPStreamStats: stats.RemoteInboundRTPStreamStats{},
			OutboundRTPStreamStats: stats.OutboundRTPStreamStats{
				SentRTPStreamStats: stats.SentRTPStreamStats{
					PacketsSent: 45,
				},
			},
		},
	}
	rtcMon.lastRcvStats = map[webrtc.SSRC]*stats.Stats{
		45454545: {
			InboundRTPStreamStats: stats.InboundRTPStreamStats{
				ReceivedRTPStreamStats: stats.ReceivedRTPStreamStats{
					PacketsReceived: 45,
				},
			},
		},
	}
	rtcMon.processStats(map[webrtc.SSRC]*stats.Stats{
		45454545: {
			RemoteInboundRTPStreamStats: stats.RemoteInboundRTPStreamStats{
				FractionLost: 0.45,
				ReceivedRTPStreamStats: stats.ReceivedRTPStreamStats{
					Jitter: 0.4545,
				},
			},
			OutboundRTPStreamStats: stats.OutboundRTPStreamStats{
				SentRTPStreamStats: stats.SentRTPStreamStats{
					PacketsSent: 4545,
				},
			},
		},
	}, map[webrtc.SSRC]*stats.Stats{
		45454545: {
			InboundRTPStreamStats: stats.InboundRTPStreamStats{
				ReceivedRTPStreamStats: stats.ReceivedRTPStreamStats{
					PacketsReceived: 4545,
				},
			},
		},
	})

	select {
	case stats := <-rtcMon.StatsCh():
		require.Equal(t, rtcStats{
			lossRate: 0.45,
			jitter:   0.4545,
		}, stats)
	default:
		require.Fail(t, "channel should have stats")
	}
}
