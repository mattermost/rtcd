// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"fmt"
	"strings"
)

type ServerConfig struct {
	// ICEPortUDP specifies the UDP port the RTC service should listen to.
	ICEPortUDP int `toml:"ice_port_udp"`
	// ICEHostOverride optionally specifies an IP address (or hostname)
	// to be used as the main host ICE candidate.
	ICEHostOverride string `toml:"ice_host_override"`
	// A list of ICE server (STUN/TURN) configurations to use.
	ICEServers ICEServers `toml:"ice_servers"`
}

func (c ServerConfig) IsValid() error {
	if c.ICEPortUDP < 80 || c.ICEPortUDP > 49151 {
		return fmt.Errorf("invalid ICEPortUDP value: %d is not in allowed range [80, 49151]", c.ICEPortUDP)
	}

	if err := c.ICEServers.IsValid(); err != nil {
		return fmt.Errorf("invalid ICEServers value: %w", err)
	}

	return nil
}

type SessionConfig struct {
	// GroupID specifies the id of the group the session should belong to.
	GroupID string
	// CallID specifies the id of the call the session should belong to.
	CallID string
	// UserID specifies the id of the user the session belongs to.
	UserID string
	// SessionID specifies the unique identifier for the session.
	SessionID string
}

func (c SessionConfig) IsValid() error {
	if c.GroupID == "" {
		return fmt.Errorf("invalid GroupID value: should not be empty")
	}

	if c.CallID == "" {
		return fmt.Errorf("invalid CallID value: should not be empty")
	}

	if c.UserID == "" {
		return fmt.Errorf("invalid UserID value: should not be empty")
	}

	if c.SessionID == "" {
		return fmt.Errorf("invalid SessionID value: should not be empty")
	}

	return nil
}

type ICEServerConfig struct {
	URLs       []string `toml:"urls" json:"urls"`
	Username   string   `toml:"username,omitempty" json:"username,omitempty"`
	Credential string   `toml:"credential,omitempty" json:"credential,omitempty"`
}

type ICEServers []ICEServerConfig

func (c ICEServerConfig) IsValid() error {
	if len(c.URLs) == 0 {
		return fmt.Errorf("invalid empty URLs")
	}
	for _, u := range c.URLs {
		if u == "" {
			return fmt.Errorf("invalid empty URL")
		}

		if !strings.HasPrefix(u, "stun:") && !strings.HasPrefix(u, "turn:") {
			return fmt.Errorf("URL is not a valid STUN/TURN server")
		}
	}
	return nil
}

func (s ICEServers) IsValid() error {
	for _, cfg := range s {
		if err := cfg.IsValid(); err != nil {
			return err
		}
	}
	return nil
}

func (s ICEServers) getSTUN() string {
	for _, cfg := range s {
		for _, u := range cfg.URLs {
			if strings.HasPrefix(u, "stun:") {
				return u
			}
		}
	}
	return ""
}

func (s *ICEServers) UnmarshalTOML(data interface{}) error {
	d, ok := data.([]interface{})
	if !ok {
		return fmt.Errorf("invalid type %T", data)
	}

	var iceServers []ICEServerConfig
	for _, obj := range d {
		var server ICEServerConfig

		switch t := obj.(type) {
		case string:
			server.URLs = append(server.URLs, obj.(string))
		case map[string]interface{}:
			m := obj.(map[string]interface{})
			urls, _ := m["urls"].([]interface{})
			for _, u := range urls {
				uVal, _ := u.(string)
				server.URLs = append(server.URLs, uVal)
			}
			server.Username, _ = m["username"].(string)
			server.Credential, _ = m["credential"].(string)
		default:
			return fmt.Errorf("unknown type %T", t)
		}

		iceServers = append(iceServers, server)
	}

	*s = iceServers

	return nil
}
