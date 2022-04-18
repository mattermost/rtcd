// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package auth

import (
	"errors"
	"fmt"

	"github.com/mattermost/rtcd/service/store"
)

const DefaultKeyLen = 32

type Service struct {
	store store.Store
}

func NewService(store store.Store) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("invalid store")
	}
	return &Service{
		store: store,
	}, nil
}

func (s *Service) Authenticate(id, authToken string) error {
	hash, err := s.store.Get(id)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	if err := compareKeyHash(hash, authToken); err != nil {
		return fmt.Errorf("authentication failed")
	}
	return nil
}

func (s *Service) Register(id string) (string, error) {
	if _, err := s.store.Get(id); err == nil {
		return "", fmt.Errorf("registration failed: already registered")
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return "", fmt.Errorf("registration failed: %w", err)
	}

	authToken, err := newRandomString(DefaultKeyLen)
	if err != nil {
		return "", fmt.Errorf("registration failed: %w", err)
	}

	hash, err := hashKey(authToken)
	if err != nil {
		return "", fmt.Errorf("registration failed: %w", err)
	}

	if err := s.store.Put(id, hash); errors.Is(err, store.ErrConflict) {
		return "", fmt.Errorf("registration failed: already registered")
	} else if err != nil {
		return "", fmt.Errorf("registration failed: %w", err)
	}

	return authToken, nil
}

func (s *Service) Unregister(id string) error {
	if _, err := s.store.Get(id); err != nil {
		return fmt.Errorf("unregister failed: %w", err)
	}

	err := s.store.Delete(id)
	if err != nil {
		return fmt.Errorf("unregister failed: %w", err)
	}

	return nil
}
