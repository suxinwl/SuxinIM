// Copyright 2026 GoSuxin OpenIM upgrade contributors.
// Licensed under the Apache License, Version 2.0.
package service

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"

	"github.com/livekit/livekit-server/pkg/config"
)

func embeddedTURNTLSURL(c config.TURNConfig) string {
	port := int(c.AdvertisedTLSPort)
	if port == 0 {
		port = 443
	}
	return fmt.Sprintf("turns:%s?transport=tcp", net.JoinHostPort(c.Domain, strconv.Itoa(port)))
}

// No CIDR/subnet widening: only addresses discovered on this SFU are eligible.
// newTURNRelayPolicy wraps the resulting socket and enforces RTC media ports,
// so this permission cannot expose the HTTP/RPC/database ports on the host.
// Standalone TURN deliberately keeps the unmodified upstream permission policy.
func allowTURNLocalMediaPeer(enabled, standalone bool, localIPs map[netip.Addr]struct{}, peer net.IP) bool {
	if !enabled || standalone {
		return false
	}
	addr, ok := netip.AddrFromSlice(peer)
	if !ok {
		return false
	}
	_, ok = localIPs[addr.Unmap().WithZone("")]
	return ok
}
