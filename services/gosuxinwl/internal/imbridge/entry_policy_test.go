package imbridge

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/internal/accountsecurity"
)

// Model the trusted proxy in existing HTTP fixtures. The production server
// never derives this marker from a caller's private-looking address alone.
func setTestTrustedEntry(r *http.Request, ip string) {
	r.Header.Set("X-GoSuxin-Client-IP", ip)
	r.Host = "hr.szputy.com:9880"
	r.Header.Set("X-Puty-Access-Zone", "internet")
	if address, err := netip.ParseAddr(ip); err == nil && address.IsPrivate() {
		r.Host = "192.168.10.11:9880"
		r.Header.Set("X-Puty-Access-Zone", "lan")
	}
}

func entryRequest(peer, source, host string, values ...string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/sso/policy?zone=lan&entryZone=lan", nil)
	r.RemoteAddr = peer
	r.Host = host
	if source != "" {
		r.Header.Set("X-GoSuxin-Client-IP", source)
	}
	for _, value := range values {
		r.Header.Add("X-Puty-Access-Zone", value)
	}
	// None of these public inputs may supply the policy zone or audit address.
	r.Header.Set("X-Forwarded-Host", "192.168.10.11:9885")
	r.Header.Set("X-Forwarded-For", "192.168.10.5")
	r.Header.Set("Forwarded", "for=192.168.10.5;host=192.168.10.11:9885")
	r.Header.Set("X-Puty-Entry", "lan")
	return r
}

func TestHTTPAccessZoneUsesTrustedEntryInsteadOfSourceCIDR(t *testing.T) {
	ctx, store, _ := fixture(t)
	for _, tc := range []struct {
		name, peer, source, host, want string
		headers                        []string
	}{
		{"routed internal VLAN via HR IP", "127.0.0.1:3400", "192.168.14.8", "192.168.10.11:9880", "lan", []string{"lan"}},
		{"routed internal VLAN via IM IP", "127.0.0.1:3400", "192.168.14.8", "192.168.10.11:9885", "lan", []string{"lan"}},
		{"same VLAN via HR domain", "127.0.0.1:3400", "192.168.14.8", "hr.szputy.com:9880", "internet", []string{"internet"}},
		{"same VLAN via IM domain", "127.0.0.1:3400", "192.168.14.8", "im.szputy.com:9885", "internet", []string{"internet"}},
		{"domain cannot claim LAN", "127.0.0.1:3400", "192.168.10.5", "im.szputy.com:9885", "internet", []string{"lan"}},
		{"public source cannot claim IP LAN", "127.0.0.1:3400", "203.0.113.8", "192.168.10.11:9885", "internet", []string{"lan"}},
		{"untrusted peer with forged internal address", "203.0.113.8:3400", "192.168.10.5", "192.168.10.11:9885", "internet", []string{"lan"}},
		{"direct internal peer cannot self classify", "192.168.14.8:3400", "192.168.10.5", "192.168.10.11:9885", "internet", []string{"lan"}},
		{"missing header never falls back to CIDR", "127.0.0.1:3400", "192.168.10.5", "192.168.10.11:9885", "internet", nil},
		{"duplicate header", "127.0.0.1:3400", "192.168.10.5", "192.168.10.11:9885", "internet", []string{"lan", "lan"}},
		{"conflicting header", "127.0.0.1:3400", "192.168.10.5", "192.168.10.11:9885", "internet", []string{"lan", "internet"}},
		{"combined header", "127.0.0.1:3400", "192.168.10.5", "192.168.10.11:9885", "internet", []string{"lan,internet"}},
		{"unknown header", "127.0.0.1:3400", "192.168.10.5", "192.168.10.11:9885", "internet", []string{"internal"}},
		{"unknown host", "127.0.0.1:3400", "192.168.10.5", "evil.invalid", "internet", []string{"lan"}},
		{"wrong port", "127.0.0.1:3400", "192.168.10.5", "192.168.10.11:443", "internet", []string{"lan"}},
		{"missing original address", "127.0.0.1:3400", "", "192.168.10.11:9885", "internet", []string{"lan"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := entryRequest(tc.peer, tc.source, tc.host, tc.headers...)
			value, err := store.NetworkPolicy(requestAccessContext(ctx, r))
			must(t, err)
			if value.CurrentZone != tc.want {
				t.Fatalf("got zone %q, want %q", value.CurrentZone, tc.want)
			}
			if tc.name == "routed internal VLAN via HR IP" && value.CurrentIP != "192.168.14.8" {
				t.Fatal("entry classification rewrote the real audit IP")
			}
		})
	}
	r := entryRequest("127.0.0.1:3400", "192.168.10.5", "192.168.10.11:9885", "lan")
	r.Header.Add("X-GoSuxin-Client-IP", "192.168.14.8")
	if requestEntryZone(r) != "internet" {
		t.Fatal("ambiguous original source granted LAN")
	}
}

