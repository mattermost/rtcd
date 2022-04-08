// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package random

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewID(t *testing.T) {
	for i := 0; i < 1000; i++ {
		id := NewID()
		require.Equal(t, len(id), 26)
		for _, c := range id {
			require.Contains(t, []rune(charset), c)
		}
	}
}
