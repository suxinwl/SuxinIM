package identity

import (
	"context"
	"net"
	"net/http"
	"net/url"
)

// EntryZoneMetadataKey is private RPC metadata written by the authenticated
// ingress, never copied from public HTTP headers or message payloads.
const EntryZoneMetadataKey = "puty-entry-zone"

func singleHeader(r *http.Request, name string) string {
	values := r.Header.Values(name)
	if len(values) != 1 {
		return ""
	}
	return values[0]
}

// RequestEntryZone trusts only an ingress assertion. X-Puty-Entry retains its
// independent URL-profile meaning; source IP alone can never grant LAN access.
// The ingress also validates the client-facing TLS/Host before writing this
// single-valued access header. Ambiguous, legacy and direct requests are public.
func RequestEntryZone(r *http.Request) string {
	if !TrustedProxy(r.RemoteAddr) || singleHeader(r, "X-Puty-Access-Zone") != "lan" || singleHeader(r, "X-Puty-Entry") != "lan" {
		return "internet"
	}
	origin, err := url.Parse(EntryOrigin(r))
	if err != nil || origin.Scheme != "https" || origin.Hostname() != "192.168.10.11" || origin.Host != r.Host || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return "internet"
	}
	// Both address assertions are overwritten by the ingress. Conflicting
	// assertions must not turn an otherwise public request into a LAN request.
	gosuxin, real := r.Header.Values("X-GoSuxin-Client-IP"), r.Header.Values("X-Real-IP")
	if len(gosuxin) > 1 || len(real) > 1 || len(gosuxin) == 1 && len(real) == 1 && gosuxin[0] != real[0] {
		return "internet"
	}
	address, err := ClientIP(r)
	ip := net.ParseIP(address)
	if err != nil || ip == nil || !ip.IsPrivate() && !ip.IsLoopback() {
		return "internet"
	}
	return "lan"
}

func EntryZoneFromContext(ctx context.Context) string {
	values, ok := ctx.Value(EntryZoneMetadataKey).([]string)
	if ok && len(values) == 1 && values[0] == "lan" {
		return "lan"
	}
	return "internet"
}

// WithEntryZone is only for trusted ingress extraction or a verified private
// signed body. The same fact is transported by the locked RPC interceptors.
func WithEntryZone(ctx context.Context, zone string) context.Context {
	if zone != "lan" {
		zone = "internet"
	}
	keys, _ := ctx.Value(customHeaderKey).([]string)
	keys = append([]string(nil), keys...)
	found := false
	for _, key := range keys {
		found = found || key == EntryZoneMetadataKey
	}
	if !found {
		keys = append(keys, EntryZoneMetadataKey)
	}
	ctx = context.WithValue(ctx, customHeaderKey, keys)
	return context.WithValue(ctx, EntryZoneMetadataKey, []string{zone})
}

func WithRequestEntry(ctx context.Context, r *http.Request) context.Context {
	return WithEntryZone(ctx, RequestEntryZone(r))
}
