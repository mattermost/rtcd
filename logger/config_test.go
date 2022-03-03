// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package logger

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsValid(t *testing.T) {
	t.Run("empty struct", func(t *testing.T) {
		var cfg Config
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "should enable at least one logging target", err.Error())
	})

	t.Run("ConsoleLevel", func(t *testing.T) {
		var cfg Config
		cfg.EnableConsole = true
		err := cfg.IsValid()
		require.Error(t, err)

		cfg.ConsoleLevel = "invalid"
		err = cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, `invalid ConsoleLevel value "invalid"`, err.Error())

		cfg.ConsoleLevel = "INFO"
		err = cfg.IsValid()
		require.NoError(t, err)
	})

	t.Run("FileLevel", func(t *testing.T) {
		var cfg Config
		cfg.EnableFile = true
		cfg.FileLocation = "rtcd.log"
		err := cfg.IsValid()
		require.Error(t, err)

		cfg.FileLevel = "invalid"
		err = cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, `invalid FileLevel value "invalid"`, err.Error())

		cfg.FileLevel = "INFO"
		err = cfg.IsValid()
		require.NoError(t, err)
	})

	t.Run("FileLocation", func(t *testing.T) {
		var cfg Config
		cfg.EnableFile = true
		cfg.FileLevel = "DEBUG"
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, `invalid FileLocation value: should not be empty`, err.Error())

		cfg.FileLocation = "rtcd.log"
		err = cfg.IsValid()
		require.NoError(t, err)
	})
}
