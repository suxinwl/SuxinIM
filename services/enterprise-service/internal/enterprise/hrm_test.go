package enterprise

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gorilla/websocket"
	"github.com/livekit/protocol/livekit"
	"github.com/redis/go-redis/v9"
	"google.golang.org/protobuf/proto"
	"openim38.local/identity"
)

func TestHRMOnlineManagementNeverReturnsConnectionCredentials(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/get_users_online_status" {
			t.Errorf("unexpected upstream route %s", r.URL.Path)
		}
		io.WriteString(w, `{"errCode":0,"data":[{"userID":"emp_fixture","status":1,"detailPlatformStatus":[{"platformID":5,"isBackground":false,"token":"private-live-token","connID":"private-connection","futureSecret":"never-public"}]}]}`)
	}))
	defer upstream.Close()
	b := &HRMBridge{Server: &Server{Upstream: &Upstream{IMBase: upstream.URL, adminToken: "private-admin", tokenExpiry: time.Now().Add(time.Hour)}}}
	value, err := b.manage(context.Background(), "users/online", asRequest(map[string]any{"userIDs": []string{"emp_fixture"}}))
	if err != nil {
		t.Fatal(err)
	}
	raw := string(mustJSON(value))
	for _, secret := range []string{"token", "connID", "futureSecret", "private-", "never-public"} {
		if strings.Contains(raw, secret) {
			t.Fatal("connection credential crossed the management boundary")
		}
	}
	if !strings.Contains(raw, `"platformID":5`) || !strings.Contains(raw, `"userID":"emp_fixture"`) {
		t.Fatal("public online status was lost")
	}
}

