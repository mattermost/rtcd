// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package random

import (
	"bytes"
	"encoding/base32"

	"github.com/pborman/uuid"
)

const charset = "ybndrfg8ejkmcpqxot1uwisza345h769"

var encoding = base32.NewEncoding(charset)

// NewID is a globally unique identifier.  It is a [A-Z0-9] string 26
// characters long.  It is a UUID version 4 Guid that is zbased32 encoded
// with the padding stripped off.
func NewID() string {
	var b bytes.Buffer
	encoder := base32.NewEncoder(encoding, &b)
	if _, err := encoder.Write(uuid.NewRandom()); err != nil {
		return ""
	}
	encoder.Close()
	b.Truncate(26) // removes the '==' padding
	return b.String()
}
