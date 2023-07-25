// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
	"github.com/pion/logging"
)

const pionPkgPrefix = "github.com/pion/"

func getLogOrigin() string {
	pc, file, line, ok := runtime.Caller(2)
	if !ok {
		return ""
	}

	f := runtime.FuncForPC(pc)
	if f == nil {
		return ""
	}

	if idx := strings.Index(file, pionPkgPrefix); idx > 0 {
		file = file[idx+len(pionPkgPrefix):]
	}

	return fmt.Sprintf("%s %s:%d", strings.TrimPrefix(f.Name(), pionPkgPrefix), file, line)
}

type pionLogger struct {
	log mlog.LoggerIFace
}

func newPionLeveledLogger(log mlog.LoggerIFace) logging.LeveledLogger {
	return &pionLogger{
		log: log,
	}
}

func (s *Server) NewLogger(_ string) logging.LeveledLogger {
	return newPionLeveledLogger(s.log)
}

func (log *pionLogger) Trace(msg string) {
	log.log.Trace(msg, mlog.String("origin", getLogOrigin()))
}

func (log *pionLogger) Tracef(format string, args ...interface{}) {
	log.log.Trace(fmt.Sprintf(format, args...), mlog.String("origin", getLogOrigin()))
}

func (log *pionLogger) Debug(msg string) {
	log.log.Trace(msg, mlog.String("origin", getLogOrigin()))
}

func (log *pionLogger) Debugf(format string, args ...interface{}) {
	log.log.Trace(fmt.Sprintf(format, args...), mlog.String("origin", getLogOrigin()))
}

func (log *pionLogger) Info(msg string) {
	log.log.Info(msg, mlog.String("origin", getLogOrigin()))
}

func (log *pionLogger) Infof(format string, args ...interface{}) {
	log.log.Info(fmt.Sprintf(format, args...), mlog.String("origin", getLogOrigin()))
}

func (log *pionLogger) Warn(msg string) {
	log.log.Warn(msg, mlog.String("origin", getLogOrigin()))
}

func (log *pionLogger) Warnf(format string, args ...interface{}) {
	log.log.Warn(fmt.Sprintf(format, args...), mlog.String("origin", getLogOrigin()))
}

func (log *pionLogger) Error(msg string) {
	log.log.Error(msg, mlog.String("origin", getLogOrigin()))
}

func (log *pionLogger) Errorf(format string, args ...interface{}) {
	log.log.Error(fmt.Sprintf(format, args...), mlog.String("origin", getLogOrigin()))
}
