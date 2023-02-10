// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"github.com/pion/webrtc/v3"
)

type trackAction int

const (
	trackActionAdd trackAction = iota + 1
	trackActionRemove
)

type trackActionContext struct {
	action trackAction
	track  webrtc.TrackLocal
}
