// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsValid(t *testing.T) {
	t.Run("empty struct", func(t *testing.T) {
		var cfg Config
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid ReadBufferSize value: should be greater than zero", err.Error())
	})

	t.Run("invalid WriteBufferSize", func(t *testing.T) {
		var cfg Config
		cfg.ReadBufferSize = 1024
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid WriteBufferSize value: should be greater than zero", err.Error())
	})

	t.Run("valid", func(t *testing.T) {
		var cfg Config
		cfg.ReadBufferSize = 1024
		cfg.WriteBufferSize = 1024
		err := cfg.IsValid()
		require.NoError(t, err)
	})
}
