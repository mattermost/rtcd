// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package random

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewSecureString(t *testing.T) {
	str, err := NewSecureString(0)
	require.NoError(t, err)
	require.Empty(t, str)

	for i := 1; i <= 1024; i++ {
		str, err = NewSecureString(i)
		require.NoError(t, err)
		require.NotEmpty(t, str)
		require.Len(t, str, i)
	}
}
