package identity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func peerFixture(t *testing.T) (*Store, Session, context.Context) {
	t.Helper()
	t.Setenv("HRM_SSO_REQUIRED", "true")
	s, _, ctx := fixture(t)
	if err := s.PutProjection(ctx, Projection{UserID: "emp_peer", Enabled: true, Version: 1}); err != nil {
		t.Fatal(err)
	}
	v := session("session-peer-00000000001", 3, 1)
	if _, err := s.Exchange(ctx, v, "chat-fixture", "im-fixture"); err != nil {
		t.Fatal(err)
	}
	return s, v, WithClientSession(ctx, v, "192.0.2.15")
}

func TestColleagueAuthorizationUsesBoundSessionAndRealTimeHRMPolicy(t *testing.T) {
	s, _, ctx := peerFixture(t)
	t.Setenv("HRM_BRIDGE_HMAC_KEY", strings.Repeat("k", 32))
	requests := 0
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if len(body) != 7 || body["logicalClient"] != "pc" || body["userID"] != "emp_test" || body["resourcePath"] != "emp_peer" || body["action"] != "direct_chat" || body["platform"] != float64(3) || body["clientIP"] != "192.0.2.15" || body["entryZone"] != "internet" {
			t.Error("unbound or widened HRM policy request", body)
		}
		io.WriteString(w, `{"errCode":0,"data":{"allowed":true,"securityVersion":1}}`)
	}))
	defer policy.Close()
	t.Setenv("HRM_ACCESS_CHECK_URL", policy.URL+"/internal/im-access/check")
	if err := s.AuthorizePeer(ctx, "emp_test", "emp_peer"); err != nil || requests != 1 {
		t.Fatal("eligible colleague pair denied", err, requests)
	}
	for _, pair := range [][2]string{{"emp_test", "emp_test"}, {"emp_test", "external-peer"}, {"hrm-im-system", "emp_peer"}, {"emp_peer", "emp_test"}} {
		if err := s.AuthorizePeer(ctx, pair[0], pair[1]); err == nil {
			t.Fatal("unbound, self or nonemployee pair authorized", pair)
		}
	}
	if requests != 1 {
		t.Fatal("invalid local identities reached policy endpoint")
	}
}

func TestColleagueAuthorizationRejectsDisabledMissingAndMalformedProjection(t *testing.T) {
	for _, state := range []Projection{{UserID: "emp_peer", Enabled: false, Version: 2}, {UserID: "emp_peer", Enabled: true, Version: 0}} {
		s, _, ctx := peerFixture(t)
		_ = s.PutProjection(ctx, state)
		if err := s.AuthorizePeer(ctx, "emp_test", "emp_peer"); err == nil {
			t.Fatal("ineligible target authorized")
		}
	}
	s, _, ctx := peerFixture(t)
	if err := s.AuthorizePeer(ctx, "emp_test", "emp_missing"); err == nil {
		t.Fatal("missing target authorized")
	}
	_ = s.Write(ctx, userKey("emp_peer"), Projection{UserID: "emp_other", Enabled: true, Version: 1}, 0)
	if err := s.AuthorizePeer(ctx, "emp_test", "emp_peer"); err == nil {
		t.Fatal("projection identity mismatch authorized")
	}
	_ = s.PutProjection(ctx, Projection{UserID: "emp_test", Enabled: false, Version: 2})
	if err := s.AuthorizePeer(ctx, "emp_test", "emp_peer"); err == nil {
		t.Fatal("disabled sender authorized")
	}
}

func TestColleagueAuthorizationRejectsMissingDuplicateOrInvalidClientFacts(t *testing.T) {
	s, v, _ := peerFixture(t)
	for _, ctx := range []context.Context{context.Background(), WithClientSession(context.Background(), v, "not-an-IP"), context.WithValue(WithClientSession(context.Background(), v, "192.0.2.1"), clientSessionKey, []string{v.SessionID, "extra"})} {
		if err := s.AuthorizePeer(ctx, "emp_test", "emp_peer"); err == nil {
			t.Fatal("forged/missing client facts authorized")
		}
	}
}

func TestColleagueAuthorizationRejectsRevokedAndReplacedSessions(t *testing.T) {
	s, old, ctx := peerFixture(t)
	fresh := old
	fresh.SessionID = "session-peer-replacement-0001"
	fresh.ExpiresAt = time.Now().Add(time.Hour).Unix()
	_, _ = s.Exchange(ctx, fresh, "chat-new", "im-new")
	if err := s.AuthorizePeer(ctx, "emp_test", "emp_peer"); err == nil {
		t.Fatal("replaced SID authorized")
	}
	ctx = WithClientSession(ctx, fresh, "192.0.2.15")
	_, _ = s.Revoke(ctx, fresh.UserID, fresh.SessionID, 0, false, 0)
	if err := s.AuthorizePeer(ctx, "emp_test", "emp_peer"); err == nil {
		t.Fatal("revoked SID authorized")
	}
}

func TestColleagueAuthorizationFailsClosedOnHostAndRegistryOutages(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusServiceUnavailable} {
		s, _, ctx := peerFixture(t)
		policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			io.WriteString(w, `{"errCode":403,"data":{"allowed":false}}`)
		}))
		t.Setenv("HRM_ACCESS_CHECK_URL", policy.URL+"/internal/im-access/check")
		t.Setenv("HRM_BRIDGE_HMAC_KEY", strings.Repeat("k", 32))
		if err := s.AuthorizePeer(ctx, "emp_test", "emp_peer"); err == nil {
			t.Fatal("host denial/outage authorized")
		}
		policy.Close()
		if err := s.AuthorizePeer(ctx, "emp_test", "emp_peer"); err == nil {
			t.Fatal("host network outage authorized")
		}
	}
	s, _, ctx := peerFixture(t)
	_ = s.Redis.Close()
	if err := s.AuthorizePeer(ctx, "emp_test", "emp_peer"); err == nil {
		t.Fatal("registry outage authorized")
	}
}

func TestBoundClientFactsPreserveOtherRPCMetadataWithoutMutatingItsSlice(t *testing.T) {
	keys := []string{"existing"}
	base := context.WithValue(context.Background(), customHeaderKey, keys)
	v := Session{SessionID: "session-peer-fixture-00001"}
	ctx := WithClientSession(WithClientSession(base, v, "192.0.2.1"), v, "192.0.2.2")
	actual := ctx.Value(customHeaderKey).([]string)
	if len(keys) != 1 || len(actual) != 4 || actual[0] != "existing" || ctx.Value(clientAddressKey).([]string)[0] != "192.0.2.2" || EntryZoneFromContext(ctx) != "internet" {
		t.Fatal("metadata overwritten, duplicated or source mutated")
	}
}
