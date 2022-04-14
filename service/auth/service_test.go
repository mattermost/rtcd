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

func TestNewService(t *testing.T) {
	dbStore, teardown := newTestDBStore(t)
	defer teardown()

	t.Run("missing store", func(t *testing.T) {
		s, err := NewService(nil)
		require.Error(t, err)
		require.Nil(t, s)
	})

	t.Run("valid", func(t *testing.T) {
		s, err := NewService(dbStore)
		require.NoError(t, err)
		require.NotNil(t, s)
	})
}

func TestRegister(t *testing.T) {
	dbStore, teardown := newTestDBStore(t)
	defer teardown()

	s, err := NewService(dbStore)
	require.NoError(t, err)
	require.NotNil(t, s)

	authToken, err := s.Register("instanceA")
	require.NoError(t, err)
	require.Len(t, authToken, DefaultKeyLen)

	authToken, err = s.Register("instanceA")
	require.Error(t, err)
	require.Empty(t, authToken)
	require.EqualError(t, err, "registration failed: already registered")

	err = s.Unregister("instanceA")
	require.NoError(t, err)

	err = s.Unregister("instanceA")
	require.Error(t, err)
	require.EqualError(t, err, "unregister failed: error: not found")

	authToken, err = s.Register("instanceA")
	require.NoError(t, err)
	require.Len(t, authToken, DefaultKeyLen)
}

func TestAuthenticate(t *testing.T) {
	dbStore, teardown := newTestDBStore(t)
	defer teardown()

	s, err := NewService(dbStore)
	require.NoError(t, err)
	require.NotNil(t, s)

	err = s.Authenticate("instanceA", "authkey")
	require.Error(t, err)
	require.EqualError(t, err, "authentication failed: error: not found")

	authToken, err := s.Register("instanceA")
	require.NoError(t, err)
	require.Len(t, authToken, DefaultKeyLen)

	err = s.Authenticate("instanceA", authToken)
	require.NoError(t, err)

	err = s.Authenticate("instanceA", authToken+" ")
	require.Error(t, err)
	require.EqualError(t, err, "authentication failed")

	err = s.Unregister("instanceA")
	require.NoError(t, err)

	err = s.Authenticate("instanceA", "authkey")
	require.Error(t, err)
	require.EqualError(t, err, "authentication failed: error: not found")
}
