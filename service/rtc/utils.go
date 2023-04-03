// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"fmt"
	"time"

	"github.com/mattermost/rtcd/service/random"

	"github.com/pion/webrtc/v3"
)

func genTrackID(trackType, baseID string) string {
	return trackType + "_" + baseID + "_" + random.NewID()[0:8]
}

func getTrackType(kind webrtc.RTPCodecType) string {
	if kind == webrtc.RTPCodecTypeAudio {
		return "audio"
	}

	if kind == webrtc.RTPCodecTypeVideo {
		return "video"
	}

	return "unknown"
}

func generateAddrsPairs(localIPs []string, publicAddrsMap map[string]string, hostOverride string) ([]string, error) {
	var err error
	var pairs []string
	var hostOverrideIP string

	if hostOverride != "" {
		hostOverrideIP, err = resolveHost(hostOverride, time.Second)
		if err != nil {
			return pairs, fmt.Errorf("failed to resolve host: %w", err)
		}
	}

	usedPublicAddrs := map[string]bool{}
	for _, localAddr := range localIPs {
		publicAddr := publicAddrsMap[localAddr]

		// If an override was explicitly provided we enforce that.
		if hostOverrideIP != "" {
			publicAddr = hostOverrideIP
		}

		if publicAddr != "" && !usedPublicAddrs[publicAddr] {
			// if a public IP has not been used yet we map it to
			// the first matching local ip.
			pairs = append(pairs, fmt.Sprintf("%s/%s", publicAddr, localAddr))
			usedPublicAddrs[publicAddr] = true
		} else {
			// if a public IP has been used already we map
			// any successive matching local ips to themselves.
			pairs = append(pairs, fmt.Sprintf("%s/%s", localAddr, localAddr))
		}
	}

	// If no public address was found/set there's no point in generating pairs.
	if len(usedPublicAddrs) == 0 {
		return nil, nil
	}

	return pairs, nil
}
