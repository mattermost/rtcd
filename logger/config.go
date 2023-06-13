// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package logger

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

// Config holds information used to initialize a new logger.
type Config struct {
	EnableConsole bool   `toml:"enable_console"`
	ConsoleJSON   bool   `toml:"console_json"`
	ConsoleLevel  string `toml:"console_level"`
	EnableFile    bool   `toml:"enable_file"`
	FileJSON      bool   `toml:"file_json"`
	FileLevel     string `toml:"file_level"`
	FileLocation  string `toml:"file_location"`
	EnableColor   bool   `toml:"enable_color"`
}

func (c Config) IsValid() error {
	if !c.EnableConsole && !c.EnableFile {
		return fmt.Errorf("should enable at least one logging target")
	}
	var foundConsoleLevel bool
	var foundFileLevel bool
	for _, l := range mlog.StdAll {
		if strings.ToLower(c.ConsoleLevel) == l.Name {
			foundConsoleLevel = true
		}
		if strings.ToLower(c.FileLevel) == l.Name {
			foundFileLevel = true
		}
	}
	if c.EnableConsole && !foundConsoleLevel {
		return fmt.Errorf("invalid ConsoleLevel value %q", c.ConsoleLevel)
	}
	if c.EnableFile && !foundFileLevel {
		return fmt.Errorf("invalid FileLevel value %q", c.FileLevel)
	}
	if c.EnableFile && c.FileLocation == "" {
		return fmt.Errorf("invalid FileLocation value: should not be empty")
	}
	return nil
}
