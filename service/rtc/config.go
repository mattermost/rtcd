// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
)

type ServerConfig struct {
	// ICEAddressUDP specifies the UDP address the RTC service should listen on.
	ICEAddressUDP ICEAddress `toml:"ice_address_udp"`
	// ICEPortUDP specifies the UDP port the RTC service should listen to.
	ICEPortUDP int `toml:"ice_port_udp"`
	// ICEAddressTCP specifies the TCP address the RTC service should listen on.
	ICEAddressTCP ICEAddress `toml:"ice_address_tcp"`
	// ICEPortTCP specifies the TCP port the RTC service should listen to.
	ICEPortTCP int `toml:"ice_port_tcp"`
	// ICEHostOverride optionally specifies an IP address (or hostname)
	// to be used as the main host ICE candidate.
	ICEHostOverride string `toml:"ice_host_override"`
	// ICEHostPortOverride optionally specifies a port number to override the one
	// used to listen on when sharing host candidates.
	ICEHostPortOverride ICEHostPortOverride `toml:"ice_host_port_override"`
	// A list of ICE server (STUN/TURN) configurations to use.
	ICEServers ICEServers `toml:"ice_servers"`
	TURNConfig TURNConfig `toml:"turn"`
	// EnableIPv6 specifies whether or not IPv6 should be used.
	EnableIPv6 bool `toml:"enable_ipv6"`
	// UDPSocketsCount controls the number of listening UDP sockets used for each local
	// network address. A larger number can improve performance by reducing contention
	// over a few file descriptors. At the same time, it will cause more file descriptors
	// to be open. The default is a dynamic value that scales with the number of available CPUs with
	// a constant multiplier of 100. E.g. On a 4 CPUs node, 400 sockets per local
	// network address will be open.
	UDPSocketsCount int `toml:"udp_sockets_count"`
}

func (c ServerConfig) IsValid() error {
	if err := c.ICEAddressUDP.IsValid(); err != nil {
		return fmt.Errorf("invalid ICEAddressUDP value: %w", err)
	}

	if err := c.ICEAddressTCP.IsValid(); err != nil {
		return fmt.Errorf("invalid ICEAddressTCP value: %w", err)
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

	if err := c.ICEHostPortOverride.IsValid(); err != nil {
		return fmt.Errorf("invalid ICEHostPortOverride value: %w", err)
	}

	if c.UDPSocketsCount <= 0 {
		return fmt.Errorf("invalid UDPSocketsCount value: should be greater than 0")
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
	// Props specifies some properties for the session.
	Props SessionProps
}

type SessionProps map[string]any

func (p SessionProps) ChannelID() string {
	val, _ := p["channelID"].(string)
	return val
}

func (p SessionProps) AV1Support() bool {
	val, _ := p["av1Support"].(bool)
	return val
}

func (p SessionProps) DCSignaling() bool {
	val, _ := p["dcSignaling"].(bool)
	return val
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

func (c *SessionConfig) FromMap(m map[string]any) error {
	if c == nil {
		return fmt.Errorf("invalid nil config")
	}
	if m == nil {
		return fmt.Errorf("invalid nil map")
	}

	c.GroupID, _ = m["groupID"].(string)
	c.CallID, _ = m["callID"].(string)
	c.UserID, _ = m["userID"].(string)
	c.SessionID, _ = m["sessionID"].(string)
	c.Props = SessionProps{
		"channelID":   m["channelID"],
		"av1Support":  m["av1Support"],
		"dcSignaling": m["dcSignaling"],
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

type ICEHostPortOverride string

func (s *ICEHostPortOverride) SinglePort() int {
	if s == nil {
		return 0
	}
	p, _ := strconv.Atoi(string(*s))
	return p
}

func (s *ICEHostPortOverride) ParseMap() (map[string]int, error) {
	if s == nil {
		return nil, fmt.Errorf("should not be nil")
	}

	if *s == "" {
		return nil, nil
	}

	pairs := strings.Split(string(*s), ",")

	m := make(map[string]int, len(pairs))
	ports := make(map[int]bool, len(pairs))

	for _, p := range pairs {
		pair := strings.Split(p, "/")
		if len(pair) != 2 {
			return nil, fmt.Errorf("invalid map pairing syntax")
		}

		port, err := strconv.Atoi(pair[1])
		if err != nil {
			return nil, fmt.Errorf("failed to parse port number: %w", err)
		}

		if _, ok := m[pair[0]]; ok {
			return nil, fmt.Errorf("duplicate mapping found for %s", pair[0])
		}

		if ports[port] {
			return nil, fmt.Errorf("duplicate port found for %d", port)
		}

		m[pair[0]] = port
		ports[port] = true
	}

	return m, nil
}

func (s *ICEHostPortOverride) IsValid() error {
	if s == nil {
		return fmt.Errorf("should not be nil")
	}

	if *s == "" {
		return nil
	}

	if port := s.SinglePort(); port != 0 {
		if port < 80 || port > 49151 {
			return fmt.Errorf("%d is not in allowed range [80, 49151]", port)
		}
		return nil
	}

	if _, err := s.ParseMap(); err != nil {
		return fmt.Errorf("failed to parse mapping: %w", err)
	}

	return nil
}

func (s *ICEHostPortOverride) UnmarshalTOML(data interface{}) error {
	switch t := data.(type) {
	case string:
		*s = ICEHostPortOverride(data.(string))
		return nil
	case int, int32, int64:
		*s = ICEHostPortOverride(fmt.Sprintf("%v", data))
	default:
		return fmt.Errorf("unknown type %T", t)
	}

	return nil
}

type ICEAddress string

func (a ICEAddress) Parse() []string {
	if a == "" {
		return nil
	}

	var addrs []string
	for _, addr := range strings.Split(string(a), ",") {
		addrs = append(addrs, strings.TrimSpace(addr))
	}
	return addrs
}

func (a ICEAddress) IsValid() error {
	if a == "" {
		return nil
	}

	for _, addr := range a.Parse() {
		if net.ParseIP(addr) == nil {
			return fmt.Errorf("invalid ICEAddress value: %s is not a valid IP address", addr)
		}
	}

	return nil
}
