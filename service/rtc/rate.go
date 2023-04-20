// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"fmt"
	"math"
	"sync"
	"time"
)

type sample struct {
	ts   time.Time
	size int
}

type RateMonitor struct {
	samples      []sample
	samplesPtr   int
	samplingSize time.Duration
	filled       bool
	now          func() time.Time
	mut          sync.RWMutex
}

func NewRateMonitor(samplingSize time.Duration, now func() time.Time) (*RateMonitor, error) {
	if samplingSize <= 0 {
		return nil, fmt.Errorf("invalid sampling size")
	}

	if now == nil {
		now = time.Now
	}

	return &RateMonitor{
		now:          now,
		samplingSize: samplingSize,
		samples:      make([]sample, 0),
	}, nil
}

func (m *RateMonitor) PushSample(size int) {
	m.mut.Lock()
	defer m.mut.Unlock()

	// Filling up to double the sampling size to make sure we have enough samples
	// to calculate the desired duration since at the beginning it's likely we get
	// a burst of packets.
	if !m.filled {
		m.samples = append(m.samples, sample{ts: m.now(), size: size})
		m.samplesPtr++
		if m.getSamplesDuration() >= m.samplingSize*2 {
			m.filled = true
		}
		return
	}

	m.samples[m.samplesPtr%len(m.samples)] = sample{ts: m.now(), size: size}
	m.samplesPtr++
}

func (m *RateMonitor) GetSamplesDuration() time.Duration {
	m.mut.RLock()
	defer m.mut.RUnlock()

	return m.getSamplesDuration()
}

func (m *RateMonitor) getSamplesDuration() time.Duration {
	if len(m.samples) == 0 {
		return 0
	}

	lastTS := m.samples[(m.samplesPtr-1)%len(m.samples)].ts
	firstTS := m.samples[m.samplesPtr%len(m.samples)].ts

	return lastTS.Sub(firstTS)
}

func (m *RateMonitor) GetRate() (int, time.Duration) {
	m.mut.RLock()
	defer m.mut.RUnlock()

	if !m.filled {
		return -1, 0
	}

	now := m.now()

	var totalBytes int
	var samplesDuration time.Duration
	for i := m.samplesPtr - 1; i >= m.samplesPtr-len(m.samples); i-- {
		sample := m.samples[i%len(m.samples)]
		samplesDuration = now.Sub(sample.ts)
		totalBytes += sample.size

		if samplesDuration >= m.samplingSize {
			break
		}
	}

	bitsPerSec := math.Round((float64(totalBytes) / samplesDuration.Seconds()) * 8)

	return int(bitsPerSec), samplesDuration
}
