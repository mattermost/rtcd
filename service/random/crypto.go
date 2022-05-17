// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package random

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// NewSecureString returns a secure random string of the given length.
// The resulting entropy will be (6 * length) bits.
func NewSecureString(length int) (string, error) {
	data := make([]byte, 1+(length*4)/3)
	if n, err := rand.Read(data); err != nil {
		return "", err
	} else if n != len(data) {
		return "", fmt.Errorf("failed to read enough data")
	}
	return base64.RawURLEncoding.EncodeToString(data)[:length], nil
}
