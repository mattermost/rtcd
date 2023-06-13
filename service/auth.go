// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/mattermost/mattermost/server/public/shared/mlog"
)

const bearerPrefix = "Bearer "

func (s *Service) authHandler(w http.ResponseWriter, r *http.Request) (clientID string, code int, err error) {
	defer func() {
		data := &httpData{
			reqData: map[string]string{},
			resData: map[string]string{},
		}

		data.code = code
		if err != nil {
			data.err = err.Error()
		}
		data.reqData["clientID"] = clientID

		s.httpAudit("authHandler", data, nil, r)
	}()

	if strings.HasPrefix(r.Header.Get("Authorization"), bearerPrefix) {
		return s.bearerAuthHandler(w, r)
	}
	return s.basicAuthHandler(w, r)
}

func (s *Service) basicAuthHandler(_ http.ResponseWriter, r *http.Request) (string, int, error) {
	clientID, authKey, ok := r.BasicAuth()
	if !ok {
		return "", http.StatusUnauthorized, errors.New("authentication failed: invalid auth header")
	}

	if s.cfg.API.Security.EnableAdmin && authKey == s.cfg.API.Security.AdminSecretKey {
		return "", http.StatusOK, nil
	}

	if clientID == "" {
		return "", http.StatusUnauthorized, errors.New("authentication failed: unauthorized")
	}

	if err := s.auth.Authenticate(clientID, authKey); err != nil {
		s.log.Error("authentication failed", mlog.Err(err))
		return "", http.StatusUnauthorized, errors.New("authentication failed")
	}

	return clientID, http.StatusOK, nil
}

func (s *Service) bearerAuthHandler(_ http.ResponseWriter, r *http.Request) (string, int, error) {
	bearerToken, ok := parseBearerAuth(r.Header.Get("Authorization"))
	if !ok {
		return "", http.StatusUnauthorized, errors.New("authentication failed: invalid auth header")
	}

	session, err := s.sessionCache.Get(bearerToken)
	if err != nil {
		return "", http.StatusUnauthorized, fmt.Errorf("authentication failed: %w", err)
	}

	if session.ClientID == "" {
		return "", http.StatusUnauthorized, errors.New("authentication failed: unauthorized")
	}

	return session.ClientID, http.StatusOK, nil
}

func parseBearerAuth(auth string) (token string, ok bool) {
	if len(auth) < len(bearerPrefix) || !strings.EqualFold(auth[:len(bearerPrefix)], bearerPrefix) {
		return
	}
	return auth[len(bearerPrefix):], true
}

func (s *Service) registerClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}

	data := &httpData{
		reqData: map[string]string{},
		resData: map[string]string{},
	}
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
	authKey := data.reqData["authKey"]
	err := s.auth.Register(clientID, authKey)
	if err != nil {
		data.err = err.Error()
		data.code = http.StatusBadRequest
		return
	}

	s.log.Debug("registered new client", mlog.String("clientID", clientID))
	data.code = http.StatusCreated
	data.resData["clientID"] = clientID
}

func (s *Service) unregisterClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}

	data := &httpData{
		reqData: map[string]string{},
		resData: map[string]string{},
	}
	defer s.httpAudit("unregisterClient", data, w, r)

	// If an admin client is not enabled, and self registration is not allowed,
	// clients cannot unregister themselves.
	if !s.cfg.API.Security.EnableAdmin && !s.cfg.API.Security.AllowSelfRegistration {
		s.log.Warn("/unregister was called, but enable_admin and allow_self_registration are both false")
		data.err = "unregister not enabled"
		data.code = http.StatusForbidden
		return
	}

	// Check if admin authKey or clientID + authKey have been provided
	authedClientID, code, err := s.authHandler(w, r)
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

	// an authedClientID == "" means admin. So if there is an authedClientID,
	// then the requested clientID needs to be the same.
	if authedClientID != "" && authedClientID != clientID {
		data.err = "client id not valid"
		data.code = http.StatusForbidden
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

func (s *Service) loginClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}

	data := &httpData{
		reqData: map[string]string{},
		resData: map[string]string{},
	}
	defer s.httpAudit("loginClient", data, w, r)

	if err := json.NewDecoder(r.Body).Decode(&data.reqData); err != nil {
		data.err = err.Error()
		data.code = http.StatusBadRequest
		return
	}

	clientID := data.reqData["clientID"]
	authKey := data.reqData["authKey"]
	bearerToken, err := s.auth.Login(clientID, authKey)
	if err != nil {
		data.err = err.Error()
		data.code = http.StatusBadRequest
		return
	}

	s.log.Debug("logged in client", mlog.String("clientID", clientID))
	data.code = http.StatusOK
	data.resData["bearerToken"] = bearerToken
}
