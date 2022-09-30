// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// newRandomToken returns a secure token with a fixed length of 32 characters.
func newRandomToken() (string, error) {
	return newRandomString(MinKeyLen)
}

// newRandomString returns a secure random string of the given length.
// The resulting entropy will be (6 * length) bits.
func newRandomString(length int) (string, error) {
	data := make([]byte, 1+(length*4)/3)
	if n, err := rand.Read(data); err != nil {
		return "", err
	} else if n != len(data) {
		return "", fmt.Errorf("failed to read enough data")
	}
	return base64.RawURLEncoding.EncodeToString(data)[:length], nil
}

// hashKey generates a hash using the bcrypt.GenerateFromPassword
func hashKey(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("invalid empty key")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(key), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// compareKeyHash compares the given hash and key using bcrypt.CompareHashAndPassword
func compareKeyHash(hash string, key string) error {
	if hash == "" {
		return fmt.Errorf("invalid empty hash")
	}
	if key == "" {
		return fmt.Errorf("invalid empty key")
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(key))
}
