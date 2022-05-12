// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"encoding/json"
	"net/http"
	"runtime"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

var (
	buildVersion string
	buildHash    string
	buildDate    string
)

type versionInfo struct {
	BuildDate    string `json:"buildDate"`
	BuildVersion string `json:"buildVersion"`
	BuildHash    string `json:"buildHash"`
	GoVersion    string `json:"goVersion"`
}

func getVersionInfo() versionInfo {
	return versionInfo{
		BuildDate:    buildDate,
		BuildVersion: buildVersion,
		BuildHash:    buildHash,
		GoVersion:    runtime.Version(),
	}
}

func (v versionInfo) logFields() []mlog.Field {
	return []mlog.Field{
		mlog.String("buildDate", v.BuildDate),
		mlog.String("buildVersion", v.BuildVersion),
		mlog.String("buildHash", v.BuildHash),
		mlog.String("goVersion", v.GoVersion),
	}
}

func (s *Service) getVersion(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.NotFound(w, req)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(getVersionInfo()); err != nil {
		s.log.Error("failed to encode data", mlog.Err(err))
	}
}
