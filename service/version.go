// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"encoding/json"
	"net/http"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

var (
	version   = "0.1.0"
	buildHash string
)

func (s *Service) getVersion(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.NotFound(w, req)
		return
	}

	version := map[string]string{
		"version": version,
		"build":   buildHash,
	}

	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(version); err != nil {
		s.log.Error("failed to encode data", mlog.Err(err))
	}
}
