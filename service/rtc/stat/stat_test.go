// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package stat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAvg(t *testing.T) {
	t.Run("uint8", func(t *testing.T) {
		t.Run("no samples", func(t *testing.T) {
			require.Equal(t, uint8(0), Avg([]uint8(nil)))
			require.Equal(t, uint8(0), Avg([]uint8{}))
		})

		t.Run("with samples", func(t *testing.T) {
			require.Equal(t, uint8(5), Avg([]uint8{
				2, 4, 4, 4, 5, 5, 7, 9,
			}))
		})

		t.Run("rounded", func(t *testing.T) {
			require.Equal(t, uint8(3), Avg([]uint8{
				1, 2, 3, 4,
			}))

			require.Equal(t, uint8(3), Avg([]uint8{
				1, 2, 3, 4,
			}))

			require.Equal(t, uint8(1), Avg([]uint8{
				9, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			}))

			require.Equal(t, uint8(2), Avg([]uint8{
				24, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			}))
		})
	})

	t.Run("int", func(t *testing.T) {
		t.Run("no samples", func(t *testing.T) {
			require.Equal(t, int(0), Avg([]int(nil)))
			require.Equal(t, int(0), Avg([]int{}))
		})

		t.Run("with samples", func(t *testing.T) {
			require.Equal(t, int(0), Avg([]int{
				-2, 2, -2, 2, -4, 4,
			}))
		})

		t.Run("rounded", func(t *testing.T) {
			require.Equal(t, int(-3), Avg([]int{
				-1, -2, -3, -4,
			}))

			require.Equal(t, int(-3), Avg([]int{
				-1, -2, -3, -4,
			}))

			require.Equal(t, int(-1), Avg([]int{
				-9, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			}))

			require.Equal(t, int(-2), Avg([]int{
				-24, 0, 0, 0, 0, 0, 0, 0, 0, 0,
			}))
		})
	})
}

func TestStdDev(t *testing.T) {
	t.Run("uint8", func(t *testing.T) {
		t.Run("no samples", func(t *testing.T) {
			require.Equal(t, uint8(0), StdDev([]uint8(nil), 0))
			require.Equal(t, uint8(0), StdDev([]uint8{}, 0))
		})

		t.Run("with samples", func(t *testing.T) {
			samples := []uint8{2, 4, 4, 4, 5, 5, 7, 9}
			require.Equal(t, uint8(2), StdDev(samples, Avg(samples)))
		})

		t.Run("rounded", func(t *testing.T) {
			samples := []uint8{2, 4, 9, 4, 5, 5, 7, 9}
			require.Equal(t, uint8(3), StdDev(samples, Avg(samples)))
		})
	})

	t.Run("int", func(t *testing.T) {
		t.Run("no samples", func(t *testing.T) {
			require.Equal(t, int(0), StdDev([]int(nil), 0))
			require.Equal(t, int(0), StdDev([]int{}, 0))
		})

		t.Run("with samples", func(t *testing.T) {
			samples := []int{-2, -4, -4, -4, -5, -5, -7, -9}
			require.Equal(t, int(2), StdDev(samples, Avg(samples)))
		})

		t.Run("rounded", func(t *testing.T) {
			samples := []int{-2, -4, -9, -4, -5, -5, -7, -9}
			require.Equal(t, int(3), StdDev(samples, Avg(samples)))
		})
	})
}
