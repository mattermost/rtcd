// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package stat

import (
	"math"
)

type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~float32 | ~float64
}

func Sum[T Number](samples []T) float64 {
	var total float64
	for _, sample := range samples {
		total += float64(sample)
	}
	return total
}

func Avg[T Number](samples []T) T {
	if len(samples) == 0 {
		return 0
	}

	return T(math.Round(Sum(samples) / float64(len(samples))))
}

func StdDev[T Number](samples []T, avg T) T {
	if len(samples) == 0 {
		return 0
	}

	var total float64
	for _, sample := range samples {
		total += math.Pow(float64(sample)-float64(avg), 2)
	}

	// Applying Bessel's correction as we are dealing with just a subset of samples.
	return T(math.Round(math.Sqrt(total / float64(len(samples)-1))))
}
