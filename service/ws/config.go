// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package ws

import (
	"fmt"
)

type Config struct {
	// ReadBufferSize specifies the size of the internal buffer
	// used to read from a ws connection.
	ReadBufferSize int
	// WriteBufferSize specifies the size of the internal buffer
	// used to wirte to a ws connection.
	WriteBufferSize int
}

func (c Config) IsValid() error {
	if c.ReadBufferSize <= 0 {
		return fmt.Errorf("invalid ReadBufferSize value: should be greater than zero")
	}
	if c.WriteBufferSize <= 0 {
		return fmt.Errorf("invalid WriteBufferSize value: should be greater than zero")
	}
	return nil
}
