// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRandomString(t *testing.T) {
	str, err := newRandomString(0)
	require.NoError(t, err)
	require.Empty(t, str)

	for i := 1; i <= 1024; i++ {
		str, err = newRandomString(i)
		require.NoError(t, err)
		require.NotEmpty(t, str)
		require.Len(t, str, i)
	}
}

func TestHashKey(t *testing.T) {
	key, err := newRandomString(MinKeyLen)
	require.NoError(t, err)
	require.Len(t, key, MinKeyLen)

	hash, err := hashKey("")
	require.Error(t, err)
	require.EqualError(t, err, "invalid empty key")
	require.Empty(t, hash)

	hash, err = hashKey(key)
	require.NoError(t, err)
	require.NotEmpty(t, hash)
}

func TestCompareKeyHash(t *testing.T) {
	key, err := newRandomString(MinKeyLen)
	require.NoError(t, err)
	require.Len(t, key, MinKeyLen)

	hash, err := hashKey(key)
	require.NoError(t, err)
	require.NotEmpty(t, hash)

	err = compareKeyHash("", key)
	require.Error(t, err)
	require.EqualError(t, err, "invalid empty hash")

	err = compareKeyHash(hash, "")
	require.Error(t, err)
	require.EqualError(t, err, "invalid empty key")

	err = compareKeyHash(key, hash)
	require.Error(t, err)

	err = compareKeyHash(hash, key+" ")
	require.Error(t, err)
	require.EqualError(t, err, "crypto/bcrypt: hashedPassword is not the hash of the given password")

	err = compareKeyHash(hash, key)
	require.NoError(t, err)
}
