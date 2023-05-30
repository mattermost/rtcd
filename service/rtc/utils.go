// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"fmt"
	"strings"
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

	// If the override is in full NAT mapping format (e.g. "EA/IA,EB/IB") we return
	// that directly.
	if strings.Contains(hostOverride, "/") {
		return strings.Split(hostOverride, ","), nil
	}

	// If the override is set we resolve it in case it's a hostname.
	if hostOverride != "" {
		hostOverrideIP, err = resolveHost(hostOverride, time.Second)
		if err != nil {
			return pairs, fmt.Errorf("failed to resolve host: %w", err)
		}
	}

	// Nothing to do at this point if no local IP was found.
	if len(localIPs) == 0 {
		return nil, nil
	}

	// If the override is set but no explicit mapping is given, we try to
	// generate one.
	if hostOverrideIP != "" {
		// If only one local interface is found, we map that to the given public ip
		// override.
		if len(localIPs) == 1 {
			return []string{
				fmt.Sprintf("%s/%s", hostOverrideIP, localIPs[0]),
			}, nil
		}

		// Otherwise we map the override to any non-loopback IP.
		for _, localAddr := range localIPs {
			// TODO: consider a better check to figure out if it's loopback.
			if localAddr == "127.0.0.1" {
				pairs = append(pairs, fmt.Sprintf("%s/%s", localAddr, localAddr))
			} else {
				pairs = append(pairs, fmt.Sprintf("%s/%s", hostOverrideIP, localAddr))
			}
		}

		return pairs, nil
	}

	// Nothing to do if no public address was found.
	if len(publicAddrsMap) == 0 {
		return nil, nil
	}

	// We finally try to generate a mapping from any public IP we have
	// found through STUN.
	for _, localAddr := range localIPs {
		publicAddr := publicAddrsMap[localAddr]
		if publicAddr == "" {
			publicAddr = localAddr
		}
		pairs = append(pairs, fmt.Sprintf("%s/%s", publicAddr, localAddr))
	}

	return pairs, nil
}
