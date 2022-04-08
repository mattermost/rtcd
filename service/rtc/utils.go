// Copyright (c) 2022-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package rtc

import (
	"context"
	"fmt"
	"net"
	"time"
)

func resolveHost(host string, timeout time.Duration) (string, error) {
	var ip string
	r := net.Resolver{}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	addrs, err := r.LookupIP(ctx, "ip4", host)
	if err != nil {
		return ip, fmt.Errorf("failed to resolve host %q: %w", host, err)
	}
	if len(addrs) > 0 {
		ip = addrs[0].String()
	}
	return ip, err
}
