package enterprise

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"openim38.local/identity"
)

type policyFixture struct {
	state       *identity.Store
	v           identity.Session
	server      *Server
	login, save atomic.Bool
	epoch       atomic.Int64
	status      atomic.Int64
}

type pairFixtureAuth struct{ state *identity.Store }

func (a pairFixtureAuth) Authenticate(ctx context.Context, r *http.Request, _ bool) (Actor, error) {
	var binding identity.Binding
	if a.state.Read(ctx, "HRM38:BIND:"+identity.Hash(r.Header.Get("chatToken")), &binding) != nil {
		return Actor{}, unauthorized("invalid pair")
	}
	bound, err := a.state.Live(ctx, binding.SessionID)
	if err != nil {
		return Actor{}, unauthorized("invalid pair")
	}
	v, err := a.state.Pair(ctx, r.Header.Get("chatToken"), r.Header.Get("token"), bound.UserID, bound.Platform)
	if err != nil {
		return Actor{}, unauthorized("invalid pair")
	}
	return Actor{UserID: v.UserID, SessionID: v.SessionID, Platform: v.Platform, LogicalClient: v.LogicalClient, Epoch: v.Epoch, ExpiresAt: v.ExpiresAt}, nil
}

func newPolicyFixture(t *testing.T) *policyFixture {
	t.Helper()
	t.Setenv("HRM_SSO_REQUIRED", "true")
	r := miniredis.RunT(t)
	t.Setenv("HRM_REDIS_URL", "redis://"+r.Addr()+"/2")
	t.Setenv("HRM_BRIDGE_HMAC_KEY", strings.Repeat("k", 32))
	f := &policyFixture{}
	f.login.Store(true)
	f.save.Store(true)
	f.epoch.Store(1)
	f.status.Store(200)
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var q struct {
			Action, LogicalClient string
			Platform              int
		}
		_ = json.NewDecoder(r.Body).Decode(&q)
		if q.Platform != 5 || q.LogicalClient != "web_mobile" {
			t.Error("lost mobile web logical client")
		}
		status := int(f.status.Load())
		if status != 200 {
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"errCode": status})
			return
		}
		allowed := f.login.Load() && (q.Action != "save" || f.save.Load())
		p := identity.EffectivePolicy{Zone: "lan", LogicalClient: "web_mobile", Login: f.login.Load(), Preview: f.login.Load(), Save: f.save.Load(), SecurityVersion: f.epoch.Load(), PolicyRevision: 3, NetworkRevision: 4, CurrentIP: "192.0.2.1"}
		d := identity.AccessDecision{Allowed: allowed, Zone: p.Zone, LogicalClient: p.LogicalClient, LoginAllowed: p.Login, SaveAllowed: p.Save, SecurityVersion: p.SecurityVersion, PolicyRevision: 3, NetworkRevision: 4, EffectivePolicy: p}
		_ = json.NewEncoder(w).Encode(map[string]any{"errCode": 0, "data": d})
	}))
	t.Cleanup(policy.Close)
	t.Setenv("HRM_ACCESS_CHECK_URL", policy.URL+"/internal/im-access/check")
	var err error
	f.state, err = identity.Global()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = f.state.PutProjection(ctx, identity.Projection{UserID: "emp_policy_fixture", Enabled: true, Version: 1}); err != nil {
		t.Fatal(err)
	}
	f.v = identity.Session{UserID: "emp_policy_fixture", SessionID: "sid-policy-fixture-000001", Epoch: 1, Platform: 5, LogicalClient: "web_mobile", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	if _, err = f.state.Exchange(ctx, f.v, "chat", "im"); err != nil {
		t.Fatal(err)
	}
	f.server = &Server{Store: NewMemoryStore(), Auth: pairFixtureAuth{f.state}}
	return f
}

func policyRequest(method, target string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Puty-Entry", "lan")
	r.Header.Set("X-Real-IP", "192.0.2.1")
	return r
}

