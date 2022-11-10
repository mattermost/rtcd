// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package auth

import (
	"os"
	"testing"

	"github.com/mattermost/rtcd/service/store"

	"github.com/stretchr/testify/require"
)

func newTestDBStore(t *testing.T) (store.Store, func()) {
	t.Helper()
	dbDir, err := os.MkdirTemp("", "db")
	require.NoError(t, err)
	dbStore, err := store.New(dbDir)
	require.NoError(t, err)
	return dbStore, func() {
		err := dbStore.Close()
		require.NoError(t, err)
		err = os.RemoveAll(dbDir)
		require.NoError(t, err)
	}
}

func newTestSessionCache(t *testing.T) *SessionCache {
	t.Helper()
	sessionCache, err := NewSessionCache(SessionCacheConfig{ExpirationMinutes: 1440})
	require.NoError(t, err)
	require.NotNil(t, sessionCache)
	return sessionCache
}

func TestNewService(t *testing.T) {
	dbStore, teardown := newTestDBStore(t)
	defer teardown()
	sessionCache := newTestSessionCache(t)

	t.Run("missing store", func(t *testing.T) {
		s, err := NewService(nil, sessionCache)
		require.Error(t, err)
		require.Nil(t, s)
	})

	t.Run("missing session cache", func(t *testing.T) {
		s, err := NewService(dbStore, nil)
		require.Error(t, err)
		require.Nil(t, s)
	})

	t.Run("valid", func(t *testing.T) {
		s, err := NewService(dbStore, sessionCache)
		require.NoError(t, err)
		require.NotNil(t, s)
	})
}

func TestRegister(t *testing.T) {
	dbStore, teardown := newTestDBStore(t)
	defer teardown()
	sessionCache := newTestSessionCache(t)

	s, err := NewService(dbStore, sessionCache)
	require.NoError(t, err)
	require.NotNil(t, s)

	err = s.Register("instanceA", "short key")
	require.Error(t, err)
	require.EqualError(t, err, "registration failed: key not long enough")

	authKey, err := newRandomString(MinKeyLen)
	require.NoError(t, err)
	err = s.Register("instanceA", authKey)
	require.NoError(t, err)

	err = s.Register("instanceA", authKey)
	require.Error(t, err)
	require.EqualError(t, err, "registration failed: already registered")

	err = s.Unregister("instanceA")
	require.NoError(t, err)

	err = s.Unregister("instanceA")
	require.Error(t, err)
	require.EqualError(t, err, "unregister failed: error: not found")

	err = s.Register("instanceA", authKey)
	require.NoError(t, err)
}

func TestAuthenticate(t *testing.T) {
	dbStore, teardown := newTestDBStore(t)
	defer teardown()
	sessionCache := newTestSessionCache(t)

	s, err := NewService(dbStore, sessionCache)
	require.NoError(t, err)
	require.NotNil(t, s)

	err = s.Authenticate("instanceA", "authkey")
	require.Error(t, err)
	require.EqualError(t, err, "authentication failed: error: not found")

	authKey, err := newRandomString(MinKeyLen)
	require.NoError(t, err)
	err = s.Register("instanceA", authKey)
	require.NoError(t, err)

	err = s.Authenticate("instanceA", authKey)
	require.NoError(t, err)

	err = s.Authenticate("instanceA", authKey+" ")
	require.Error(t, err)
	require.EqualError(t, err, "authentication failed")

	err = s.Unregister("instanceA")
	require.NoError(t, err)

	err = s.Authenticate("instanceA", "authkey")
	require.Error(t, err)
	require.EqualError(t, err, "authentication failed: error: not found")
}
