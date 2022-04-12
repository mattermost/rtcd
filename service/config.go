// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package service

import (
	"fmt"
	"net/url"

	"github.com/mattermost/rtcd/service/api"
	"github.com/mattermost/rtcd/service/rtc"
)

type AdminConfig struct {
	// Whether or not to enable admin API access.
	Enable bool `toml:"enable"`
	// The secret key used to authenticate admin requests.
	SecretKey string `toml:"secret_key"`
}

func (c AdminConfig) IsValid() error {
	if !c.Enable {
		return nil
	}

	if c.SecretKey == "" {
		return fmt.Errorf("invalid SecretKey value: should not be empty")
	}

	return nil
}

type APIConfig struct {
	HTTP  api.Config  `toml:"http"`
	Admin AdminConfig `toml:"admin"`
}

type Config struct {
	API   APIConfig
	RTC   rtc.ServerConfig
	Store StoreConfig
}

func (c APIConfig) IsValid() error {
	if err := c.Admin.IsValid(); err != nil {
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

	if err := c.Store.IsValid(); err != nil {
		return err
	}

	return nil
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

	ClientID string
	AuthKey  string
	URL      string
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

	return nil
}