func TestSessionStateIsReadOnlyLiveDualTokenAndCompletePolicy(t *testing.T) {
	f := newPolicyFixture(t)
	const path = "https://192.168.10.11:9885/enterprise/v1/session/state"
	read := func(method, target, im, chat string) *httptest.ResponseRecorder {
		r := policyRequest(method, target)
		r.Header.Set("token", im)
		r.Header.Set("chatToken", chat)
		w := httptest.NewRecorder()
		f.server.ServeHTTP(w, r)
		return w
	}
	w := read("GET", path, "im", "chat")
	var out struct {
		Data struct {
			UserID, SessionID, LogicalClient string
			Epoch, ExpiresAt                 int64
			LoginAllowed, SaveAllowed        bool
			EffectivePolicy                  identity.EffectivePolicy
		}
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Data.SessionID != f.v.SessionID || out.Data.LogicalClient != "web_mobile" || !out.Data.LoginAllowed || !out.Data.SaveAllowed || out.Data.EffectivePolicy.SecurityVersion != 1 || w.Header().Get("Cache-Control") != "no-store" || len(w.Result().Cookies()) != 0 {
		t.Fatal("state is incomplete or issued credentials")
	}
	for _, field := range []string{"imToken", "chatToken", "resourceProof", "sessionToken"} {
		if strings.Contains(w.Body.String(), `"`+field+`"`) {
			t.Fatal("state exposed credentials")
		}
	}
	f.save.Store(false)
	w = read("GET", path, "im", "chat")
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 200 || out.Data.SaveAllowed || !out.Data.LoginAllowed {
		t.Fatal("save denial incorrectly denied login or used stale policy")
	}
	for _, input := range []struct{ method, path, im, chat string }{{"GET", path, "", ""}, {"GET", path, "im", "wrong"}, {"POST", path, "im", "chat"}, {"GET", path + "?logicalClient=pc", "im", "chat"}} {
		if w = read(input.method, input.path, input.im, input.chat); w.Code == 200 {
			t.Fatal("invalid state request accepted")
		}
	}
	f.epoch.Store(2)
	if w = read("GET", path, "im", "chat"); w.Code != 403 {
		t.Fatal("security version changed without immediate state denial", w.Code)
	}
	f.epoch.Store(1)
	f.status.Store(503)
	if w = read("GET", path, "im", "chat"); w.Code != 503 {
		t.Fatal("dependency outage mistaken for authorized state", w.Code)
	}
	f.status.Store(403)
	if w = read("GET", path, "im", "chat"); w.Code != 403 {
		t.Fatal("explicit denial classification changed", w.Code)
	}
}

func TestAttachmentProofPurposeEntryRefreshAndCurrentPolicy(t *testing.T) {
	f := newPolicyFixture(t)
	var storageCalls atomic.Int64
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		storageCalls.Add(1)
		for _, key := range []string{"token", "chatToken", "Cookie", "Authorization", resourceProofHeader} {
			if r.Header.Get(key) != "" {
				t.Error("business proof reached storage", key)
			}
		}
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
	ctx := context.Background()
	const lan = "https://192.168.10.11:9885"
	const raw = "https://im.szputy.com:9885/openim38/policy-fixture?X-Amz-Signature=fixture"
	preview, err := f.state.WrapAttachmentPurpose(ctx, f.v, lan, raw, false, "preview")
	if err != nil {
		t.Fatal(err)
	}
	save, err := f.state.WrapAttachmentPurpose(ctx, f.v, lan, raw, false, "save")
	if err != nil {
		t.Fatal(err)
	}
	proof, err := f.state.IssueResourceProof(ctx, f.v, lan)
	if err != nil {
		t.Fatal(err)
	}
	read := func(target, proofValue, cookie string, modify func(*http.Request)) *httptest.ResponseRecorder {
		r := policyRequest("GET", target)
		if proofValue != "" {
			r.Header.Set(resourceProofHeader, proofValue)
		}
		if cookie != "" {
			r.AddCookie(&http.Cookie{Name: resourceCookie, Value: cookie})
		}
		if modify != nil {
			modify(r)
		}
		w := httptest.NewRecorder()
		f.server.ServeHTTP(w, r)
		return w
	}
	if w := read(preview, "", "", nil); w.Code != 401 {
		t.Fatal("copied grant was enough", w.Code)
	}
	if w := read(preview, proof, "", nil); w.Code != 200 || w.Header().Get("Content-Disposition") != "inline" {
		t.Fatal("current preview failed", w.Code)
	}
	if w := read(preview, proof, "", func(r *http.Request) { r.Header.Set("token", "im"); r.Header.Set("chatToken", "chat") }); w.Code != 200 {
		t.Fatal("same-SID proof and native pair denied", w.Code)
	}
	if w := read(preview, proof, "", func(r *http.Request) { r.Header.Set("token", "wrong"); r.Header.Set("chatToken", "chat") }); w.Code != 401 {
		t.Fatal("explicit proof overrode an invalid native pair", w.Code)
	}
	if w := read(save, proof, "", nil); w.Code != 200 || w.Header().Get("Content-Disposition") != "attachment" {
		t.Fatal("current save failed", w.Code)
	}
	if w := read(preview, "", proof, func(r *http.Request) { r.Method = "HEAD" }); w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("browser cookie HEAD failed", w.Code)
	}
	if w := read(preview, proof, "", func(r *http.Request) { r.Header.Set("Range", "bytes=1-2") }); w.Code != 206 || w.Body.String() != "bc" {
		t.Fatal("authorized Range changed", w.Code)
	}
	before := storageCalls.Load()
	for _, modify := range []func(*http.Request){func(r *http.Request) { r.Header.Set("X-Puty-Entry", "internet") }, func(r *http.Request) { r.Header.Add(resourceProofHeader, proof) }, func(r *http.Request) { r.RemoteAddr = "203.0.113.1:1234" }} {
		if w := read(preview, proof, "", modify); w.Code == 200 {
			t.Fatal("ambiguous or cross-entry proof accepted")
		}
	}
	if storageCalls.Load() != before {
		t.Fatal("invalid proof reached storage")
	}
	f.save.Store(false)
	if w := read(save, proof, "", nil); w.Code != 403 || storageCalls.Load() != before {
		t.Fatal("old save grant ignored current save denial", w.Code)
	}
	if w := read(preview, proof, "", nil); w.Code != 200 {
		t.Fatal("save denial broke preview", w.Code)
	}
	// Refresh adds tokens to the same SID; the browser proof remains usable.
	if _, err = f.state.Exchange(ctx, f.v, "chat-refreshed", "im-refreshed"); err != nil {
		t.Fatal(err)
	}
	if w := read(preview, proof, "", nil); w.Code != 200 {
		t.Fatal("same SID refresh invalidated proof", w.Code)
	}
	if w := read(preview, "", "stale-cookie", func(r *http.Request) {
		r.Header.Set("token", "im-refreshed")
		r.Header.Set("chatToken", "chat-refreshed")
	}); w.Code != 200 {
		t.Fatal("ambient cookie overrode explicit native pair", w.Code)
	}
	// A different current client session cannot consume the publisher's grant.
	other := f.v
	other.SessionID = "sid-other-client-0000001"
	other.Platform = 2
	other.LogicalClient = "android"
	if _, err = f.state.Exchange(ctx, other, "other-chat", "other-im"); err != nil {
		t.Fatal(err)
	}
	otherProof, err := f.state.IssueResourceProof(ctx, other, lan)
	if err != nil {
		t.Fatal(err)
	}
	if w := read(preview, otherProof, "", nil); w.Code != 401 {
		t.Fatal("another live SID consumed grant", w.Code)
	}
	if w := read(preview, proof, "", func(r *http.Request) { r.Header.Set("token", "other-im"); r.Header.Set("chatToken", "other-chat") }); w.Code != 401 {
		t.Fatal("old proof overrode a different valid native SID", w.Code)
	}
	f.epoch.Store(2)
	if w := read(preview, proof, "", nil); w.Code != 403 {
		t.Fatal("stale SQL security version retained preview", w.Code)
	}
	f.epoch.Store(1)
	f.status.Store(503)
	if w := read(preview, proof, "", nil); w.Code != 503 {
		t.Fatal("dependency outage was mistaken for policy denial", w.Code)
	}
	f.status.Store(200)
	f.login.Store(false)
	if w := read(preview, proof, "", nil); w.Code != 403 {
		t.Fatal("current login denial left preview live", w.Code)
	}
	if _, err = f.state.Revoke(ctx, f.v.UserID, f.v.SessionID, 0, false, 0); err != nil {
		t.Fatal(err)
	}
	if w := read(preview, proof, "", nil); w.Code != 401 {
		t.Fatal("revoked proof consumed grant", w.Code)
	}
}

