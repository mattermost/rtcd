// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package logger

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

// Settings holds information used to initialize a new logger.
type Settings struct {
	EnableConsole bool   `toml:"enable_console"`
	ConsoleJSON   bool   `toml:"console_json"`
	ConsoleLevel  string `toml:"console_level"`
	EnableFile    bool   `toml:"enable_file"`
	FileJSON      bool   `toml:"file_json"`
	FileLevel     string `toml:"file_level"`
	FileLocation  string `toml:"file_location"`
	EnableColor   bool   `toml:"enable_color"`
}

func getLevels(level string) []mlog.Level {
	var levels []mlog.Level
	for _, l := range mlog.StdAll {
		levels = append(levels, l)
		if l.Name == strings.ToLower(level) {
			break
		}
	}
	return levels
}

func (s Settings) IsValid() error {
	if !s.EnableConsole && !s.EnableFile {
		return fmt.Errorf("should enable at least one logging target")
	}
	var foundConsoleLevel bool
	var foundFileLevel bool
	for _, l := range mlog.StdAll {
		if strings.ToLower(s.ConsoleLevel) == l.Name {
			foundConsoleLevel = true
		}
		if strings.ToLower(s.FileLevel) == l.Name {
			foundFileLevel = true
		}
	}
	if s.EnableConsole && !foundConsoleLevel {
		return fmt.Errorf("invalid ConsoleLevel value %q", s.ConsoleLevel)
	}
	if s.EnableFile && !foundFileLevel {
		return fmt.Errorf("invalid FileLevel value %q", s.FileLevel)
	}
	if s.EnableFile && s.FileLocation == "" {
		return fmt.Errorf("invalid FileLocation value: should not be empty")
	}
	return nil
}

// New returns a newly created and initialized logger with the given settings.
func New(settings Settings) (*mlog.Logger, error) {
	if err := settings.IsValid(); err != nil {
		return nil, err
	}

	logger, err := mlog.NewLogger()
	if err != nil {
		return nil, err
	}

	cfg := mlog.LoggerConfiguration{}
	if settings.EnableConsole {
		var format string
		var formatOpts string
		if settings.ConsoleJSON {
			format = "json"
			formatOpts = `{"enable_caller": true}`
		} else {
			format = "plain"
			formatOpts = fmt.Sprintf(`{"delim": " ", "min_level_len": 5, "min_msg_len": 45, "enable_color": %t, "enable_caller": true}`, settings.EnableColor)
		}

		cfg["_defConsole"] = mlog.TargetCfg{
			Type:          "console",
			Levels:        getLevels(settings.ConsoleLevel),
			Options:       json.RawMessage(`{"out": "stdout"}`),
			Format:        format,
			FormatOptions: json.RawMessage(formatOpts),
			MaxQueueSize:  1000,
		}
	}

	if settings.EnableFile {
		var format string
		var formatOpts string
		if settings.FileJSON {
			format = "json"
			formatOpts = `{"enable_caller": true}`
		} else {
			format = "plain"
			formatOpts = fmt.Sprintf(`{"delim": " ", "min_level_len": 5, "min_msg_len": 45, "enable_color": false, "enable_caller": true}`)
		}

		opts := fmt.Sprintf(`{"filename": "%s", "max_size": 100, "max_age": 0, "max_backups": 0, "compress": true}`, settings.FileLocation)
		cfg["_defFile"] = mlog.TargetCfg{
			Type:          "file",
			Levels:        getLevels(settings.FileLevel),
			Options:       json.RawMessage(opts),
			Format:        format,
			FormatOptions: json.RawMessage(formatOpts),
			MaxQueueSize:  1000,
		}
	}
	if err := logger.ConfigureTargets(cfg, nil); err != nil {
		return nil, err
	}

	logger.RedirectStdLog(mlog.LvlStdLog)

	return logger, nil
}
