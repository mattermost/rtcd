// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package auth

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"golang.org/x/time/rate"

	"github.com/mattermost/rtcd/service/store"
)

const (
	MinKeyLen                              = 32
	authTimeout                            = 10 * time.Second
	authRequestsPerSecondPerCPU rate.Limit = 12 // MM-53483
)

type Service struct {
	sessionCache *SessionCache
	store        store.Store
	limiter      *rate.Limiter
}

func NewService(store store.Store, sessionCache *SessionCache) (*Service, error) {
	if store == nil {
		return nil, errors.New("invalid store")
	}
	if sessionCache == nil {
		return nil, errors.New("invalid session cache")
	}
	return &Service{
		sessionCache: sessionCache,
		store:        store,
		limiter:      rate.NewLimiter(authRequestsPerSecondPerCPU*rate.Limit(runtime.NumCPU()), 1),
	}, nil
}

func (s *Service) Authenticate(id, authToken string) error {
	hash, err := s.store.Get(id)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), authTimeout)
	defer cancel()
	if err := s.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	if err := compareKeyHash(hash, authToken); err != nil {
		return errors.New("authentication failed")
	}
	return nil
}

func (s *Service) Register(id, key string) error {
	if len(key) < MinKeyLen {
		return errors.New("registration failed: key not long enough")
	}

	if _, err := s.store.Get(id); err == nil {
		return errors.New("registration failed: already registered")
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("registration failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), authTimeout)
	defer cancel()
	if err := s.limiter.Wait(ctx); err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	hash, err := hashKey(key)
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	if err := s.store.Put(id, hash); errors.Is(err, store.ErrConflict) {
		return errors.New("registration failed: already registered")
	} else if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	return nil
}

func (s *Service) Unregister(id string) error {
	if _, err := s.store.Get(id); err != nil {
		return fmt.Errorf("unregister failed: %w", err)
	}

	err := s.store.Delete(id)
	if err != nil {
		return fmt.Errorf("unregister failed: %w", err)
	}

	// Invalidate token when unregistering
	s.sessionCache.Delete(id)

	return nil
}

func (s *Service) Login(id, key string) (string, error) {
	if err := s.Authenticate(id, key); err != nil {
		return "", fmt.Errorf("login failed: %w", err)
	}
	bearerToken, err := newRandomToken()
	if err != nil {
		return "", fmt.Errorf("login failed: %w", err)
	}
	if err = s.sessionCache.Put(id, bearerToken); err != nil {
		return "", fmt.Errorf("login failed: %w", err)
	}
	return bearerToken, nil
}
