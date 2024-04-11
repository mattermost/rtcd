// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package vad

import (
	"testing"
	"time"

	"github.com/mattermost/rtcd/service/rtc/stat"

	"github.com/stretchr/testify/require"
)

func TestNewMonitor(t *testing.T) {
	defaultCfg := (MonitorConfig{}).SetDefaults()

	t.Run("invalid config", func(t *testing.T) {
		m, err := NewMonitor(MonitorConfig{}, func(_ bool) {})
		require.EqualError(t, err, "invalid config: VoiceLevelsSampleSize should be > 0")
		require.Nil(t, m)
	})

	t.Run("missing callback", func(t *testing.T) {
		m, err := NewMonitor(defaultCfg, nil)
		require.EqualError(t, err, "voice event callback is required")
		require.Nil(t, m)
	})

	t.Run("default config", func(t *testing.T) {
		m, err := NewMonitor(defaultCfg, func(_ bool) {})
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
		m, err := NewMonitor(cfg, func(_ bool) {})
		require.NoError(t, err)
		require.NotNil(t, m)

		require.Equal(t, cfg, m.cfg)
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

	require.Equal(t, uint8(50), stat.Avg(m.voiceLevelsSample))
	require.Equal(t, uint8(5), stat.StdDev(m.voiceLevelsSample, stat.Avg(m.voiceLevelsSample)))
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

func TestReset(t *testing.T) {
	cfg := MonitorConfig{
		VoiceLevelsSampleSize:      10,
		ActivationDuration:         250 * time.Millisecond,
		VoiceActivationThreshold:   10,
		VoiceDeactivationThreshold: 5,
	}

	var activated bool
	var deactivated bool
	m, err := NewMonitor(cfg, func(voice bool) {
		if voice {
			activated = true
		} else {
			deactivated = true
		}
	})
	require.NoError(t, err)
	require.NotNil(t, m)
	require.Empty(t, m.voiceLevelsSample)
	require.Zero(t, m.voiceLevelsSamplePtr)
	require.False(t, m.voiceState)
	require.Zero(t, m.lastActivationTime)

	for i := 0; i < cfg.VoiceLevelsSampleSize; i++ {
		m.PushAudioLevel(45)
	}

	require.False(t, activated)
	require.False(t, deactivated)

	m.PushAudioLevel(100)

	require.True(t, m.voiceState)
	require.NotZero(t, m.lastActivationTime)
	require.Len(t, m.voiceLevelsSample, cfg.VoiceLevelsSampleSize)
	require.Equal(t, 1, m.voiceLevelsSamplePtr)
	require.True(t, activated)
	require.False(t, deactivated)

	m.Reset()

	require.Empty(t, m.voiceLevelsSample)
	require.Zero(t, m.voiceLevelsSamplePtr)
	require.False(t, m.voiceState)
	require.Zero(t, m.lastActivationTime)
	require.True(t, activated)
	require.True(t, deactivated)
}
