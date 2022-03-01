// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsValid(t *testing.T) {
	t.Run("empty struct", func(t *testing.T) {
		var cfg Config
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid ListenAddress value: should not be empty", err.Error())
	})

	t.Run("missing tls cert", func(t *testing.T) {
		var cfg Config
		cfg.ListenAddress = ":8080"
		cfg.TLS.Enable = true
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid TLS config: invalid CertFile value: should not be empty", err.Error())
	})

	t.Run("missing tls key", func(t *testing.T) {
		var cfg Config
		cfg.ListenAddress = ":8080"
		cfg.TLS.Enable = true
		cfg.TLS.CertFile = "cert.pem"
		err := cfg.IsValid()
		require.Error(t, err)
		require.Equal(t, "invalid TLS config: invalid CertKey value: should not be empty", err.Error())
	})

	t.Run("valid no tls", func(t *testing.T) {
		var cfg Config
		cfg.ListenAddress = ":8080"
		err := cfg.IsValid()
		require.NoError(t, err)
	})

	t.Run("valid with tls", func(t *testing.T) {
		var cfg Config
		cfg.ListenAddress = ":8080"
		cfg.TLS.Enable = true
		cfg.TLS.CertFile = "cert.pem"
		cfg.TLS.CertKey = "key.pem"
		err := cfg.IsValid()
		require.NoError(t, err)
	})
}
