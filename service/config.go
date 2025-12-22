// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"fmt"

	"github.com/mattermost/rtcd/service/auth"

	"github.com/mattermost/rtcd/logger"
	"github.com/mattermost/rtcd/service/api"
	"github.com/mattermost/rtcd/service/rtc"
)

type SecurityConfig struct {
	// Whether or not to enable admin API access.
	EnableAdmin bool `toml:"enable_admin"`
	// The secret key used to authenticate admin requests.
	AdminSecretKey string `toml:"admin_secret_key"`
	// Whether or not to allow clients to self-register.
	AllowSelfRegistration bool                    `toml:"allow_self_registration"`
	SessionCache          auth.SessionCacheConfig `toml:"session_cache"`
}

func (c SecurityConfig) IsValid() error {
	if !c.EnableAdmin {
		return nil
	}

	if c.AdminSecretKey == "" {
		return fmt.Errorf("invalid AdminSecretKey value: should not be empty")
	}

	return nil
}

type APIConfig struct {
	HTTP     api.Config     `toml:"http"`
	Security SecurityConfig `toml:"security"`
}

type Config struct {
	API    APIConfig
	RTC    rtc.ServerConfig
	Store  StoreConfig
	Logger logger.Config
}

func (c APIConfig) IsValid() error {
	if err := c.Security.IsValid(); err != nil {
		return fmt.Errorf("failed to validate admin config: %w", err)
	}

	if err := c.HTTP.IsValid(); err != nil {
		return fmt.Errorf("failed to validate http config: %w", err)
	}

	return nil
}

func (c Config) IsValid() error {
	if err := c.API.IsValid(); err != nil {
		return err
	}

	if err := c.RTC.IsValid(); err != nil {
		return err
	}

	if err := c.Store.IsValid(); err != nil {
		return err
	}

	return c.Logger.IsValid()
}

func (c *Config) SetDefaults() {
	c.API.HTTP.ListenAddress = ":8045"
	c.API.Security.SessionCache.ExpirationMinutes = 1440
	c.RTC.ICEPortUDP = 8443
	c.RTC.ICEPortTCP = 8443
	c.RTC.TURNConfig.CredentialsExpirationMinutes = 1440
	c.RTC.UDPSocketsCount = rtc.GetDefaultUDPListeningSocketsCount()
	c.RTC.NACKBufferSize = 256       // Default: 256 packets (~8.5 seconds at 30fps)
	c.RTC.NACKDisableCopy = true     // Default: true for backward compatibility (but not recommended)
	c.Store.DataSource = "/tmp/rtcd_db"
	c.Logger.EnableConsole = true
	c.Logger.ConsoleJSON = false
	c.Logger.ConsoleLevel = "INFO"
	c.Logger.EnableFile = true
	c.Logger.FileJSON = true
	c.Logger.FileLocation = "rtcd.log"
	c.Logger.FileLevel = "DEBUG"
	c.Logger.EnableColor = false
}

type StoreConfig struct {
	DataSource string `toml:"data_source"`
}

func (c StoreConfig) IsValid() error {
	if c.DataSource == "" {
		return fmt.Errorf("invalid DataSource value: should not be empty")
	}
	return nil
}
