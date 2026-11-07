package imbridge

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAccessDefaultAndIndependentInternetDownloadMobilePermissions(t *testing.T) {
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	assert := func(platform int, ip, action string, allowed bool) {
		t.Helper()
		d, e := store.CheckAccess(ctx, 1, platform, ip, action)
		if (e == nil && d.Allowed) != allowed {
			t.Fatalf("platform %d ip %s action %s allowed=%v error=%v", platform, ip, action, d.Allowed, e)
		}
	}
	assert(3, "192.168.10.50", "connect", true)
	assert(5, "192.168.14.12", "download", true)
	assert(2, "192.168.10.50", "connect", false)
	assert(3, "192.168.11.50", "connect", false) // private but untrusted VLAN
	assert(3, "203.0.113.8", "connect", false)
	assert(7, "192.168.10.50", "connect", false)
	assert(3, "invalid", "connect", false)
	assert(3, "192.168.10.50", "unknown", false)
	svc := &Service{Store: store, Remote: remote, Cache: newFakeCache()}
	p, e := svc.SaveEmployeeAccess(ctx, EmployeeAccess{EmployeeID: "a", AllowLAN: true, AllowPC: true, AllowInternet: true}, 0, 99)
	must(t, e)
	assert(3, "203.0.113.8", "connect", true)
	assert(3, "203.0.113.8", "upload", true)
	assert(3, "203.0.113.8", "download", false)
	assert(2, "203.0.113.8", "connect", false)
	p.AllowMobile, p.AllowInternetDownload = true, true
	p, e = svc.SaveEmployeeAccess(ctx, p, p.Revision, 99)
	must(t, e)
	assert(2, "203.0.113.8", "download", true)
	assert(2, "192.168.10.50", "connect", true)
	p.AllowInternet, p.AllowInternetDownload = false, true
	if _, e = svc.SaveEmployeeAccess(ctx, p, p.Revision, 99); !errors.Is(e, ErrInvalid) {
		t.Fatal("public download granted without Internet access")
	}
	_, e = store.DB.ExecContext(ctx, `UPDATE biz_hrm_employee SET employee_no='NEW-NUMBER' WHERE id='a'`)
	must(t, e)
	must(t, store.Reconcile(ctx))
	must(t, store.Drain(ctx, remote))
	assert(2, "203.0.113.8", "download", true)
}

func TestAccessTighteningAtomicallyRevokesAndAuditsWithoutDeletingIdentity(t *testing.T) {
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	svc := &Service{Store: store, Remote: remote, Cache: newFakeCache()}
	login, e := svc.Login(ctx, 1, "windows")
	must(t, e)
	policy, e := svc.SaveEmployeeAccess(ctx, EmployeeAccess{EmployeeID: "a", AllowLAN: true, AllowPC: false}, 0, 99)
	must(t, e)
	if _, e = svc.Validate(ctx, login.SessionToken); e == nil {
		t.Fatal("old session survived platform disable")
	}
	if _, e = store.Eligible(ctx, 1); e != nil {
		t.Fatal("access setting removed employee identity", e)
	}
	if len(remote.revokes) != 1 || !remote.revokes[0].All {
		t.Fatal("tightening did not notify token and websocket revocation")
	}
	var n int
	must(t, store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM biz_openim_outbox WHERE kind='revoke' AND status='pending'`).Scan(&n))
	if n != 1 {
		t.Fatal("revocation not durable")
	}
	if _, e = svc.SaveEmployeeAccess(ctx, policy, 0, 99); !errors.Is(e, ErrConflict) {
		t.Fatal("stale policy edit overwrote newer value")
	}
	must(t, store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM biz_openim_access_audit WHERE kind='employee' AND actor_uid=99 AND client_ip='192.168.10.10'`).Scan(&n))
	if n != 1 {
		t.Fatal("conflicting write changed audit or missing committed audit")
	}
	policy.EmployeeID = "missing"
	if _, e = svc.SaveEmployeeAccess(ctx, policy, 0, 99); !errors.Is(e, ErrInvalid) {
		t.Fatal("missing employee accepted")
	}
	if _, e = svc.Login(context.Background(), 1, "web"); e == nil {
		t.Fatal("missing verified caller IP granted access")
	}
}

func TestAccessNetworkChangeRechecksExistingSessionAndTicketExchange(t *testing.T) {
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	svc := &Service{Store: store, Remote: remote, Cache: newFakeCache()}
	login, e := svc.Login(ctx, 1, "web")
	must(t, e)
	ticket, e := svc.IssueTicket(ctx, 1)
	must(t, e)
	public := WithClientIP(ctx, "203.0.113.8")
	if _, e = svc.Exchange(public, ticket, "web"); e == nil {
		t.Fatal("LAN issued ticket bypassed public policy")
	}
	policy, e := store.NetworkPolicy(ctx)
	must(t, e)
	policy, e = store.SaveNetworkPolicy(ctx, []string{"192.168.14.12/24", "192.168.14.0/24"}, policy.Revision, 99)
	must(t, e)
	if len(policy.CIDRs) != 1 || policy.CIDRs[0] != "192.168.14.0/24" || policy.CurrentZone != "internet" {
		t.Fatal("network normalization/classification failed")
	}
	if _, e = svc.Validate(ctx, login.SessionToken); e == nil {
		t.Fatal("old session not checked against new network")
	}
	if _, e = svc.Refresh(ctx, login.SessionToken); e == nil {
		t.Fatal("refresh bypassed changed network")
	}
	if _, e = store.SaveNetworkPolicy(ctx, []string{"192.168.10.0/24"}, policy.Revision-1, 99); !errors.Is(e, ErrConflict) {
		t.Fatal("stale network update succeeded")
	}
	for _, invalid := range [][]string{nil, {"0.0.0.0/0"}, {"::/0"}, {"127.0.0.0/8"}, {"224.0.0.0/4"}, {"192.168.0.0/15"}, {"bad"}} {
		if _, e = store.SaveNetworkPolicy(ctx, invalid, policy.Revision, 99); !errors.Is(e, ErrInvalid) {
			t.Fatal("unsafe network accepted", invalid)
		}
	}
}

