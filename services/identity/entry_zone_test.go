package identity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEntryZoneRequiresConsistentTrustedIngressRatherThanSourceSubnet(t *testing.T) {
	t.Setenv("TRUSTED_ENTRY_PROXY_CIDRS", "172.30.38.1/32")
	for _, tc := range []struct {
		name, want string
		modify     func(*http.Request)
	}{
		{"private-routed-ip-entry", "lan", func(*http.Request) {}},
		{"domain-on-lan", "internet", func(r *http.Request) {
			r.Header.Set("X-Puty-Entry", "internet")
			r.Header.Set("X-Puty-Access-Zone", "internet")
		}},
		{"domain-selector-conflict", "internet", func(r *http.Request) { r.Header.Set("X-Puty-Entry", "internet") }},
		{"domain-host-conflict", "internet", func(r *http.Request) { r.Host = "im.szputy.com:9885" }},
		{"unconfigured-host", "internet", func(r *http.Request) { r.Host = "unknown.invalid:9885" }},
		{"ip-entry-public-assertion", "internet", func(r *http.Request) { r.Header.Set("X-Puty-Access-Zone", "internet") }},
		{"private-address-no-zone", "internet", func(r *http.Request) { r.Header.Del("X-Puty-Access-Zone") }},
		{"missing-entry", "internet", func(r *http.Request) { r.Header.Del("X-Puty-Entry") }},
		{"duplicate-zone", "internet", func(r *http.Request) { r.Header.Add("X-Puty-Access-Zone", "lan") }},
		{"duplicate-entry", "internet", func(r *http.Request) { r.Header.Add("X-Puty-Entry", "lan") }},
		{"comma-zone", "internet", func(r *http.Request) { r.Header.Set("X-Puty-Access-Zone", "lan,internet") }},
		{"public-address", "internet", func(r *http.Request) { r.Header.Set("X-Real-IP", "203.0.113.8") }},
		{"untrusted-private-peer", "internet", func(r *http.Request) { r.RemoteAddr = "192.168.14.8:1234" }},
		{"conflicting-addresses", "internet", func(r *http.Request) { r.Header.Set("X-GoSuxin-Client-IP", "10.0.0.1") }},
		{"duplicate-real-address", "internet", func(r *http.Request) { r.Header.Add("X-Real-IP", "192.168.14.8") }},
		{"xff-cannot-grant-lan", "internet", func(r *http.Request) { r.Header.Del("X-Real-IP"); r.Header.Set("X-Forwarded-For", "192.168.14.8") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "https://192.168.10.11:9885/enterprise/v1/session/state", nil)
			r.RemoteAddr = "172.30.38.1:1234"
			r.Header.Set("X-Real-IP", "192.168.14.8")
			r.Header.Set("X-Puty-Entry", "lan")
			r.Header.Set("X-Puty-Access-Zone", "lan")
			tc.modify(r)
			if got := RequestEntryZone(r); got != tc.want {
				t.Fatalf("entry zone %q, want %q", got, tc.want)
			}
			if got := EntryZoneFromContext(WithRequestEntry(context.Background(), r)); got != tc.want {
				t.Fatal("request context lost trusted zone")
			}
		})
	}
}

func TestEntryZoneIsHMACBoundAndKeepsActualClientIP(t *testing.T) {
	t.Setenv("HRM_SSO_REQUIRED", "true")
	key := strings.Repeat("k", 32)
	t.Setenv("HRM_BRIDGE_HMAC_KEY", key)
	seen := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-HRM-Signature") != Sign(key, r.Header.Get("X-HRM-Timestamp"), r.Header.Get("X-HRM-Nonce"), r.Method, r.URL.Path, raw) {
			t.Error("entry zone was not part of the signed body")
		}
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if len(body) != 7 || body["clientIP"] != "192.168.14.8" {
			t.Error("entry classification replaced audit IP or widened the contract")
		}
		seen = append(seen, body["entryZone"].(string))
		_ = json.NewEncoder(w).Encode(map[string]any{"errCode": 0, "data": AccessDecision{Allowed: true}})
	}))
	defer server.Close()
	t.Setenv("HRM_ACCESS_CHECK_URL", server.URL+"/internal/im-access/check")
	for _, ctx := range []context.Context{WithEntryZone(context.Background(), "lan"), WithEntryZone(context.Background(), "internet"), context.Background()} {
		if _, err := CheckLogicalAccess(ctx, "emp_fixture", 3, "pc", "192.168.14.8", "connect", ""); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(seen, ",") != "lan,internet,internet" {
		t.Fatal("missing context fell back to a private source IP", seen)
	}
}

func TestMalformedRPCZoneFailsPublicAndCannotMutateParentMetadata(t *testing.T) {
	keys := []string{"existing"}
	base := context.WithValue(context.Background(), customHeaderKey, keys)
	for _, values := range [][]string{nil, {"lan", "internet"}, {"LAN"}, {"lan,internet"}} {
		ctx := context.WithValue(base, EntryZoneMetadataKey, values)
		if EntryZoneFromContext(ctx) != "internet" {
			t.Fatal("ambiguous RPC metadata granted LAN")
		}
	}
	ctx := WithEntryZone(WithEntryZone(base, "lan"), "internet")
	if len(keys) != 1 || len(ctx.Value(customHeaderKey).([]string)) != 2 || EntryZoneFromContext(ctx) != "internet" {
		t.Fatal("RPC metadata duplicated or modified parent")
	}
}
