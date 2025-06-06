// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"fmt"
	"math/rand"
	"net/netip"
	"strings"
	"time"

	"github.com/mattermost/rtcd/service/random"

	"github.com/pion/webrtc/v4"
)

type trackType string

const (
	trackTypeVoice       trackType = "voice"
	trackTypeScreen      trackType = "screen"
	trackTypeScreenAudio trackType = "screen-audio"
)

var trackTypes = map[string]trackType{
	"voice":        trackTypeVoice,
	"screen":       trackTypeScreen,
	"screen-audio": trackTypeScreenAudio,
}

func genTrackID(tt trackType, baseID string) string {
	return string(tt) + "_" + baseID + "_" + random.NewID()[0:8]
}

func getTrackIndex(mimeType, rid string) string {
	return mimeType + "_" + rid
}

func isValidTrackID(trackID string) bool {
	fields := strings.Split(trackID, "_")
	if len(fields) != 3 {
		return false
	}

	return trackTypes[fields[0]] != ""
}

func generateAddrsPairs(localIPs []netip.Addr, publicAddrsMap map[netip.Addr]string, hostOverride string, dualStack bool) ([]string, error) {
	var err error
	var pairs []string
	var hostOverrideIP string

	// If the override is in full NAT mapping format (e.g. "EA/IA,EB/IB") we return
	// that directly.
	if strings.Contains(hostOverride, "/") {
		return strings.Split(hostOverride, ","), nil
	}

	ipNetwork := "ip4"
	if dualStack {
		ipNetwork = "ip"
	}

	// If the override is set we resolve it in case it's a hostname.
	if hostOverride != "" {
		hostOverrideIP, err = resolveHost(hostOverride, ipNetwork, time.Second)
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
		hostOverrideAddr, err := netip.ParseAddr(hostOverrideIP)
		if err != nil {
			return nil, fmt.Errorf("failed to parse hostOverrideIP: %w", err)
		}

		// If only one local interface is found, we map that to the given public ip
		// override.
		if len(localIPs) == 1 && areAddressesSameStack(hostOverrideAddr, localIPs[0]) {
			return []string{
				fmt.Sprintf("%s/%s", hostOverrideAddr.String(), localIPs[0].String()),
			}, nil
		}

		// Otherwise we map the override to any non-loopback IP.
		for _, localAddr := range localIPs {
			if localAddr.IsLoopback() {
				pairs = append(pairs, fmt.Sprintf("%s/%s", localAddr.String(), localAddr.String()))
			} else if areAddressesSameStack(hostOverrideAddr, localAddr) {
				pairs = append(pairs, fmt.Sprintf("%s/%s", hostOverrideAddr.String(), localAddr.String()))
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
			publicAddr = localAddr.String()
		}
		pairs = append(pairs, fmt.Sprintf("%s/%s", publicAddr, localAddr.String()))
	}

	return pairs, nil
}

func getExternalAddrMapFromHostOverride(override string, publicAddrsMap map[netip.Addr]string) map[string]bool {
	m := make(map[string]bool)

	// If the override is empty we add any external address found through STUN.
	if override == "" {
		for _, addr := range publicAddrsMap {
			m[addr] = true
		}
		return m
	}

	// If the override is set and it's a single address we only need that.
	if !strings.Contains(override, "/") {
		m[override] = true
		return m
	}

	// Otherwise we need to add all the external addresses passed through the advanced syntax.
	pairs := strings.Split(override, ",")
	for _, p := range pairs {
		pair := strings.Split(p, "/")
		if len(pair) != 2 {
			continue
		}

		if pair[0] != pair[1] {
			m[pair[0]] = true
		}
	}

	return m
}

func pickRandom[S ~[]*E, E any](s S) *E {
	if len(s) == 0 {
		return nil
	}
	return s[rand.Intn(len(s))]
}

func getTrackMimeType(track webrtc.TrackLocal) string {
	if localTrack, ok := track.(*webrtc.TrackLocalStaticRTP); ok {
		return localTrack.Codec().MimeType
	}

	return "unknown"
}
