// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

func (s *Service) authHandler(w http.ResponseWriter, r *http.Request) (clientID string, code int, err error) {
	defer func() {
		data := newHTTPData()
		data.code = code
		if err != nil {
			data.err = err.Error()
		}
		data.reqData["clientID"] = clientID

		s.httpAudit("authHandler", data, nil, r)
	}()

	clientID, authKey, ok := r.BasicAuth()
	if !ok {
		return "", http.StatusUnauthorized, fmt.Errorf("authentication failed: invalid auth header")
	}

	if s.cfg.API.Security.EnableAdmin && authKey == s.cfg.API.Security.AdminSecretKey {
		return "", http.StatusOK, nil
	}

	if clientID == "" {
		return "", http.StatusUnauthorized, fmt.Errorf("authentication failed: unauthorized")
	}

	if err := s.auth.Authenticate(clientID, authKey); err != nil {
		s.log.Error("authentication failed", mlog.Err(err))
		return "", http.StatusUnauthorized, fmt.Errorf("authentication failed")
	}

	return clientID, http.StatusOK, nil
}

func (s *Service) registerClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}

	data := newHTTPData()
	defer s.httpAudit("registerClient", data, w, r)

	if !s.cfg.API.Security.AllowSelfRegistration {
		_, code, err := s.authHandler(w, r)
		if err != nil {
			data.err = err.Error()
			data.code = code
			return
		}
	}

	if err := json.NewDecoder(r.Body).Decode(&data.reqData); err != nil {
		data.err = err.Error()
		data.code = http.StatusBadRequest
		return
	}

	clientID := data.reqData["clientID"]
	authKey, err := s.auth.Register(clientID)
	if err != nil {
		data.err = err.Error()
		data.code = http.StatusBadRequest
	} else {
		s.log.Debug("registered new client", mlog.String("clientID", clientID))
		data.code = http.StatusCreated
		data.resData["clientID"] = clientID
		data.resData["authKey"] = authKey
	}
}

func (s *Service) unregisterClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}

	data := newHTTPData()
	defer s.httpAudit("unregisterClient", data, w, r)

	_, code, err := s.authHandler(w, r)
	if err != nil {
		data.err = err.Error()
		data.code = code
		return
	}

	if err := json.NewDecoder(r.Body).Decode(&data.reqData); err != nil {
		data.err = err.Error()
		data.code = http.StatusBadRequest
		return
	}

	clientID := data.reqData["clientID"]
	if clientID == "" {
		data.err = "client id should not be empty"
		data.code = http.StatusBadRequest
		return
	}

	err = s.auth.Unregister(clientID)
	if err != nil {
		data.err = err.Error()
		data.code = http.StatusBadRequest
		return
	}

	s.log.Debug("unregistered client", mlog.String("clientID", clientID))

	data.code = http.StatusOK
}
