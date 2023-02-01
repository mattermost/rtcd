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
	samplingSize int
	now          func() time.Time
	mut          sync.RWMutex
}

func NewRateMonitor(samplingSize int, now func() time.Time) (*RateMonitor, error) {
	if samplingSize < 1 {
		return nil, fmt.Errorf("invalid sampling size")
	}

	if now == nil {
		now = time.Now
	}

	return &RateMonitor{
		now:          now,
		samplingSize: samplingSize,
		samples:      make([]int, 0, samplingSize),
		timestamps:   make([]time.Time, 0, samplingSize),
	}, nil
}

func (m *RateMonitor) PushSample(size int) {
	m.mut.Lock()
	defer m.mut.Unlock()

	if len(m.samples) < m.samplingSize {
		m.samples = append(m.samples, size)
		m.timestamps = append(m.timestamps, m.now())
		m.samplesPtr++
		return
	}

	m.samples[m.samplesPtr%m.samplingSize] = size
	m.timestamps[m.samplesPtr%m.samplingSize] = m.now()
	m.samplesPtr++
}

func (m *RateMonitor) GetSamplesDuration() time.Duration {
	m.mut.RLock()
	defer m.mut.RUnlock()

	if len(m.samples) < m.samplingSize {
		return -1
	}

	lastTS := m.timestamps[(m.samplesPtr-1)%m.samplingSize]
	firstTS := m.timestamps[m.samplesPtr%m.samplingSize]

	return lastTS.Sub(firstTS)
}

func (m *RateMonitor) GetRate() int {
	m.mut.RLock()
	defer m.mut.RUnlock()

	if len(m.samples) < m.samplingSize {
		return -1
	}

	totalBytes := stat.Sum(m.samples)
	samplesDuration := m.GetSamplesDuration()

	if samplesDuration <= 0 {
		return -1
	}

	kbitsPerSec := (totalBytes / float64(samplesDuration.Milliseconds())) * 8

	return int(math.Round(kbitsPerSec))
}
