// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"fmt"
)

type ServerConfig struct {
	// ICEPortUDP specifies the UDP port the RTC service should listen to.
	ICEPortUDP int `toml:"ice_port_udp"`
	// ICEHostOverride optionally specifies an IP address (or hostname)
	// to be used as the main host ICE candidate.
	ICEHostOverride string `toml:"ice_host_override"`
}

func (c ServerConfig) IsValid() error {
	if c.ICEPortUDP < 80 || c.ICEPortUDP > 49151 {
		return fmt.Errorf("invalid ICEPortUDP value: %d is not in allowed range [80, 49151]", c.ICEPortUDP)
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