func TestAccessUsesOnlyVerifiedProxyHeader(t *testing.T) {
	for _, tc := range []struct{ peer, dedicated, forwarded, want string }{
		{"203.0.113.8:1234", "192.168.10.1", "192.168.10.1", "203.0.113.8"},
		{"127.0.0.1:1234", "203.0.113.8", "192.168.10.1", "203.0.113.8"},
		{"127.0.0.1:1234", "192.168.10.1,203.0.113.8", "192.168.10.1", "127.0.0.1"},
		{"127.0.0.1:1234", "", "192.168.10.1", "127.0.0.1"},
		{"[::ffff:203.0.113.8]:1234", "192.168.10.1", "", "203.0.113.8"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.peer
		if tc.dedicated != "" {
			r.Header.Set("X-GoSuxin-Client-IP", tc.dedicated)
		}
		r.Header.Set("X-Forwarded-For", tc.forwarded)
		if got := requestClientIP(r); got != tc.want {
			t.Errorf("peer %s produced %s want %s", tc.peer, got, tc.want)
		}
	}
}

func signedAccessRequest(t *testing.T, key string, body []byte, nonce string, now time.Time) *http.Request {
	t.Helper()
	r := httptest.NewRequest("POST", "/internal/im-access/check", strings.NewReader(string(body)))
	r.RemoteAddr = "127.0.0.1:3333"
	ts := strconv.FormatInt(now.Unix(), 10)
	sum := sha256.Sum256(body)
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(ts + "\n" + nonce + "\nPOST\n/internal/im-access/check\n" + hex.EncodeToString(sum[:])))
	r.Header.Set("X-HRM-Timestamp", ts)
	r.Header.Set("X-HRM-Nonce", nonce)
	r.Header.Set("X-HRM-Signature", hex.EncodeToString(mac.Sum(nil)))
	return r
}

func TestAccessInternalHMACFreshPolicyReplayAndCallerChecks(t *testing.T) {
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	now := time.Now()
	svc := &Service{Store: store, Remote: remote, Cache: newFakeCache(), Now: func() time.Time { return now }}
	key := strings.Repeat("a", 48)
	handler := svc.internalAccessHandler(key, &accessHMACVerifier{})
	body, _ := json.Marshal(AccessCheckRequest{UserID: UserID("a"), Platform: 3, ClientIP: "192.168.10.50", Action: "download"})
	request := func(nonce string) *http.Request {
		return signedAccessRequest(t, key, body, strings.Repeat(nonce, 64), now)
	}
	w := httptest.NewRecorder()
	handler(w, request("1"))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"allowed":true`) {
		t.Fatal("signed LAN check failed", w.Code)
	}
	w = httptest.NewRecorder()
	handler(w, request("1"))
	if w.Code != 401 {
		t.Fatal("HMAC nonce replay accepted")
	}
	r := request("2")
	r.RemoteAddr = "203.0.113.8:32100"
	r.Header.Set("X-GoSuxin-Client-IP", "127.0.0.1")
	w = httptest.NewRecorder()
	handler(w, r)
	if w.Code != 403 {
		t.Fatal("untrusted transport accepted")
	}
	r = request("3")
	r.Body = http.NoBody
	w = httptest.NewRecorder()
	handler(w, r)
	if w.Code != 401 {
		t.Fatal("changed signed body accepted")
	}
	_, e := svc.SaveEmployeeAccess(ctx, EmployeeAccess{EmployeeID: "a", AllowLAN: false, AllowPC: true}, 0, 99)
	must(t, e)
	w = httptest.NewRecorder()
	handler(w, request("4"))
	if w.Code != 403 {
		t.Fatal("fresh DB restriction not applied")
	}
	r = signedAccessRequest(t, key, body, strings.Repeat("5", 64), now.Add(-61*time.Second))
	w = httptest.NewRecorder()
	handler(w, r)
	if w.Code != 401 {
		t.Fatal("stale HMAC accepted")
	}
}

func TestLANCheckOnlyAllowsCurrentNetworksAndLoopbackProxy(t *testing.T) {
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	svc := &Service{Store: store, Remote: remote}
	for _, tc := range []struct {
		peer, ip string
		status   int
	}{{"127.0.0.1:1", "192.168.10.99", 204}, {"127.0.0.1:1", "192.168.11.99", 403}, {"127.0.0.1:1", "203.0.113.8", 403}, {"203.0.113.8:1", "192.168.10.99", 403}} {
		r := httptest.NewRequest("GET", "/internal/im-access/lan-check", nil)
		r.RemoteAddr = tc.peer
		r.Header.Set("X-GoSuxin-Client-IP", tc.ip)
		w := httptest.NewRecorder()
		svc.lanCheckHandler(w, r)
		if w.Code != tc.status {
			t.Errorf("got %d want %d", w.Code, tc.status)
		}
	}
}
