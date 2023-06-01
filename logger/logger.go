// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package logger

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mattermost/mattermost-server/server/public/shared/mlog"
)

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

// New returns a newly created and initialized logger with the given cfg.
func New(config Config) (*mlog.Logger, error) {
	if err := config.IsValid(); err != nil {
		return nil, err
	}

	logger, err := mlog.NewLogger()
	if err != nil {
		return nil, err
	}

	cfg := mlog.LoggerConfiguration{}
	if config.EnableConsole {
		var format string
		var formatOpts string
		if config.ConsoleJSON {
			format = "json"
			formatOpts = `{"enable_caller": true}`
		} else {
			format = "plain"
			formatOpts = fmt.Sprintf(`{"delim": " ", "min_level_len": 5, "min_msg_len": 45, "enable_color": %t, "enable_caller": true}`, config.EnableColor)
		}

		cfg["_defConsole"] = mlog.TargetCfg{
			Type:          "console",
			Levels:        getLevels(config.ConsoleLevel),
			Options:       json.RawMessage(`{"out": "stdout"}`),
			Format:        format,
			FormatOptions: json.RawMessage(formatOpts),
			MaxQueueSize:  1000,
		}
	}

	if config.EnableFile {
		var format string
		var formatOpts string
		if config.FileJSON {
			format = "json"
			formatOpts = `{"enable_caller": true}`
		} else {
			format = "plain"
			formatOpts = `{"delim": " ", "min_level_len": 5, "min_msg_len": 45, "enable_color": false, "enable_caller": true}`
		}

		opts := fmt.Sprintf(`{"filename": "%s", "max_size": 100, "max_age": 0, "max_backups": 0, "compress": true}`, config.FileLocation)
		cfg["_defFile"] = mlog.TargetCfg{
			Type:          "file",
			Levels:        getLevels(config.FileLevel),
			Options:       json.RawMessage(opts),
			Format:        format,
			FormatOptions: json.RawMessage(formatOpts),
			MaxQueueSize:  1000,
		}
	}
	if err := logger.ConfigureTargets(cfg, nil); err != nil {
		return nil, err
	}

	return logger, nil
}
