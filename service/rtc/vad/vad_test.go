// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package vad

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewMonitor(t *testing.T) {
	defaultCfg := (MonitorConfig{}).SetDefaults()

	t.Run("invalid config", func(t *testing.T) {
		m, err := NewMonitor(MonitorConfig{}, func(voice bool) {})
		require.EqualError(t, err, "invalid config: VoiceLevelsSampleSize should be > 0")
		require.Nil(t, m)
	})

	t.Run("missing callback", func(t *testing.T) {
		m, err := NewMonitor(defaultCfg, nil)
		require.EqualError(t, err, "voice event callback is required")
		require.Nil(t, m)
	})

	t.Run("default config", func(t *testing.T) {
		m, err := NewMonitor(defaultCfg, func(voice bool) {})
		require.NoError(t, err)
		require.NotNil(t, m)

		require.Empty(t, m.voiceLevelsSample)
		require.Equal(t, defaultCfg, m.cfg)
	})

	t.Run("custom config", func(t *testing.T) {
		cfg := MonitorConfig{
			VoiceLevelsSampleSize:      defaultVoiceLevelsSampleSize * 2,
			ActivationDuration:         defaultActivationDuration * 2,
			VoiceActivationThreshold:   defaultVoiceActivationThreshold * 2,
			VoiceDeactivationThreshold: defaultVoiceDeactivationThreshold * 2,
		}
		m, err := NewMonitor(cfg, func(voice bool) {})
		require.NoError(t, err)
		require.NotNil(t, m)

		require.Equal(t, cfg, m.cfg)
	})
}

func TestGetAvg(t *testing.T) {
	t.Run("no samples", func(t *testing.T) {
		require.Equal(t, uint8(0), getAvg(nil))
		require.Equal(t, uint8(0), getAvg([]uint8{}))
	})

	t.Run("with samples", func(t *testing.T) {
		require.Equal(t, uint8(5), getAvg([]uint8{
			2, 4, 4, 4, 5, 5, 7, 9,
		}))
	})

	t.Run("rounded", func(t *testing.T) {
		require.Equal(t, uint8(3), getAvg([]uint8{
			1, 2, 3, 4,
		}))

		require.Equal(t, uint8(3), getAvg([]uint8{
			1, 2, 3, 4,
		}))

		require.Equal(t, uint8(1), getAvg([]uint8{
			9, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		}))

		require.Equal(t, uint8(2), getAvg([]uint8{
			24, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		}))
	})
}

func TestGetStdDev(t *testing.T) {
	t.Run("no samples", func(t *testing.T) {
		require.Equal(t, uint8(0), getStdDev(nil, 0))
		require.Equal(t, uint8(0), getStdDev([]uint8{}, 0))
	})

	t.Run("with samples", func(t *testing.T) {
		samples := []uint8{2, 4, 4, 4, 5, 5, 7, 9}
		require.Equal(t, uint8(2), getStdDev(samples, getAvg(samples)))
	})

	t.Run("rounded", func(t *testing.T) {
		samples := []uint8{2, 4, 9, 4, 5, 5, 7, 9}
		require.Equal(t, uint8(3), getStdDev(samples, getAvg(samples)))
	})
}

func TestPushAudioLevel(t *testing.T) {
	cfg := MonitorConfig{
		VoiceLevelsSampleSize:      10,
		ActivationDuration:         250 * time.Millisecond,
		VoiceActivationThreshold:   10,
		VoiceDeactivationThreshold: 5,
	}

	var voiceOn bool
	var voiceOff bool
	cb := func(voice bool) {
		if voice {
			voiceOn = true
		} else {
			voiceOff = true
		}
	}

	m, err := NewMonitor(cfg, cb)
	require.NoError(t, err)
	require.NotNil(t, m)
	require.Empty(t, m.voiceLevelsSample)
	require.False(t, voiceOn)
	require.False(t, voiceOff)

	// We fill the voice levels sample.
	for i := 0; i < cfg.VoiceLevelsSampleSize; i++ {
		if (i % 2) == 0 {
			m.PushAudioLevel(55)
		} else {
			m.PushAudioLevel(45)
		}
	}

	require.Equal(t, uint8(50), getAvg(m.voiceLevelsSample))
	require.Equal(t, uint8(5), getStdDev(m.voiceLevelsSample, getAvg(m.voiceLevelsSample)))
	require.False(t, m.voiceState)
	require.False(t, voiceOn)
	require.False(t, voiceOff)

	// Pushing some higher levels to trigger voice detection.
	m.PushAudioLevel(30)
	m.PushAudioLevel(30)
	m.PushAudioLevel(30)
	m.PushAudioLevel(30)

	require.True(t, m.voiceState)
	require.True(t, voiceOn)
	require.False(t, voiceOff)

	// Pushing some lower levels to trigger voice detection.
	m.PushAudioLevel(40)
	m.PushAudioLevel(40)
	m.PushAudioLevel(40)
	m.PushAudioLevel(40)
	m.PushAudioLevel(40)
	m.PushAudioLevel(40)
	m.PushAudioLevel(40)
	m.PushAudioLevel(40)

	require.True(t, m.voiceState)
	require.False(t, voiceOff)
	time.Sleep(cfg.ActivationDuration)
	m.PushAudioLevel(40)

	require.True(t, voiceOff)
	require.False(t, m.voiceState)
}
