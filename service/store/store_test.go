// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package store

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	dbDir, err := os.MkdirTemp("", "db")
	require.NoError(t, err)
	defer os.RemoveAll(dbDir)

	t.Run("invalid db path", func(t *testing.T) {
		store, err := New("")
		require.Error(t, err)
		require.Nil(t, store)
	})

	t.Run("valid", func(t *testing.T) {
		store, err := New(dbDir)
		require.NoError(t, err)
		require.NotNil(t, store)
		err = store.Close()
		require.NoError(t, err)
	})
}

func TestGetSet(t *testing.T) {
	dbDir, err := os.MkdirTemp("", "db")
	require.NoError(t, err)
	defer os.RemoveAll(dbDir)

	store, err := New(dbDir)
	require.NoError(t, err)
	require.NotNil(t, store)
	defer store.Close()

	t.Run("getting missing key", func(t *testing.T) {
		val, err := store.Get("missing")
		require.Error(t, err)
		require.Equal(t, ErrNotFound, err)
		require.Empty(t, val)
	})

	t.Run("setting empty key", func(t *testing.T) {
		err := store.Set("", "")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrEmptyKey)
	})

	t.Run("setting", func(t *testing.T) {
		err := store.Set("key", "value")
		require.NoError(t, err)
	})

	t.Run("getting", func(t *testing.T) {
		val, err := store.Get("key")
		require.NoError(t, err)
		require.Equal(t, "value", val)
	})

	t.Run("getting after reopening", func(t *testing.T) {
		err = store.Close()
		require.NoError(t, err)
		store, err := New(dbDir)
		require.NoError(t, err)
		require.NotNil(t, store)

		val, err := store.Get("key")
		require.NoError(t, err)
		require.Equal(t, "value", val)
	})
}

func TestDelete(t *testing.T) {
	dbDir, err := os.MkdirTemp("", "db")
	require.NoError(t, err)
	defer os.RemoveAll(dbDir)

	store, err := New(dbDir)
	require.NoError(t, err)
	require.NotNil(t, store)
	defer store.Close()

	t.Run("delete empty", func(t *testing.T) {
		err := store.Delete("")
		require.Error(t, err)
		require.Equal(t, ErrEmptyKey, err)
	})

	t.Run("delete missing", func(t *testing.T) {
		err := store.Delete("key")
		require.NoError(t, err)
	})

	t.Run("delete existing", func(t *testing.T) {
		err := store.Set("key", "value")
		require.NoError(t, err)

		val, err := store.Get("key")
		require.NoError(t, err)
		require.Equal(t, "value", val)

		err = store.Delete("key")
		require.NoError(t, err)

		val, err = store.Get("key")
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNotFound)
		require.Empty(t, val)
	})
}