func restrictFixtureToInternalDesktop(t *testing.T, ctx context.Context, store *SQLStore) {
	t.Helper()
	matrix := accountsecurity.DefaultMatrix("deny")
	for _, client := range []string{"pc", "web_desktop"} {
		matrix["lan"][client] = accountsecurity.Cell{Login: "allow", Save: "allow"}
	}
	_, err := store.security().SavePolicy(ctx, accountsecurity.Policy{Subject: "person", SubjectID: 1, Matrix: matrix}, 99)
	must(t, err)
}

func TestTrustedEntryPreservesPersonalTerminalRulesAndSSORechecks(t *testing.T) {
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	restrictFixtureToInternalDesktop(t, ctx, store)
	svc := &Service{Store: store, Remote: remote, Cache: newFakeCache()}
	lan := requestAccessContext(ctx, entryRequest("127.0.0.1:3400", "192.168.14.8", "192.168.10.11:9885", "lan"))
	wan := requestAccessContext(ctx, entryRequest("127.0.0.1:3400", "192.168.14.8", "im.szputy.com:9885", "internet"))
	for _, client := range []struct {
		name     string
		platform int
	}{{"pc", 3}, {"web_desktop", 5}} {
		for _, action := range []string{"connect", "preview", "save"} {
			decision, err := store.CheckAccessLogical(lan, 1, client.platform, "192.168.14.8", action, client.name)
			must(t, err)
			if !decision.Allowed || decision.Zone != "lan" || decision.EffectivePolicy.CurrentIP != "192.168.14.8" {
				t.Fatal("routed IP entry did not preserve LAN and bare audit IP")
			}
			if _, err = store.CheckAccessLogical(wan, 1, client.platform, "192.168.14.8", action, client.name); !errors.Is(err, ErrDenied) {
				t.Fatal("same source using domain bypassed personal Internet denial")
			}
		}
	}
	if _, err := store.CheckAccessLogical(lan, 1, 2, "192.168.14.8", "connect", "android"); !errors.Is(err, ErrDenied) {
		t.Fatal("LAN entry weakened the employee's Android terminal denial")
	}
	login, err := svc.Login(lan, 1, "windows")
	must(t, err)
	_, err = svc.Validate(lan, login.SessionToken)
	must(t, err)
	_, err = svc.Refresh(lan, login.SessionToken)
	must(t, err)
	if _, err = svc.Validate(wan, login.SessionToken); err == nil {
		t.Fatal("live session validation ignored the current domain entry")
	}
	if _, err = svc.Refresh(wan, login.SessionToken); err == nil {
		t.Fatal("refresh adopted the original LAN policy at a domain entry")
	}
	ticket, err := svc.IssueTicket(lan, 1)
	must(t, err)
	if _, err = svc.Exchange(wan, ticket, "web"); err == nil {
		t.Fatal("LAN SSO ticket was exchanged through forbidden Internet entry")
	}
	ticket, err = svc.IssueTicket(lan, 1)
	must(t, err)
	_, err = svc.Exchange(lan, ticket, "web")
	must(t, err)
}

func TestSignedAccessEntryZoneIsBoundToHMACAndMissingDefaultsToInternet(t *testing.T) {
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	restrictFixtureToInternalDesktop(t, ctx, store)
	now := time.Now()
	svc := &Service{Store: store, Remote: remote, Cache: newFakeCache(), Now: func() time.Time { return now }}
	key := strings.Repeat("a", 48)
	handler := svc.internalAccessHandler(key, &accessHMACVerifier{})
	for i, tc := range []struct {
		name, zone, ip string
		want           int
	}{
		{"routed internal entry", "lan", "192.168.14.8", 200},
		{"domain from internal source", "internet", "192.168.14.8", 403},
		{"legacy producer missing entry", "", "192.168.10.8", 403},
		{"invalid zone", "lan,internet", "192.168.14.8", 403},
		{"public source cannot claim LAN", "lan", "203.0.113.8", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(AccessCheckRequest{UserID: UserID("a"), Platform: 3, LogicalClient: "pc", ClientIP: tc.ip, Action: "connect", EntryZone: tc.zone})
			must(t, err)
			r := signedAccessRequest(t, key, body, strings.Repeat(string(rune('a'+i)), 64), now)
			r.Header.Set("X-Puty-Access-Zone", "lan") // Never overrides signed body.
			w := httptest.NewRecorder()
			handler(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d, want %d", w.Code, tc.want)
			}
			if tc.want == 200 && (!strings.Contains(w.Body.String(), `"zone":"lan"`) || !strings.Contains(w.Body.String(), `"currentIP":"192.168.14.8"`)) {
				t.Fatal("signed entry decision lost the zone or real source")
			}
		})
	}
	body, err := json.Marshal(AccessCheckRequest{UserID: UserID("a"), Platform: 3, LogicalClient: "pc", ClientIP: "192.168.14.8", Action: "connect", EntryZone: "internet"})
	must(t, err)
	r := signedAccessRequest(t, key, body, strings.Repeat("f", 64), now)
	r.Body = http.NoBody
	w := httptest.NewRecorder()
	handler(w, r)
	if w.Code != 401 {
		t.Fatal("changed signed entry body bypassed HMAC verification")
	}
}
