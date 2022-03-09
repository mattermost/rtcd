// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewId(t *testing.T) {
	for i := 0; i < 1000; i++ {
		id := newID()
		require.Equal(t, len(id), 26)
		for _, c := range id {
			require.Contains(t, []rune(charset), c)
		}
	}
}
