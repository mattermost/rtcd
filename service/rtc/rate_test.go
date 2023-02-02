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

		require.Equal(t, -1, rm.GetRate())
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

		require.Equal(t, -1, rm.GetRate())
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

		for i := 0; i < 11; i++ {
			if i > 0 {
				rm.PushSample(1000)
			} else {
				rm.PushSample(0)
			}
		}

		require.Equal(t, samplingSize, rm.getSamplesDuration())

		require.Len(t, rm.samples, 11)
		require.Len(t, rm.timestamps, 11)
		require.Equal(t, 11, rm.samplesPtr)

		require.Equal(t, 80000, rm.GetRate())

		rm, err = NewRateMonitor(time.Second, now)
		require.NoError(t, err)
		require.NotNil(t, rm)

		for i := 0; i < 11; i++ {
			if i%2 == 0 {
				rm.PushSample(0)
			} else {
				rm.PushSample(1000)
			}
		}

		require.Equal(t, 40000, rm.GetRate())
	})
}
