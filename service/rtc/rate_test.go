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
		samplingSize := 100
		rm, err := NewRateMonitor(100, nil)
		require.NoError(t, err)
		require.NotNil(t, rm)

		for i := 0; i < samplingSize-1; i++ {
			rm.PushSample(1000)
		}

		require.Equal(t, -1, rm.GetRate())
	})

	t.Run("invalid timestamps", func(t *testing.T) {
		samplingSize := 100

		tt := time.Now()
		now := func() time.Time {
			return tt
		}

		rm, err := NewRateMonitor(samplingSize, now)
		require.NoError(t, err)
		require.NotNil(t, rm)

		for i := 0; i < samplingSize; i++ {
			rm.PushSample(1000)
		}

		require.Equal(t, -1, rm.GetRate())
	})

	t.Run("expected rate", func(t *testing.T) {
		samplingSize := 100

		tt := time.Now()
		now := func() time.Time {
			tt = tt.Add(time.Millisecond * 1000)
			return tt
		}

		rm, err := NewRateMonitor(samplingSize, now)
		require.NoError(t, err)
		require.NotNil(t, rm)

		for i := 0; i < samplingSize; i++ {
			if i != 0 {
				rm.PushSample(1000)
			} else {
				rm.PushSample(0)
			}
		}

		require.Len(t, rm.samples, samplingSize)
		require.Len(t, rm.timestamps, samplingSize)
		require.Equal(t, samplingSize, rm.samplesPtr)

		require.Equal(t, 8, rm.GetRate())

		rm, err = NewRateMonitor(samplingSize+1, now)
		require.NoError(t, err)
		require.NotNil(t, rm)

		for i := 0; i < samplingSize+1; i++ {
			if i%2 == 0 {
				rm.PushSample(0)
			} else {
				rm.PushSample(1000)
			}
		}

		require.Equal(t, 4, rm.GetRate())
	})
}
