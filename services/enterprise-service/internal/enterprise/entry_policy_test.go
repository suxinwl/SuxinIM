package enterprise

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"openim38.local/identity"
)

func TestPrivateBridgePropagatesOnlySignedEntryZoneAndDefaultsLegacyPublic(t *testing.T) {
	f := newPolicyFixture(t)
	key := strings.Repeat("k", 32)
	var zones []string
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-HRM-Signature") != identity.Sign(key, r.Header.Get("X-HRM-Timestamp"), r.Header.Get("X-HRM-Nonce"), r.Method, r.URL.Path, raw) {
			t.Error("forwarded access body was not signed")
		}
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		if len(body) != 7 || body["clientIP"] != "192.168.14.8" {
			t.Error("private forwarding changed actual IP or fields")
		}
		zones = append(zones, body["entryZone"].(string))
		_ = json.NewEncoder(w).Encode(map[string]any{"errCode": 0, "data": identity.AccessDecision{Allowed: true}})
	}))
	defer policy.Close()
	t.Setenv("HRM_ACCESS_CHECK_URL", policy.URL+"/internal/im-access/check")
	bridge := &HRMBridge{State: f.state, Key: key}
	for i, row := range []struct {
		zone   any
		omit   bool
		tamper bool
		status int
	}{{"lan", false, false, 200}, {"internet", false, false, 200}, {nil, true, false, 200}, {"lan,internet", false, false, 400}, {[]string{"lan"}, false, false, 400}, {"internet", false, true, 401}} {
		body := map[string]any{"userID": "emp_fixture", "platform": 3, "logicalClient": "pc", "clientIP": "192.168.14.8", "action": "connect", "resourcePath": ""}
		if !row.omit {
			body["entryZone"] = row.zone
		}
		raw, _ := json.Marshal(body)
		stamp, nonce, path := strconv.FormatInt(time.Now().Unix(), 10), strings.Repeat("n", 30)+strconv.Itoa(i), "/internal/hrm/access/check"
		signature := identity.Sign(key, stamp, nonce, "POST", path, raw)
		if row.tamper {
			body["entryZone"] = "lan"
			raw, _ = json.Marshal(body)
		}
		r := httptest.NewRequest("POST", path, bytes.NewReader(raw))
		r.Header.Set("X-HRM-Timestamp", stamp)
		r.Header.Set("X-HRM-Nonce", nonce)
		r.Header.Set("X-HRM-Signature", signature)
		// Private access facts come from the verified body, not HTTP headers.
		r.Header.Set("X-Puty-Access-Zone", "lan")
		w := httptest.NewRecorder()
		bridge.ServeHTTP(w, r)
		if w.Code != row.status {
			t.Fatalf("private zone case %d status %d, want %d", i, w.Code, row.status)
		}
	}
	if strings.Join(zones, ",") != "lan,internet,internet" {
		t.Fatal("unsigned/invalid zone reached policy or legacy body gained LAN", zones)
	}
}

func TestPublicSessionAndAttachmentUseEntryZoneWithUnchangedSourceIP(t *testing.T) {
	f := newPolicyFixture(t)
	var zones []string
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-HRM-Signature") != identity.Sign(strings.Repeat("k", 32), r.Header.Get("X-HRM-Timestamp"), r.Header.Get("X-HRM-Nonce"), r.Method, r.URL.Path, raw) {
			t.Error("entry policy was not signed")
		}
		var body struct{ EntryZone, ClientIP, LogicalClient string }
		_ = json.Unmarshal(raw, &body)
		zones = append(zones, body.EntryZone)
		if body.ClientIP != "192.168.14.8" {
			t.Error("classification replaced actual client address", body.ClientIP)
		}
		allowed := body.EntryZone == "lan"
		p := identity.EffectivePolicy{Zone: body.EntryZone, LogicalClient: body.LogicalClient, Login: allowed, Preview: allowed, Save: allowed, SecurityVersion: 1, PolicyRevision: 3, NetworkRevision: 4, CurrentIP: body.ClientIP}
		d := identity.AccessDecision{Allowed: allowed, Zone: p.Zone, LogicalClient: p.LogicalClient, LoginAllowed: allowed, SaveAllowed: allowed, SecurityVersion: 1, PolicyRevision: 3, NetworkRevision: 4, EffectivePolicy: p}
		_ = json.NewEncoder(w).Encode(map[string]any{"errCode": 0, "data": d})
	}))
	defer policy.Close()
	t.Setenv("HRM_ACCESS_CHECK_URL", policy.URL+"/internal/im-access/check")
	const lan = "https://192.168.10.11:9885"
	read := func(method, target, entry, zone string, resource bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, nil)
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("X-Real-IP", "192.168.14.8")
		r.Header.Set("X-Puty-Entry", entry)
		if zone != "" {
			r.Header.Set("X-Puty-Access-Zone", zone)
		}
		r.Header.Set("token", "im")
		r.Header.Set("chatToken", "chat")
		if resource {
			proof, err := f.state.IssueResourceProof(r.Context(), f.v, lan)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set(resourceProofHeader, proof)
			if method == "GET" {
				r.Header.Set("Range", "bytes=1-2")
			}
		}
		w := httptest.NewRecorder()
		f.server.ServeHTTP(w, r)
		return w
	}
	for _, row := range []struct {
		origin, entry, zone string
		status              int
	}{
		{lan, "lan", "lan", 200},
		{"https://im.szputy.com:9885", "internet", "internet", 403},
		{lan, "lan", "", 403},
		{"https://im.szputy.com:9885", "lan", "lan", 403},
	} {
		if w := read("GET", row.origin+"/enterprise/v1/session/state", row.entry, row.zone, false); w.Code != row.status {
			t.Fatalf("session state entry %q/%q: %d", row.entry, row.zone, w.Code)
		}
	}
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			w.Header().Set("Content-Range", "bytes 1-2/4")
			w.WriteHeader(206)
			_, _ = io.WriteString(w, "bc")
			return
		}
		_, _ = io.WriteString(w, "abcd")
	}))
	defer storage.Close()
	t.Setenv("OBJECT_STORAGE_UPSTREAM", storage.URL)
	wrapped, err := f.state.WrapAttachmentPurpose(t.Context(), f.v, lan, "https://im.szputy.com:9885/openim38/entry-test?X-Amz-Signature=fixture", false, "preview")
	if err != nil {
		t.Fatal(err)
	}
	if w := read("GET", wrapped, "lan", "lan", true); w.Code != 206 || w.Body.String() != "bc" {
		t.Fatal("authorized entry Range failed", w.Code)
	}
	if w := read("HEAD", wrapped, "lan", "lan", true); w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("authorized entry HEAD failed", w.Code)
	}
	if w := read("GET", wrapped, "lan", "internet", true); w.Code != 403 {
		t.Fatal("existing LAN grant ignored current public policy", w.Code)
	}
	if strings.Join(zones, ",") != "lan,internet,internet,internet,lan,lan,internet" {
		t.Fatal("HTTP/resource chain lost its ingress context", zones)
	}
}
