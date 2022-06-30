// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"fmt"
	"net/url"
	"time"

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
	AllowSelfRegistration bool `toml:"allow_self_registration"`
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

	if err := c.Logger.IsValid(); err != nil {
		return err
	}

	return nil
}

func (c *Config) SetDefaults() {
	c.API.HTTP.ListenAddress = ":8045"
	c.RTC.ICEPortUDP = 8443
	c.RTC.TURNConfig.CredentialsExpirationMinutes = 1440
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

type ClientConfig struct {
	httpURL string
	wsURL   string

	ClientID          string
	AuthKey           string
	URL               string
	ReconnectInterval time.Duration
}

func (c *ClientConfig) Parse() error {
	if c.URL == "" {
		return fmt.Errorf("invalid URL value: should not be empty")
	}

	u, err := url.Parse(c.URL)
	if err != nil {
		return fmt.Errorf("failed to parse url: %w", err)
	}

	if u.Host == "" {
		return fmt.Errorf("invalid url host: should not be empty")
	}

	switch u.Scheme {
	case "http":
		c.httpURL = c.URL
		u.Scheme = "ws"
		u.Path = "/ws"
		c.wsURL = u.String()
	case "https":
		c.httpURL = c.URL
		u.Scheme = "wss"
		u.Path = "/ws"
		c.wsURL = u.String()
	default:
		return fmt.Errorf("invalid url scheme: %q is not valid", u.Scheme)
	}

	if c.ReconnectInterval <= 0 {
		c.ReconnectInterval = defaultReconnectInterval
	}

	return nil
}
