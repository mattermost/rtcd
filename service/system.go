// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/prometheus/procfs"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

type SystemInfo struct {
	CPULoad float64 `json:"cpu_load"`
}

func (s *Service) collectSystemInfo() {
	// One second sampling interval.
	ticker := time.NewTicker(time.Second)

	var prevStat procfs.Stat
	var prevTime time.Time

	for {
		select {
		case currTime := <-ticker.C:
			currStat, err := s.proc.Stat()

			if err != nil {
				s.log.Error("failed to get cpu stat", mlog.Err(err))
				continue
			}

			idleDiff := currStat.CPUTotal.Idle - prevStat.CPUTotal.Idle

			if !prevTime.IsZero() {
				s.mut.Lock()
				s.systemInfo.CPULoad = 1 / (idleDiff / currTime.Sub(prevTime).Seconds())
				s.mut.Unlock()
			}

			prevStat = currStat
			prevTime = currTime
		case <-s.stopCh:
			return
		}
	}
}

func (s *Service) getSystemInfo(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.NotFound(w, req)
		return
	}

	s.mut.RLock()
	defer s.mut.RUnlock()

	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&s.systemInfo); err != nil {
		s.log.Error("failed to encode data", mlog.Err(err))
	}
}
