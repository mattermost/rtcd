// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

func (s *Service) wsAuthHandler(w http.ResponseWriter, r *http.Request) (string, error) {
	clientID, authKey, ok := r.BasicAuth()
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return "", fmt.Errorf("authentication failed: invalid auth header")
	}

	if err := s.auth.Authenticate(clientID, authKey); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return "", fmt.Errorf("authentication failed: %w", err)
	}

	return clientID, nil
}

func (s *Service) registerClient(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.NotFound(w, req)
		return
	}

	response := map[string]string{}

	defer func() {
		w.Header().Add("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			s.log.Error("failed to encode data", mlog.Err(err))
		}
	}()

	request := map[string]string{}
	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response["error"] = err.Error()
		return
	}

	authKey, err := s.auth.Register(request["clientID"])
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response["error"] = err.Error()
	} else {
		w.WriteHeader(http.StatusCreated)
		response["authKey"] = authKey
	}
}
