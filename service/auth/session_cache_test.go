// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewSessionCache(t *testing.T) {
	t.Run("empty config", func(t *testing.T) {
		tc, err := NewSessionCache(SessionCacheConfig{})
		require.Error(t, err)
		require.Equal(t, "invalid ExpirationMinutes value: should be a positive number", err.Error())
		require.Nil(t, tc)
	})

	t.Run("invalid session expiration minutes", func(t *testing.T) {
		tc, err := NewSessionCache(SessionCacheConfig{})
		require.Error(t, err)
		require.Equal(t, "invalid ExpirationMinutes value: should be a positive number", err.Error())
		require.Nil(t, tc)
	})

	t.Run("success", func(t *testing.T) {
		tc, err := NewSessionCache(SessionCacheConfig{ExpirationMinutes: 1440})
		require.NoError(t, err)
		require.NotNil(t, tc)
		require.NotEmpty(t, tc)
		require.Equal(t, tc.sessionMap, map[string]CachedSession{})
	})
}

func TestGet(t *testing.T) {
	tc, err := NewSessionCache(SessionCacheConfig{ExpirationMinutes: 1440})
	require.NoError(t, err)

	t.Run("token is invalid", func(t *testing.T) {
		session, err := tc.Get("foo")
		require.Error(t, err)
		require.Equal(t, "token is invalid", err.Error())
		require.Empty(t, session)
	})

	t.Run("session is expired", func(t *testing.T) {
		tc.sessionMap = map[string]CachedSession{"foo": {ClientID: "bar", ExpirationDate: time.Now().Add(-10 * time.Minute)}}
		session, err := tc.Get("foo")
		require.Error(t, err)
		require.Equal(t, "session is expired", err.Error())
		require.Empty(t, session)
	})

	t.Run("valid session returned", func(t *testing.T) {
		expirationDate := time.Now().Add(10 * time.Minute)
		tc.sessionMap = map[string]CachedSession{"foo": {ClientID: "bar", ExpirationDate: expirationDate}}
		session, err := tc.Get("foo")
		require.NoError(t, err)
		require.NotNil(t, session)
		require.NotEmpty(t, session)
		require.Equal(t, CachedSession{ClientID: "bar", ExpirationDate: expirationDate}, session)
	})
}

func TestPut(t *testing.T) {
	tc, err := NewSessionCache(SessionCacheConfig{ExpirationMinutes: 1440})
	require.NoError(t, err)

	t.Run("invalid client id", func(t *testing.T) {
		err := tc.Put("", "bar")
		require.Error(t, err)
		require.Equal(t, "can not cache: invalid client id", err.Error())
	})

	t.Run("invalid token", func(t *testing.T) {
		err := tc.Put("foo", "")
		require.Error(t, err)
		require.Equal(t, "can not cache: invalid token", err.Error())
	})

	t.Run("session is cached", func(t *testing.T) {
		err := tc.Put("foo", "bar")
		require.NoError(t, err)
		require.Len(t, tc.sessionMap, 1)
	})

	t.Run("token already used", func(t *testing.T) {
		require.Len(t, tc.sessionMap, 1)
		err = tc.Put("test", "bar")
		require.Error(t, err)
		require.Equal(t, "can not cache: token in use", err.Error())
		require.Len(t, tc.sessionMap, 1)
	})

	t.Run("existing session is deleted", func(t *testing.T) {
		require.Len(t, tc.sessionMap, 1)
		err := tc.Put("foo", "foo")
		require.NoError(t, err)
		require.Len(t, tc.sessionMap, 1)
	})
}

func TestDelete(t *testing.T) {
	tc, err := NewSessionCache(SessionCacheConfig{ExpirationMinutes: 1440})
	require.NoError(t, err)

	t.Run("session is deleted", func(t *testing.T) {
		err := tc.Put("foo", "bar")
		require.NoError(t, err)
		require.Len(t, tc.sessionMap, 1)
		tc.Delete("foo")
		require.Len(t, tc.sessionMap, 0)
	})
}
