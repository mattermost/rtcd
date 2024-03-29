// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package vad

import (
	"fmt"
	"time"

	"github.com/mattermost/rtcd/service/rtc/stat"
)

const (
	defaultVoiceLevelsSampleSize      = 50
	defaultVoiceActivationThreshold   = 10
	defaultVoiceDeactivationThreshold = 4
	defaultActivationDuration         = 2 * time.Second
)

type VoiceCB func(voice bool)

type Monitor struct {
	cfg MonitorConfig

	voiceLevelsSample    []uint8
	voiceLevelsSamplePtr int
	lastActivationTime   time.Time
	voiceState           bool
	cb                   VoiceCB
}

type MonitorConfig struct {
	VoiceLevelsSampleSize      int
	ActivationDuration         time.Duration
	VoiceActivationThreshold   int
	VoiceDeactivationThreshold int
}

func (c MonitorConfig) SetDefaults() MonitorConfig {
	if c.VoiceLevelsSampleSize == 0 {
		c.VoiceLevelsSampleSize = defaultVoiceLevelsSampleSize
	}

	if c.ActivationDuration == 0 {
		c.ActivationDuration = defaultActivationDuration
	}

	if c.VoiceActivationThreshold == 0 {
		c.VoiceActivationThreshold = defaultVoiceActivationThreshold
	}

	if c.VoiceDeactivationThreshold == 0 {
		c.VoiceDeactivationThreshold = defaultVoiceDeactivationThreshold
	}

	return c
}

func (c MonitorConfig) IsValid() error {
	if c.VoiceLevelsSampleSize <= 0 {
		return fmt.Errorf("VoiceLevelsSampleSize should be > 0")
	}

	if c.ActivationDuration <= 0 {
		return fmt.Errorf("ActivationDuration should be > 0")
	}

	if c.VoiceActivationThreshold <= 0 {
		return fmt.Errorf("VoiceActivationThreshold should be > 0")
	}

	if c.VoiceDeactivationThreshold <= 0 {
		return fmt.Errorf("VoiceDeactivationThreshold should be > 0")
	}

	return nil
}

func NewMonitor(cfg MonitorConfig, cb VoiceCB) (*Monitor, error) {
	if err := cfg.IsValid(); err != nil {
		return nil, fmt.Errorf("invalid config: %s", err)
	}

	if cb == nil {
		return nil, fmt.Errorf("voice event callback is required")
	}

	return &Monitor{
		cfg:               cfg,
		voiceLevelsSample: make([]uint8, 0, cfg.VoiceLevelsSampleSize),
		cb:                cb,
	}, nil
}

func (m *Monitor) PushAudioLevel(level uint8) {
	if len(m.voiceLevelsSample) < m.cfg.VoiceLevelsSampleSize {
		m.voiceLevelsSample = append(m.voiceLevelsSample, level)
		return
	}

	m.voiceLevelsSample[m.voiceLevelsSamplePtr] = level
	if m.voiceLevelsSamplePtr == (m.cfg.VoiceLevelsSampleSize - 1) {
		m.voiceLevelsSamplePtr = 0
	} else {
		m.voiceLevelsSamplePtr++
	}

	avg := stat.Avg(m.voiceLevelsSample)
	dev := stat.StdDev(m.voiceLevelsSample, avg)

	var newState bool
	if !m.voiceState && int(dev) > m.cfg.VoiceActivationThreshold {
		newState = true
		m.lastActivationTime = time.Now()
	} else if m.voiceState && int(dev) < m.cfg.VoiceDeactivationThreshold {
		newState = false
	} else {
		return
	}

	if newState == m.voiceState || (!newState && time.Since(m.lastActivationTime) < m.cfg.ActivationDuration) {
		return
	}

	m.cb(newState)
	m.voiceState = newState
}

func (m *Monitor) Reset() {
	m.voiceLevelsSamplePtr = 0
	m.voiceLevelsSample = m.voiceLevelsSample[:0]
	m.lastActivationTime = time.Time{}
	m.voiceState = false
	m.cb(false)
}
