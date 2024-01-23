// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
)

type ServerConfig struct {
	// ICEAddressUDP specifies the UDP address the RTC service should listen on.
	ICEAddressUDP string `toml:"ice_address_udp"`
	// ICEPortUDP specifies the UDP port the RTC service should listen to.
	ICEPortUDP int `toml:"ice_port_udp"`
	// ICEAddressTCP specifies the TCP address the RTC service should listen on.
	ICEAddressTCP string `toml:"ice_address_tcp"`
	// ICEPortTCP specifies the TCP port the RTC service should listen to.
	ICEPortTCP int `toml:"ice_port_tcp"`
	// ICEHostOverride optionally specifies an IP address (or hostname)
	// to be used as the main host ICE candidate.
	ICEHostOverride string `toml:"ice_host_override"`
	// ICEHostPortOverride optionally specifies a port number to override the one
	// used to listen on when sharing host candidates.
	ICEHostPortOverride int `toml:"ice_host_port_override"`
	// A list of ICE server (STUN/TURN) configurations to use.
	ICEServers ICEServers `toml:"ice_servers"`
	TURNConfig TURNConfig `toml:"turn"`
	// EnableIPv6 specifies whether or not IPv6 should be used.
	EnableIPv6 bool `toml:"enable_ipv6"`
}

func (c ServerConfig) IsValid() error {
	if c.ICEAddressUDP != "" && net.ParseIP(c.ICEAddressUDP) == nil {
		return fmt.Errorf("invalid ICEAddressUDP value: not a valid address")
	}

	if c.ICEAddressTCP != "" && net.ParseIP(c.ICEAddressTCP) == nil {
		return fmt.Errorf("invalid ICEAddressTCP value: not a valid address")
	}

	if c.ICEPortUDP < 80 || c.ICEPortUDP > 49151 {
		return fmt.Errorf("invalid ICEPortUDP value: %d is not in allowed range [80, 49151]", c.ICEPortUDP)
	}

	if c.ICEPortTCP < 80 || c.ICEPortTCP > 49151 {
		return fmt.Errorf("invalid ICEPortTCP value: %d is not in allowed range [80, 49151]", c.ICEPortTCP)
	}

	if err := c.ICEServers.IsValid(); err != nil {
		return fmt.Errorf("invalid ICEServers value: %w", err)
	}

	if err := c.TURNConfig.IsValid(); err != nil {
		return fmt.Errorf("invalid TURNConfig: %w", err)
	}

	if c.ICEHostPortOverride != 0 && (c.ICEHostPortOverride < 80 || c.ICEHostPortOverride > 49151) {
		return fmt.Errorf("invalid ICEHostPortOverride value: %d is not in allowed range [80, 49151]", c.ICEHostPortOverride)
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

		if !c.IsSTUN() && !c.IsTURN() {
			return fmt.Errorf("URL is not a valid STUN/TURN server")
		}
	}
	return nil
}

func (c ICEServerConfig) IsTURN() bool {
	for _, u := range c.URLs {
		if !strings.HasPrefix(u, "turn:") && !strings.HasPrefix(u, "turns:") {
			return false
		}
	}
	return len(c.URLs) > 0
}

func (c ICEServerConfig) IsSTUN() bool {
	for _, u := range c.URLs {
		if !strings.HasPrefix(u, "stun:") && !strings.HasPrefix(u, "stuns:") {
			return false
		}
	}
	return len(c.URLs) > 0
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
		if cfg.IsSTUN() {
			return cfg.URLs[0]
		}
	}
	return ""
}

func (s *ICEServers) Decode(value string) error {
	fmt.Println(value)

	var urls []string
	err := json.Unmarshal([]byte(value), &urls)
	if err == nil {
		iceServers := []ICEServerConfig{
			{
				URLs: urls,
			},
		}
		*s = iceServers
		return nil
	}

	return json.Unmarshal([]byte(value), s)
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
