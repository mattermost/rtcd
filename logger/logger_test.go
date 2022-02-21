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

func TestIsValid(t *testing.T) {
	t.Run("empty struct", func(t *testing.T) {
		var settings Settings
		err := settings.IsValid()
		require.Error(t, err)
		require.Equal(t, "should enable at least one logging target", err.Error())
	})

	t.Run("ConsoleLevel", func(t *testing.T) {
		var settings Settings
		settings.EnableConsole = true
		err := settings.IsValid()
		require.Error(t, err)

		settings.ConsoleLevel = "invalid"
		err = settings.IsValid()
		require.Error(t, err)
		require.Equal(t, `invalid ConsoleLevel value "invalid"`, err.Error())

		settings.ConsoleLevel = "INFO"
		err = settings.IsValid()
		require.NoError(t, err)
	})

	t.Run("FileLevel", func(t *testing.T) {
		var settings Settings
		settings.EnableFile = true
		settings.FileLocation = "rtcd.log"
		err := settings.IsValid()
		require.Error(t, err)

		settings.FileLevel = "invalid"
		err = settings.IsValid()
		require.Error(t, err)
		require.Equal(t, `invalid FileLevel value "invalid"`, err.Error())

		settings.FileLevel = "INFO"
		err = settings.IsValid()
		require.NoError(t, err)
	})

	t.Run("FileLocation", func(t *testing.T) {
		var settings Settings
		settings.EnableFile = true
		settings.FileLevel = "DEBUG"
		err := settings.IsValid()
		require.Error(t, err)
		require.Equal(t, `invalid FileLocation value: should not be empty`, err.Error())

		settings.FileLocation = "rtcd.log"
		err = settings.IsValid()
		require.NoError(t, err)
	})
}

func TestNewLogger(t *testing.T) {
	t.Run("empty settings", func(t *testing.T) {
		var settings Settings
		logger, err := New(settings)
		require.Nil(t, logger)
		require.Error(t, err)
	})

	t.Run("invalid settings", func(t *testing.T) {
		var settings Settings
		settings.EnableConsole = true
		settings.ConsoleLevel = "INVALID"
		logger, err := New(settings)
		require.Nil(t, logger)
		require.Error(t, err)
		require.Equal(t, `invalid ConsoleLevel value "INVALID"`, err.Error())
	})

	t.Run("valid settings", func(t *testing.T) {
		var settings Settings
		settings.EnableConsole = true
		settings.ConsoleLevel = "INFO"
		logger, err := New(settings)
		require.NoError(t, err)
		require.NotNil(t, logger)
	})
}
