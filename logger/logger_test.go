// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package logger

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

func TestGetLevels(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		levels := getLevels("")
		require.Equal(t, levels, mlog.StdAll)
	})

	t.Run("invalid input", func(t *testing.T) {
		levels := getLevels("invalid")
		require.Equal(t, levels, mlog.StdAll)
	})

	t.Run("debug", func(t *testing.T) {
		levels := getLevels("DEBUG")
		require.Equal(t, []mlog.Level{
			mlog.LvlPanic,
			mlog.LvlFatal,
			mlog.LvlError,
			mlog.LvlWarn,
			mlog.LvlInfo,
			mlog.LvlDebug,
		}, levels)
	})

	t.Run("info", func(t *testing.T) {
		levels := getLevels("INFO")
		require.Equal(t, []mlog.Level{
			mlog.LvlPanic,
			mlog.LvlFatal,
			mlog.LvlError,
			mlog.LvlWarn,
			mlog.LvlInfo,
		}, levels)
	})

	t.Run("error", func(t *testing.T) {
		levels := getLevels("ERROR")
		require.Equal(t, []mlog.Level{
			mlog.LvlPanic,
			mlog.LvlFatal,
			mlog.LvlError,
		}, levels)
	})
}

func TestNewLogger(t *testing.T) {
	t.Run("empty cfg", func(t *testing.T) {
		var cfg Config
		logger, err := New(cfg)
		require.Nil(t, logger)
		require.Error(t, err)
	})

	t.Run("invalid cfg", func(t *testing.T) {
		var cfg Config
		cfg.EnableConsole = true
		cfg.ConsoleLevel = "INVALID"
		logger, err := New(cfg)
		require.Nil(t, logger)
		require.Error(t, err)
		require.Equal(t, `invalid ConsoleLevel value "INVALID"`, err.Error())
	})

	t.Run("valid cfg", func(t *testing.T) {
		var cfg Config
		cfg.EnableConsole = true
		cfg.ConsoleLevel = "INFO"
		logger, err := New(cfg)
		require.NoError(t, err)
		require.NotNil(t, logger)
	})
}
