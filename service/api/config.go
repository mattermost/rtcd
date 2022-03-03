// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package api

import (
	"fmt"
)

type TLSConfig struct {
	Enable   bool
	CertFile string `toml:"cert_file"`
	CertKey  string `toml:"cert_key"`
}

func (c TLSConfig) IsValid() error {
	if c.Enable {
		if c.CertFile == "" {
			return fmt.Errorf("invalid CertFile value: should not be empty")
		}
		if c.CertKey == "" {
			return fmt.Errorf("invalid CertKey value: should not be empty")
		}
	}
	return nil
}

type Config struct {
	ListenAddress string `toml:"listen_address"`
	TLS           TLSConfig
}

func (c Config) IsValid() error {
	if c.ListenAddress == "" {
		return fmt.Errorf("invalid ListenAddress value: should not be empty")
	}
	if err := c.TLS.IsValid(); err != nil {
		return fmt.Errorf("invalid TLS config: %w", err)
	}
	return nil
}
