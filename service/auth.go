// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

func (s *Service) authHandler(w http.ResponseWriter, r *http.Request) (string, error) {
	clientID, authKey, ok := r.BasicAuth()
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return "", fmt.Errorf("authentication failed: invalid auth header")
	}

	if s.cfg.API.Security.EnableAdmin && authKey == s.cfg.API.Security.AdminSecretKey {
		return "", nil
	}

	if clientID == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return "", fmt.Errorf("authentication failed: unauthorized")
	}

	if err := s.auth.Authenticate(clientID, authKey); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		s.log.Error("authentication failed", mlog.Err(err))
		return "", fmt.Errorf("authentication failed")
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

	if !s.cfg.API.Security.AllowSelfRegistration {
		_, err := s.authHandler(w, req)
		if err != nil {
			response["error"] = err.Error()
			return
		}
	}

	request := map[string]string{}
	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response["error"] = err.Error()
		return
	}

	clientID := request["clientID"]
	authKey, err := s.auth.Register(clientID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response["error"] = err.Error()
	} else {
		s.log.Debug("registered new client", mlog.String("clientID", clientID))
		w.WriteHeader(http.StatusCreated)
		response["clientID"] = clientID
		response["authKey"] = authKey
	}
}

func (s *Service) unregisterClient(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.NotFound(w, req)
		return
	}

	response := map[string]string{}

	defer func() {
		if len(response) == 0 {
			return
		}
		w.Header().Add("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			s.log.Error("failed to encode data", mlog.Err(err))
		}
	}()

	_, err := s.authHandler(w, req)
	if err != nil {
		response["error"] = err.Error()
		return
	}

	request := map[string]string{}
	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response["error"] = err.Error()
		return
	}

	clientID := request["clientID"]
	if clientID == "" {
		w.WriteHeader(http.StatusBadRequest)
		response["error"] = "client id should not be empty"
		return
	}

	err = s.auth.Unregister(clientID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		response["error"] = err.Error()
		return
	}

	s.log.Debug("unregistered client", mlog.String("clientID", clientID))

	w.WriteHeader(http.StatusOK)
}