func TestAttachmentAuthorizeSetsHostOnlyCookieAndRequiresExplicitSavePolicy(t *testing.T) {
	f := newPolicyFixture(t)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"errCode":0,"data":{"url":"https://im.szputy.com:9885/openim38/authorize-fixture?X-Amz-Signature=fixture"}}`)
	}))
	defer api.Close()
	f.server.Upstream = &Upstream{IMBase: api.URL, adminToken: "fixture", tokenExpiry: time.Now().Add(time.Hour)}
	authorize := func(purpose string) *httptest.ResponseRecorder {
		r := policyRequest("POST", "https://192.168.10.11:9885/enterprise/v1/attachment/authorize")
		r.Body = io.NopCloser(strings.NewReader(`{"url":"https://im.szputy.com:9885/api/object/fixture-name","purpose":"` + purpose + `"}`))
		r.Header.Set("token", "im")
		r.Header.Set("chatToken", "chat")
		w := httptest.NewRecorder()
		f.server.ServeHTTP(w, r)
		return w
	}
	w := authorize("preview")
	if w.Code != 200 {
		t.Fatal("preview authorize failed", w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != resourceCookie || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].Domain != "" || cookies[0].Path != "/" || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("resource cookie scope weakened")
	}
	f.save.Store(false)
	if w = authorize("save"); w.Code != 403 || len(w.Result().Cookies()) != 0 {
		t.Fatal("save policy bypass issued proof", w.Code)
	}
	if w = authorize("upload"); w.Code != 400 {
		t.Fatal("read authorize accepted upload purpose", w.Code)
	}
}

func TestAttachmentActiveStreamStopsWithinTenSecondsOnPolicyRevocation(t *testing.T) {
	f := newPolicyFixture(t)
	started := make(chan struct{})
	cancelled := make(chan struct{})
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer storage.Close()
	t.Setenv("OBJECT_STORAGE_UPSTREAM", storage.URL)
	const origin = "https://192.168.10.11:9885"
	wrapped, err := f.state.WrapAttachment(context.Background(), f.v, origin, "https://im.szputy.com:9885/openim38/stream-fixture?X-Amz-Signature=fixture", false)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := f.state.IssueResourceProof(context.Background(), f.v, origin)
	if err != nil {
		t.Fatal(err)
	}
	r := policyRequest("GET", wrapped)
	r.Header.Set(resourceProofHeader, proof)
	done := make(chan struct{})
	go func() { defer close(done); f.server.ServeHTTP(httptest.NewRecorder(), r) }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not reach storage")
	}
	f.save.Store(false) // A preview stays live when only Save is revoked.
	f.login.Store(false)
	select {
	case <-cancelled:
	case <-time.After(9 * time.Second):
		t.Fatal("active stream retained revoked login beyond ten seconds")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("proxy retained cancelled transfer")
	}
}

func TestAttachmentProxyPreservesValidatorsAndConditionalRangeSemantics(t *testing.T) {
	f := newPolicyFixture(t)
	const etag = `"immutable-fixture-v2"`
	const modified = "Wed, 04 Nov 2026 01:00:00 GMT"
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", modified)
		w.Header().Set("Accept-Ranges", "bytes")
		if r.Header.Get("Range") == "bytes=2-3" && (r.Header.Get("If-Range") == etag || r.Header.Get("If-Range") == modified) {
			w.Header().Set("Content-Range", "bytes 2-3/4")
			w.Header().Set("Content-Length", "2")
			w.WriteHeader(206)
			_, _ = io.WriteString(w, "cd")
			return
		}
		w.Header().Set("Content-Length", "4")
		_, _ = io.WriteString(w, "abcd")
	}))
	defer storage.Close()
	t.Setenv("OBJECT_STORAGE_UPSTREAM", storage.URL)
	ctx := context.Background()
	const origin = "https://192.168.10.11:9885"
	wrapped, err := f.state.WrapAttachment(ctx, f.v, origin, "https://im.szputy.com:9885/openim38/validator-fixture?X-Amz-Signature=fixture", false)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := f.state.IssueResourceProof(ctx, f.v, origin)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		method, validator, body, contentRange string
		status                                int
	}{{"GET", etag, "cd", "bytes 2-3/4", 206}, {"GET", modified, "cd", "bytes 2-3/4", 206}, {"GET", `"old-v1"`, "abcd", "", 200}, {"HEAD", etag, "", "bytes 2-3/4", 206}} {
		r := policyRequest(row.method, wrapped)
		r.Header.Set(resourceProofHeader, proof)
		r.Header.Set("Range", "bytes=2-3")
		r.Header.Set("If-Range", row.validator)
		w := httptest.NewRecorder()
		f.server.ServeHTTP(w, r)
		if w.Code != row.status || w.Body.String() != row.body || w.Header().Get("ETag") != etag || w.Header().Get("Last-Modified") != modified || w.Header().Get("Accept-Ranges") != "bytes" || w.Header().Get("Content-Range") != row.contentRange {
			t.Fatal("upstream range/validator semantics changed", row.method, row.status, w.Code)
		}
	}
}
