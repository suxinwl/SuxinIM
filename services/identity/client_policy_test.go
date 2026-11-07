package identity

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogicalClientsSeparateTransportAndPolicyAndSupportIOS(t *testing.T) {
	for _, row := range []struct {
		platform, policy int
		client, expected string
	}{
		{1, 0, "", "ios"}, {2, 0, "", "android"}, {3, 0, "", "pc"}, {5, 0, "", "web_desktop"}, {5, 2, "", "web_mobile"}, {5, 5, "web_mobile", "web_mobile"},
	} {
		got, err := LogicalClient(row.platform, row.policy, row.client)
		if err != nil || got != row.expected {
			t.Fatal("logical client/transport mapping changed", row.platform, row.expected)
		}
	}
	for _, row := range []struct {
		platform int
		client   string
	}{{1, "android"}, {2, "pc"}, {3, "web_desktop"}, {5, "ios"}, {10, "pc"}, {5, "Web_Mobile"}} {
		if _, err := LogicalClient(row.platform, 0, row.client); err == nil {
			t.Fatal("inconsistent client category accepted")
		}
	}
	s, _, ctx := fixture(t)
	v := session("ios-session-00000001", 1, 1)
	if _, err := s.Exchange(ctx, v, "ios-chat", "ios-im"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Pair(ctx, "ios-chat", "ios-im", v.UserID, 1)
	if err != nil || got.LogicalClient != "ios" {
		t.Fatal("iOS pair failed", err)
	}
	web := session("web-client-000000001", 5, 1)
	web.LogicalClient = "web_mobile"
	if _, err := s.Exchange(ctx, web, "web-chat", "web-im"); err != nil {
		t.Fatal(err)
	}
	web.LogicalClient = "web_desktop"
	if _, err := s.Exchange(ctx, web, "changed-chat", "changed-im"); err == nil {
		t.Fatal("same SID changed logical client")
	}
}

func TestLivePolicyUsesLogicalClientAndRejectsStaleSecurityVersion(t *testing.T) {
	t.Setenv("HRM_SSO_REQUIRED", "true")
	t.Setenv("HRM_BRIDGE_HMAC_KEY", strings.Repeat("k", 32))
	version := int64(1)
	allowed := true
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		raw, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-HRM-Signature") != Sign(strings.Repeat("k", 32), r.Header.Get("X-HRM-Timestamp"), r.Header.Get("X-HRM-Nonce"), "POST", r.URL.Path, raw) {
			t.Error("logical policy body not HMAC bound")
		}
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if len(body) != 7 || body["platform"] != float64(5) || body["logicalClient"] != "web_mobile" || body["entryZone"] != "internet" {
			t.Error("mobile web policy fell back to PC")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"errCode": 0, "data": map[string]any{"allowed": allowed, "securityVersion": version, "policyRevision": 3, "networkRevision": 4}})
	}))
	defer server.Close()
	t.Setenv("HRM_ACCESS_CHECK_URL", server.URL+"/internal/im-access/check")
	s, _, ctx := fixture(t)
	v := session("mobile-web-000000001", 5, 1)
	v.PolicyPlatform = 2
	if _, err := s.Exchange(ctx, v, "chat", "im"); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"connect", "preview", "save", "upload"} {
		if err := CheckSessionAccess(ctx, v, "192.0.2.1", action, ""); err != nil {
			t.Fatal(err)
		}
	}
	version = 2
	if CheckSessionAccess(ctx, v, "192.0.2.1", "preview", "") == nil {
		t.Fatal("SQL security version advance left stale SID usable")
	}
	version = 0
	if CheckSessionAccess(ctx, v, "192.0.2.1", "preview", "") == nil {
		t.Fatal("missing security version failed open")
	}
	version = 1
	allowed = false
	if CheckSessionAccess(ctx, v, "192.0.2.1", "save", "") == nil || calls != 7 {
		t.Fatal("current policy was cached or ignored")
	}
}

func TestResourceProofRequiresCurrentSIDEntryAndEpoch(t *testing.T) {
	s, _, ctx := fixture(t)
	v := session("resource-proof-000001", 5, 1)
	v.LogicalClient = "web_mobile"
	if _, err := s.Exchange(ctx, v, "chat", "im"); err != nil {
		t.Fatal(err)
	}
	const lan = "https://192.168.10.11:9885"
	proof, err := s.IssueResourceProof(ctx, v, lan)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.ValidateResourceProof(ctx, proof, lan); err != nil || got.LogicalClient != "web_mobile" {
		t.Fatal("current proof denied", err)
	}
	for _, input := range []struct{ token, origin string }{{proof, "https://im.szputy.com:9885"}, {strings.ToUpper(proof), lan}, {proof + "x", lan}, {"", lan}} {
		if _, err := s.ValidateResourceProof(ctx, input.token, input.origin); err == nil {
			t.Fatal("forged or cross-entry proof accepted")
		}
	}
	v.SessionID = "resource-replacement-00001"
	if _, err := s.Exchange(ctx, v, "new-chat", "new-im"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ValidateResourceProof(ctx, proof, lan); err == nil {
		t.Fatal("proof survived replaced SID")
	}
	proof, err = s.IssueResourceProof(ctx, v, lan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Revoke(ctx, v.UserID, "", 0, true, 2); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ValidateResourceProof(ctx, proof, lan); err == nil {
		t.Fatal("proof survived epoch barrier")
	}
}
