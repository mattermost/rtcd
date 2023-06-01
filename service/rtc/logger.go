// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"fmt"

	"github.com/mattermost/mattermost-server/server/public/shared/mlog"
	"github.com/pion/logging"
)

type pionLogger struct {
	log mlog.LoggerIFace
}

func newPionLeveledLogger(log mlog.LoggerIFace) logging.LeveledLogger {
	return &pionLogger{
		log: log,
	}
}

func (log *pionLogger) Trace(msg string) {
	log.log.Trace(msg)
}

func (log *pionLogger) Tracef(format string, args ...interface{}) {
	log.log.Trace(fmt.Sprintf(format, args...))
}

func (log *pionLogger) Debug(msg string) {
	log.log.Trace(msg)
}

func (log *pionLogger) Debugf(format string, args ...interface{}) {
	log.log.Trace(fmt.Sprintf(format, args...))
}

func (log *pionLogger) Info(msg string) {
	log.log.Info(msg)
}

func (log *pionLogger) Infof(format string, args ...interface{}) {
	log.log.Info(fmt.Sprintf(format, args...))
}

func (log *pionLogger) Warn(msg string) {
	log.log.Warn(msg)
}

func (log *pionLogger) Warnf(format string, args ...interface{}) {
	log.log.Warn(fmt.Sprintf(format, args...))
}

func (log *pionLogger) Error(msg string) {
	log.log.Error(msg)
}

func (log *pionLogger) Errorf(format string, args ...interface{}) {
	log.log.Error(fmt.Sprintf(format, args...))
}
