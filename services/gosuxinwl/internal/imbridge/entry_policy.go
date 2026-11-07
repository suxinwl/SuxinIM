package imbridge

import (
	"context"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"github.com/suxinwl/GoSuxin/internal/accountsecurity"
)

// Entry URLs and policy zones have separate ingress headers. URL selection
// continues to use the configured Host; permission never trusts Host alone.
// Nginx must overwrite this header after validating TLS SNI/Host and transport
// source. A direct caller, missing/ambiguous header, public source or domain
// entry can never gain the internal policy by supplying a private-looking IP.
func requestEntryZone(r *http.Request) string {
	if !isAccessLoopback(r) {
		return "internet"
	}
	values := r.Header.Values("X-Puty-Access-Zone")
	if len(values) != 1 || values[0] != "lan" {
		return "internet"
	}
	internalHost := false
	for _, endpoint := range []string{LANEndpoints().HRM, LANEndpoints().IM} {
		u, err := url.Parse(endpoint)
		if err == nil && validEndpoint(endpoint) && strings.EqualFold(r.Host, u.Host) {
			internalHost = true
		}
	}
	if !internalHost {
		return "internet"
	}
	addresses := r.Header.Values("X-GoSuxin-Client-IP")
	if len(addresses) != 1 || !internalEntryAddress(addresses[0]) {
		return "internet"
	}
	return "lan"
}

func internalEntryAddress(raw string) bool {
	ip, err := netip.ParseAddr(raw)
	return err == nil && ip.Zone() == "" && (ip.Unmap().IsPrivate() || ip.Unmap().IsLoopback())
}

func accessZone(ctx context.Context, ip string, cidrs []string) string {
	address, err := netip.ParseAddr(ip)
	if err != nil || address.Zone() != "" {
		return "invalid"
	}
	if zone, present := accountsecurity.EntryZone(ctx); present {
		if zone == "lan" && !internalEntryAddress(ip) {
			return "internet"
		}
		return zone
	}
	// Retain the legacy address-only contract for trusted non-HTTP jobs. Every
	// public HTTP and signed internal access handler sets an explicit zone, so
	// this fallback cannot grant LAN policy to an unclassified HTTP request.
	return networkZone(ip, cidrs)
}
