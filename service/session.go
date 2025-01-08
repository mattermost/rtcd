// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

func (s *Service) handleGetSession(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.NotFound(w, req)
		return
	}

	clientID, code, err := s.authHandler(w, req)
	if err != nil {
		s.log.Error("failed to authenticate", mlog.Err(err), mlog.Int("code", code))
	}

	if clientID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	callID := req.PathValue("callID")
	if callID == "" {
		http.Error(w, "callID is required", http.StatusBadRequest)
		return
	}

	sessionID := req.PathValue("sessionID")
	if sessionID == "" {
		http.Error(w, "sessionID is required", http.StatusBadRequest)
		return
	}

	cfg, err := s.rtcServer.GetSessionConfig(clientID, callID, sessionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get session config: %s", err.Error()), http.StatusNotFound)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(cfg); err != nil {
		s.log.Error("failed to encode data", mlog.Err(err))
	}
}
