// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/mattermost/rtcd/service/rtc/stat"
)

type RateMonitor struct {
	samples      []int
	timestamps   []time.Time
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
		samples:      make([]int, 0),
		timestamps:   make([]time.Time, 0),
	}, nil
}

func (m *RateMonitor) PushSample(size int) {
	m.mut.Lock()
	defer m.mut.Unlock()

	if !m.filled && m.getSamplesDuration() < m.samplingSize {
		m.samples = append(m.samples, size)
		m.timestamps = append(m.timestamps, m.now())
		m.samplesPtr++
		if m.getSamplesDuration() >= m.samplingSize {
			m.filled = true
		}
		return
	}

	m.samples[m.samplesPtr%len(m.samples)] = size
	m.timestamps[m.samplesPtr%len(m.timestamps)] = m.now()
	m.samplesPtr++
}

func (m *RateMonitor) GetSamplesDuration() time.Duration {
	m.mut.RLock()
	defer m.mut.RUnlock()

	return m.getSamplesDuration()
}

func (m *RateMonitor) getSamplesDuration() time.Duration {
	if len(m.timestamps) == 0 {
		return 0
	}

	lastTS := m.timestamps[(m.samplesPtr-1)%len(m.timestamps)]
	firstTS := m.timestamps[m.samplesPtr%len(m.timestamps)]

	return lastTS.Sub(firstTS)
}

func (m *RateMonitor) GetRate() int {
	m.mut.RLock()
	defer m.mut.RUnlock()

	if !m.filled {
		return -1
	}

	totalBytes := stat.Sum(m.samples)
	samplesDuration := m.getSamplesDuration()
	bitsPerSec := math.Round((totalBytes / samplesDuration.Seconds()) * 8)

	return int(bitsPerSec)
}
