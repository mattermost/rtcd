// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package auth

import (
	"errors"
	"sync"
	"time"
)

type CachedSession struct {
	ClientID       string
	ExpirationDate time.Time
}

type SessionCacheConfig struct {
	ExpirationMinutes int `toml:"expiration_minutes"`
}

func (c SessionCacheConfig) IsValid() error {
	if c.ExpirationMinutes <= 0 {
		return errors.New("invalid ExpirationMinutes value: should be a positive number")
	}
	return nil
}

type SessionCache struct {
	cfg        SessionCacheConfig
	sessionMap map[string]CachedSession

	mut sync.RWMutex
}

func NewSessionCache(cfg SessionCacheConfig) (*SessionCache, error) {
	if err := cfg.IsValid(); err != nil {
		return nil, err
	}
	return &SessionCache{cfg: cfg, sessionMap: make(map[string]CachedSession)}, nil
}

func (t *SessionCache) Get(token string) (CachedSession, error) {
	t.mut.RLock()
	session, ok := t.sessionMap[token]
	t.mut.RUnlock()
	if !ok {
		return CachedSession{}, errors.New("token is invalid")
	}
	if time.Now().After(session.ExpirationDate) {
		t.mut.Lock()
		delete(t.sessionMap, token)
		t.mut.Unlock()
		return CachedSession{}, errors.New("session is expired")
	}
	return session, nil
}

func (t *SessionCache) Put(clientID, token string) error {
	if len(clientID) == 0 {
		return errors.New("can not cache: invalid client id")
	}
	if len(token) == 0 {
		return errors.New("can not cache: invalid token")
	}

	t.mut.Lock()
	defer t.mut.Unlock()

	_, ok := t.sessionMap[token]
	if ok {
		return errors.New("can not cache: token in use")
	}

	// Make sure there is only one bearer token per client.
	t.delete(clientID)
	t.sessionMap[token] = CachedSession{
		ClientID:       clientID,
		ExpirationDate: time.Now().Add(time.Duration(t.cfg.ExpirationMinutes) * time.Minute),
	}
	return nil
}

func (t *SessionCache) Delete(clientID string) {
	t.mut.Lock()
	t.delete(clientID)
	t.mut.Unlock()
}

func (t *SessionCache) delete(clientID string) {
	for token, session := range t.sessionMap {
		if session.ClientID == clientID {
			delete(t.sessionMap, token)
			return
		}
	}
}
