// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

type SystemInfo struct {
	CPULoad float64 `json:"cpu_load"`
}

func (s *Service) getSystemInfo(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.NotFound(w, req)
		return
	}

	var info SystemInfo
	st1, err1 := s.proc.Stat()
	t0 := time.Now()
	// We take a one second sample.
	time.Sleep(time.Second)
	st2, err2 := s.proc.Stat()
	t1 := time.Now()
	if err1 == nil && err2 == nil {
		idleDiff := st2.CPUTotal.Idle - st1.CPUTotal.Idle
		info.CPULoad = 1 / (idleDiff / t1.Sub(t0).Seconds())
	} else {
		if err1 != nil {
			s.log.Error("failed to get cpu stat", mlog.Err(err1))
		}
		if err2 != nil {
			s.log.Error("failed to get cpu stat", mlog.Err(err2))
		}
	}

	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&info); err != nil {
		s.log.Error("failed to encode data", mlog.Err(err))
	}
}
