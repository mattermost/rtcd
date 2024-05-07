// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"encoding/json"
	"net/http"

	"github.com/mattermost/mattermost/server/public/shared/mlog"

	"github.com/prometheus/procfs"
)

type SystemInfo struct {
	Load procfs.LoadAvg `json:"load"`
}

func (s *Service) getSystemInfo(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.NotFound(w, req)
		return
	}

	var info SystemInfo
	avg, err := s.proc.LoadAvg()
	if err == nil {
		info.Load = *avg
	} else {
		s.log.Error("failed to get load average", mlog.Err(err))
	}

	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&info); err != nil {
		s.log.Error("failed to encode data", mlog.Err(err))
	}
}
