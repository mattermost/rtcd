// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetRate(t *testing.T) {
	t.Run("not enough samples", func(t *testing.T) {
		samplingSize := time.Second
		rm, err := NewRateMonitor(samplingSize, nil)
		require.NoError(t, err)
		require.NotNil(t, rm)

		rm.PushSample(1000)
		rm.PushSample(1000)
		rm.PushSample(1000)

		rate, dur := rm.GetRate()

		require.Equal(t, -1, rate)
		require.Equal(t, time.Duration(0), dur)
	})

	t.Run("invalid timestamps", func(t *testing.T) {
		samplingSize := time.Second

		tt := time.Now()
		now := func() time.Time {
			return tt
		}

		rm, err := NewRateMonitor(samplingSize, now)
		require.NoError(t, err)
		require.NotNil(t, rm)

		rm.PushSample(1000)

		rate, dur := rm.GetRate()

		require.Equal(t, -1, rate)
		require.Equal(t, time.Duration(0), dur)
	})

	t.Run("expected rate", func(t *testing.T) {
		samplingSize := time.Second

		tt := time.Now()
		now := func() time.Time {
			tt = tt.Add(time.Millisecond * 100)
			return tt
		}

		rm, err := NewRateMonitor(samplingSize, now)
		require.NoError(t, err)
		require.NotNil(t, rm)

		for i := 0; i < 22; i++ {
			rm.PushSample(1000)
		}

		require.Equal(t, samplingSize*2, rm.getSamplesDuration())

		require.Len(t, rm.samples, 21)
		require.Equal(t, 22, rm.samplesPtr)

		rate, dur := rm.GetRate()
		require.Equal(t, 80000, rate)
		require.Equal(t, samplingSize, dur)

		rm, err = NewRateMonitor(time.Second, now)
		require.NoError(t, err)
		require.NotNil(t, rm)

		for i := 0; i < 22; i++ {
			if i%2 == 0 {
				rm.PushSample(0)
			} else {
				rm.PushSample(1000)
			}
		}

		rate, dur = rm.GetRate()
		require.Equal(t, 40000, rate)
		require.Equal(t, samplingSize, dur)
	})
}

func TestGetSamplesDuration(t *testing.T) {
	i := 0
	tt := time.Now()
	now := func() time.Time {
		i++
		return tt.Add(time.Duration(i) * time.Second)
	}

	samplingSize := 2 * time.Second
	rm, err := NewRateMonitor(samplingSize, now)
	require.NoError(t, err)
	require.NotNil(t, rm)

	rm.PushSample(1000)
	require.Equal(t, 1, rm.samplesPtr)

	dur := rm.GetSamplesDuration()
	require.Equal(t, time.Duration(0), dur)

	rm.PushSample(1000)
	rm.PushSample(1000)
	rm.PushSample(1000)
	require.Equal(t, 4, rm.samplesPtr)
	require.False(t, rm.filled)

	dur = rm.GetSamplesDuration()
	require.Equal(t, 3*time.Second, dur)

	rm.PushSample(1000)
	require.Equal(t, 5, rm.samplesPtr)
	require.True(t, rm.filled)

	dur = rm.GetSamplesDuration()
	require.Equal(t, 4*time.Second, dur)

	rm.PushSample(1000)
	rm.PushSample(1000)
	require.Equal(t, 7, rm.samplesPtr)

	dur = rm.GetSamplesDuration()
	require.Equal(t, 4*time.Second, dur)
}
