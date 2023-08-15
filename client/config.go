// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var idRE = regexp.MustCompile(`^[a-z0-9]{26}$`)

type Config struct {
	// SiteURL is the URL of the Mattermost installation to connect to.
	SiteURL string
	// AuthToken is a valid user session authentication token.
	AuthToken string
	// ChannelID is the id of the channel to start or join a call in.
	ChannelID string
	// ContextID is an id used to identify bot initiated sessions (e.g.
	// recording/transcription)
	ContextID string

	wsURL string
}

func (c *Config) Parse() error {
	if c.SiteURL == "" {
		return fmt.Errorf("invalid SiteURL value: should not be empty")
	}
	c.SiteURL = strings.TrimRight(strings.TrimSpace(c.SiteURL), "/")
	u, err := url.Parse(c.SiteURL)
	if err != nil {
		return fmt.Errorf("failed to parse SiteURL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid SiteURL scheme %q", u.Scheme)
	}

	if u.Scheme == "http" {
		u.Scheme = "ws"
		u.Path += mmWebSocketAPIPath
	} else {
		u.Scheme = "wss"
		u.Path += mmWebSocketAPIPath
	}
	c.wsURL = u.String()

	if c.AuthToken == "" {
		return fmt.Errorf("invalid AuthToken value: should not be empty")
	}

	if !idRE.MatchString(c.ChannelID) {
		return fmt.Errorf("invalid ChannelID value")
	}

	return nil
}