func TestHRMPrivateSignatureReplayAndPublicManagementBoundary(t *testing.T) {
	r := miniredis.RunT(t)
	db := redis.NewClient(&redis.Options{Addr: r.Addr()})
	defer db.Close()
	key := strings.Repeat("s", 32)
	b := &HRMBridge{State: &identity.Store{Redis: db}, Key: key}
	body := []byte(`{}`)
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	path := "/internal/hrm/unknown"
	nonce := strings.Repeat("n", 32)
	request := func(method, path, signature string) *httptest.ResponseRecorder {
		q := httptest.NewRequest(method, path, bytes.NewReader(body))
		q.Header.Set("X-HRM-Timestamp", stamp)
		q.Header.Set("X-HRM-Nonce", nonce)
		q.Header.Set("X-HRM-Signature", signature)
		w := httptest.NewRecorder()
		b.ServeHTTP(w, q)
		return w
	}
	good := identity.Sign(key, stamp, nonce, "POST", path, body)
	if w := request("POST", path, good); w.Code != 404 {
		t.Fatalf("signed request failed: %d %s", w.Code, w.Body.String())
	}
	if w := request("POST", path, good); w.Code != 401 {
		t.Fatal("nonce replay accepted")
	}
	if w := request("POST", "/internal/hrm/revoke", good); w.Code != 401 {
		t.Fatal("path substitution accepted")
	}
	if w := request("GET", path, good); w.Code != 403 {
		t.Fatal("GET mutation accepted")
	}
	t.Setenv("HRM_SSO_REQUIRED", "true")
	s := &Server{}
	for _, path := range []string{"/admin/login", "/admin/chat/account/login", "/admin/im/auth/get_user_token"} {
		w := httptest.NewRecorder()
		if !s.adminRoutes(w, httptest.NewRequest("POST", path, nil), path) || w.Code != 403 {
			t.Fatal("old public management credential entry survived", path)
		}
	}
}
func TestHRMICERewritesOnlyTypedTURNAddresses(t *testing.T) {
	if rewriteICEURL("turns:external.example.com:25349?transport=tcp", "192.168.10.11") != "turns:external.example.com:25349?transport=tcp" {
		t.Fatal("unrelated TURN endpoint rewritten")
	}
	response := &livekit.SignalResponse{Message: &livekit.SignalResponse_Join{Join: &livekit.JoinResponse{IceServers: []*livekit.ICEServer{{Urls: []string{"turn:192.168.10.11:23478?transport=udp", "turns:im.szputy.com:25349?transport=tcp", "stun:stun.example.com:3478"}, Username: "dynamic-user", Credential: "dynamic-secret"}}}}}
	raw, _ := proto.Marshal(response)
	out := rewriteICESignal(websocket.BinaryMessage, raw, "192.168.10.11")
	var actual livekit.SignalResponse
	if e := proto.Unmarshal(out, &actual); e != nil {
		t.Fatal(e)
	}
	servers := actual.GetJoin().IceServers
	if servers[0].Urls[1] != "turns:192.168.10.11:25349?transport=tcp" || servers[0].Urls[2] != "stun:stun.example.com:3478" || servers[0].Credential != "dynamic-secret" || servers[0].Username != "dynamic-user" {
		t.Fatal("ICE scope/credentials changed", servers)
	}
	if got := rewriteICESignal(websocket.TextMessage, []byte(`{"other":"turns:im.szputy.com:25349"}`), "192.168.10.11"); string(got) != `{"other":"turns:im.szputy.com:25349"}` {
		t.Fatal("untyped text modified")
	}
}
func TestHRMAttachmentProxyPreservesSignedHostPathRangeAndRejectsLogout(t *testing.T) {
	t.Setenv("HRM_SSO_REQUIRED", "true")
	r := miniredis.RunT(t)
	t.Setenv("HRM_REDIS_URL", "redis://"+r.Addr()+"/2")
	t.Setenv("HRM_BRIDGE_HMAC_KEY", strings.Repeat("k", 32))
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"errCode":0,"data":{"allowed":true,"securityVersion":1}}`)
	}))
	defer policy.Close()
	t.Setenv("HRM_ACCESS_CHECK_URL", policy.URL+"/internal/im-access/check")
	var observedHost, observedURI, observedRange string
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observedHost = r.Host
		observedURI = r.RequestURI
		observedRange = r.Header.Get("Range")
		w.Header().Set("Content-Range", "bytes 2-3/4")
		w.WriteHeader(206)
		io.WriteString(w, "cd")
	}))
	defer storage.Close()
	t.Setenv("OBJECT_STORAGE_UPSTREAM", storage.URL)
	state, e := identity.Global()
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	_ = state.PutProjection(ctx, identity.Projection{UserID: "emp_fixture", Enabled: true, Version: 1})
	v := identity.Session{UserID: "emp_fixture", SessionID: "sid-fixture-000000001", Platform: 3, Epoch: 1, ExpiresAt: time.Now().Add(time.Hour).Unix()}
	if _, e = state.Exchange(ctx, v, "chat", "im"); e != nil {
		t.Fatal(e)
	}
	raw := "https://im.szputy.com:9885/openim38/hash%20object?X-Amz-Signature=a%2Bb&partNumber=2"
	wrapped, e := state.WrapAttachment(ctx, v, "https://192.168.10.11:9885", raw, false)
	if e != nil {
		t.Fatal(e)
	}
	s := &Server{Store: NewMemoryStore()}
	proof, e := state.IssueResourceProof(ctx, v, "https://192.168.10.11:9885")
	if e != nil {
		t.Fatal(e)
	}
	read := func(method string) *http.Request {
		q := httptest.NewRequest(method, wrapped, nil)
		q.RemoteAddr = "127.0.0.1:1234"
		q.Header.Set("X-Puty-Entry", "lan")
		q.Header.Set("X-Real-IP", "192.0.2.1")
		q.Header.Set(resourceProofHeader, proof)
		return q
	}
	q := read("GET")
	q.Header.Set("Range", "bytes=2-3")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, q)
	if w.Code != 206 || w.Body.String() != "cd" || observedHost != "im.szputy.com:9885" || observedURI != "/openim38/hash%20object?X-Amz-Signature=a%2Bb&partNumber=2" || observedRange != "bytes=2-3" {
		t.Fatalf("signature/Range changed: %d %s %s %s", w.Code, observedHost, observedURI, observedRange)
	}
	w = httptest.NewRecorder()
	s.ServeHTTP(w, read("HEAD"))
	if w.Code != 206 || w.Body.Len() != 0 || w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("HEAD body or cache boundary changed", w.Code, w.Body.String())
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"errCode": 0, "data": map[string]string{"url": raw}})
	}))
	defer api.Close()
	s.Upstream = &Upstream{IMBase: api.URL, adminToken: "fixture-admin", tokenExpiry: time.Now().Add(time.Hour)}
	entry := httptest.NewRequest("POST", "https://192.168.10.11:9885/enterprise/v1/attachment/authorize", nil)
	entry.RemoteAddr = "127.0.0.1:1234"
	entry.Header.Set("X-Puty-Entry", "lan")
	entry.Header.Set("X-Real-IP", "192.0.2.1")
	first, err := s.attachmentAuthorize(ctx, Actor{SessionID: v.SessionID}, asRequest(map[string]any{"url": "https://im.szputy.com:9885/api/object/stable-name"}), entry)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = state.Revoke(ctx, v.UserID, v.SessionID, 0, false, 0)
	w = httptest.NewRecorder()
	s.ServeHTTP(w, read("HEAD"))
	if w.Code != 401 {
		t.Fatal("issued attachment survived logout", w.Code)
	}
	v.SessionID = "sid-replacement-000000001"
	if _, err = state.Exchange(ctx, v, "new-chat", "new-im"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OBJECT_STORAGE_ACCESS_KEY_ID", "fixture-access")
	t.Setenv("OBJECT_STORAGE_SECRET_ACCESS_KEY", "fixture-signing-secret")
	previous := first.(map[string]any)
	next, err := s.attachmentAuthorize(ctx, Actor{SessionID: v.SessionID}, asRequest(map[string]any{"url": previous["url"]}), entry)
	if err != nil || next.(map[string]any)["objectID"] != previous["objectID"] {
		t.Fatal("reauthorizing a revoked SID's grant changed stable objectID", err)
	}
}
func TestHRMManagementRejectsUnknownFiltersAndSensitiveConfig(t *testing.T) {
	b := &HRMBridge{}
	if _, e := b.revoke(context.Background(), asRequest(map[string]any{"userID": "emp_fixture", "sessionID": "fixture-session-0001", "platform": 3, "all": true})); e == nil {
		t.Fatal("ambiguous SID and all-session revocation accepted")
	}
	if _, e := b.manage(context.Background(), "messages/search", asRequest(map[string]any{"unsupported": true})); e == nil {
		t.Fatal("unknown search filter silently ignored")
	}
	if _, e := b.manage(context.Background(), "config/update", asRequest(map[string]any{"config": map[string]string{"secret": "must-not-save"}})); e == nil {
		t.Fatal("server secret config allowed")
	}
	if _, e := b.manage(context.Background(), "groups/dismiss", asRequest(map[string]any{"groupID": "dept_managed"})); e == nil {
		t.Fatal("HRM department dismissed outside reconcile")
	}
}
func TestHRMStableObjectResigningNeedsNoPriorGrant(t *testing.T) {
	t.Setenv("OBJECT_STORAGE_ACCESS_KEY_ID", "test-access")
	t.Setenv("OBJECT_STORAGE_SECRET_ACCESS_KEY", "test-secret-with-enough-length")
	s := &Server{}
	raw, e := s.presignObject(context.Background(), "https://192.168.10.11:9885", "/openim38/object%20name")
	if e != nil {
		t.Fatal(e)
	}
	if !strings.HasPrefix(raw, "https://192.168.10.11:9885/openim38/object%20name?") || !strings.Contains(raw, "X-Amz-Signature=") {
		t.Fatal("wrong resigned entry", raw)
	}
	for _, p := range []string{"/openim38/../secret", "/other/object", "/openim38/a%5Cb"} {
		if _, e = s.presignObject(context.Background(), "https://192.168.10.11:9885", p); e == nil {
			t.Fatal("unsafe signing path accepted", p)
		}
	}
}
func TestHRMDiscoveryIsDurableAndValidatesURLs(t *testing.T) {
	b := &HRMBridge{Server: &Server{Store: NewMemoryStore()}}
	ctx := context.Background()
	q := asRequest(map[string]any{"items": []any{map[string]any{"id": "fixture", "title": "Workbench", "url": "https://example.com/", "enabled": true, "sort": 1}}})
	if _, e := b.manage(ctx, "discovery/save", q); e != nil {
		t.Fatal(e)
	}
	value, e := b.manage(ctx, "discovery/list", request{})
	if e != nil {
		t.Fatal(e)
	}
	encoded, _ := json.Marshal(value)
	if !strings.Contains(string(encoded), "Workbench") {
		t.Fatal("discovery not persisted")
	}
	if _, e = b.manage(ctx, "discovery/save", asRequest(map[string]any{"items": []any{map[string]any{"title": "Unsafe", "url": "javascript:alert(1)"}}})); e == nil {
		t.Fatal("script discovery URL accepted")
	}
}

func TestHRMUpdateURLUsesTrustedEntryAndKeepsArtifactImmutable(t *testing.T) {
	t.Setenv("TRUSTED_ENTRY_PROXY_CIDRS", "172.30.38.1/32")
	s, _, _, _ := fixture(t)
	wan := "https://im.szputy.com:9885/desktop/Puty%20IM.exe?x=literal%2Bquery"
	ctx := context.Background()
	if _, err := s.publish(ctx, Actor{UserID: "hrm-bridge", Admin: true}, asRequest(map[string]any{"platform": "windows", "version": "2.0.0", "build": 20, "url": wan, "sha256": strings.Repeat("ab", 32), "size": 1234})); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ peer, profile, want string }{
		{"172.30.38.1:1234", "lan", "https://192.168.10.11:9885/desktop/Puty%20IM.exe?x=literal%2Bquery"},
		{"172.30.38.1:1234", "internet", wan},
		{"192.0.2.5:1234", "lan", wan},
	} {
		q := httptest.NewRequest("POST", "/enterprise/v1/application/latest", strings.NewReader(`{"platform":"windows","version":"1.0.0","build":1}`))
		q.RemoteAddr = tc.peer
		q.Header.Set("X-Puty-Entry", tc.profile)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, q)
		data := mustData(t, w)
		if data["url"] != tc.want {
			t.Fatal("wrong trusted update entry", tc.peer, data["url"])
		}
	}
	artifact, _, err := load[Artifact](ctx, s.Store, artifactKey("windows", "2.0.0"))
	if err != nil || artifact.URL != wan {
		t.Fatal("response projection changed immutable artifact metadata", err)
	}
	third := "https://updates.example.com/pkg.exe?x=1"
	if artifactEntryURL(third, "https://192.168.10.11:9885") != third {
		t.Fatal("third-party update source rewritten")
	}
}
