// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package client

import (
	"fmt"
)

func (c *Client) joinCall() error {
	if err := c.wsSend(wsEventJoin, CallJoinMessage{
		ChannelID: c.cfg.ChannelID,
		JobID:     c.cfg.JobID,
	}, false); err != nil {
		return fmt.Errorf("failed to send ws msg: %w", err)
	}

	return nil
}

func (c *Client) leaveCall() error {
	if err := c.wsSend(wsEventLeave, nil, false); err != nil {
		return fmt.Errorf("failed to send ws msg: %w", err)
	}

	return nil
}

func (c *Client) reconnectCall() error {
	if err := c.wsSend(wsEventReconnect, CallReconnectMessage{
		ChannelID:      c.cfg.ChannelID,
		OriginalConnID: c.originalConnID,
		PrevConnID:     c.currentConnID,
	}, false); err != nil {
		return fmt.Errorf("failed to send ws msg: %w", err)
	}

	return nil
}
