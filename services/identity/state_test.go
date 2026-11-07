package identity

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func fixture(t *testing.T) (*Store, *miniredis.Miniredis, context.Context) {
	t.Helper()
	r := miniredis.RunT(t)
	db := redis.NewClient(&redis.Options{Addr: r.Addr()})
	t.Cleanup(func() { _ = db.Close() })
	s := &Store{Redis: db}
	ctx := context.Background()
	if e := s.PutProjection(ctx, Projection{UserID: "emp_test", Enabled: true, Version: 1}); e != nil {
		t.Fatal(e)
	}
	return s, r, ctx
}
func session(id string, p int, epoch int64) Session {
	return Session{UserID: "emp_test", SessionID: id, Platform: p, Epoch: epoch, ExpiresAt: time.Now().Add(time.Hour).Unix()}
}
func TestRefreshKeepsPreviousGenerationAndBindsBothPlatforms(t *testing.T) {
	s, _, ctx := fixture(t)
	v := session("session-00000000001", 3, 1)
	if _, e := s.Exchange(ctx, v, "chat-old", "im-old"); e != nil {
		t.Fatal(e)
	}
	v.ExpiresAt += 3600
	if _, e := s.Exchange(ctx, v, "chat-new", "im-new"); e != nil {
		t.Fatal(e)
	}
	for _, pair := range [][2]string{{"chat-old", "im-old"}, {"chat-new", "im-new"}} {
		if _, e := s.Pair(ctx, pair[0], pair[1], v.UserID, 3); e != nil {
			t.Fatal(e)
		}
	}
	mobile := session("session-00000000002", 2, 1)
	if _, e := s.Exchange(ctx, mobile, "chat-mobile", "im-mobile"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Pair(ctx, "chat-new", "im-mobile", v.UserID, 0); e == nil {
		t.Fatal("mixed SID/platform tokens accepted")
	}
	s.Now = func() time.Time { return time.Now().Add(time.Hour + time.Minute) }
	if _, e := s.Validate(ctx, "im-old", v.UserID, "im", 3); e == nil {
		t.Fatal("old generation expiry extended by refresh")
	}
	if _, e := s.Validate(ctx, "im-new", v.UserID, "im", 3); e != nil {
		t.Fatal(e)
	}
}
func TestDelayedRevocationCannotKickReplacementAndEpochBarrier(t *testing.T) {
	s, _, ctx := fixture(t)
	old := session("session-00000000001", 3, 1)
	_, _ = s.Exchange(ctx, old, "chat-old", "im-old")
	newer := session("session-00000000002", 3, 2)
	ids, e := s.Exchange(ctx, newer, "chat-new", "im-new")
	if e != nil || len(ids) != 1 {
		t.Fatalf("replacement: %v %v", ids, e)
	}
	if _, e = s.Live(ctx, old.SessionID); e == nil {
		t.Fatal("replaced SID survived")
	}
	for _, platform := range []int{0, old.Platform} {
		if _, e = s.Revoke(ctx, old.UserID, old.SessionID, platform, false, 0); e != nil {
			t.Fatal(e)
		}
		if _, e = s.Live(ctx, newer.SessionID); e != nil {
			t.Fatal("old SID revoke affected new SID", e)
		}
	}
	if _, e = s.Revoke(ctx, old.UserID, "", 0, true, 2); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Live(ctx, newer.SessionID); e != nil {
		t.Fatal("delayed barrier revoked subsequent epoch", e)
	}
	if _, e = s.Exchange(ctx, session("session-00000000003", 5, 1), "stale-chat", "stale-im"); e == nil {
		t.Fatal("stale epoch login accepted")
	}
	if _, e = s.Revoke(ctx, old.UserID, "", 0, true, 3); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Live(ctx, newer.SessionID); e == nil {
		t.Fatal("older epoch survived barrier")
	}
}

func TestExactSIDWithPlatformNeverSweepsReplacementAtSameEpoch(t *testing.T) {
	s, _, ctx := fixture(t)
	old := session("old-same-epoch-000001", 3, 1)
	_, _ = s.Exchange(ctx, old, "chat-old", "im-old")
	fresh := session("new-same-epoch-000001", 3, 1)
	if _, err := s.Exchange(ctx, fresh, "chat-fresh", "im-fresh"); err != nil {
		t.Fatal(err)
	}
	for _, sid := range []string{old.SessionID, "not-yet-seen-SID-00001"} {
		ids, err := s.Revoke(ctx, old.UserID, sid, old.Platform, false, 0)
		if err != nil || len(ids) != 1 || ids[0] != sid {
			t.Fatal("exact revocation expanded its target", err)
		}
		if _, err = s.Pair(ctx, "chat-fresh", "im-fresh", fresh.UserID, fresh.Platform); err != nil {
			t.Fatal("delayed SID+platform revoke invalidated a replacement", err)
		}
		late := old
		late.SessionID = sid
		if _, err = s.Exchange(ctx, late, "late-chat", "late-im"); err == nil {
			t.Fatal("exact revocation failed to retain a tombstone")
		}
	}
}

func TestPlatformOnlyRevocationStillTargetsThatPlatform(t *testing.T) {
	s, _, ctx := fixture(t)
	pc := session("platform-pc-session-001", 3, 1)
	mobile := session("platform-mobile-session-001", 2, 1)
	_, _ = s.Exchange(ctx, pc, "chat-pc", "im-pc")
	_, _ = s.Exchange(ctx, mobile, "chat-mobile", "im-mobile")
	if _, err := s.Revoke(ctx, pc.UserID, "", pc.Platform, false, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Live(ctx, pc.SessionID); err == nil {
		t.Fatal("platform-only revocation left its target live")
	}
	if _, err := s.Live(ctx, mobile.SessionID); err != nil {
		t.Fatal("platform-only revocation swept another platform", err)
	}
}
func TestDisableAndRedisLossFailClosed(t *testing.T) {
	s, r, ctx := fixture(t)
	v := session("session-00000000001", 3, 1)
	_, _ = s.Exchange(ctx, v, "chat", "im")
	if e := s.PutProjection(ctx, Projection{UserID: v.UserID, Enabled: false, Version: 2}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Live(ctx, v.SessionID); e == nil {
		t.Fatal("disabled employee survived")
	}
	_ = s.PutProjection(ctx, Projection{UserID: v.UserID, Enabled: true, Version: 3})
	r.FlushAll()
	_ = s.PutProjection(ctx, Projection{UserID: v.UserID, Enabled: true, Version: 3})
	if _, e := s.Validate(ctx, "im", v.UserID, "im", 3); e == nil {
		t.Fatal("projection repair resurrected missing sessions")
	}
	r.Close()
	if _, e := s.Live(ctx, v.SessionID); e == nil {
		t.Fatal("registry outage accepted")
	}
}
func TestAttachmentPreservesSignatureAndRevokesWithSID(t *testing.T) {
	s, _, ctx := fixture(t)
	v := session("session-00000000001", 3, 1)
	_, _ = s.Exchange(ctx, v, "chat", "im")
	raw := "https://im.szputy.com:9885/openim38/hash%20object?X-Amz-Signature=literal%2Bsignature&partNumber=2"
	wrapped, e := s.WrapAttachment(ctx, v, "https://192.168.10.11:9885", raw, true)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.HasSuffix(wrapped, "/openim38/hash%20object?X-Amz-Signature=literal%2Bsignature&partNumber=2") {
		t.Fatal("signed path/query changed", wrapped)
	}
	token := strings.Split(strings.TrimPrefix(wrapped, "https://192.168.10.11:9885/attachments/"), "/")[0]
	grant, e := s.Attachment(ctx, token)
	if e != nil || grant.SourceHost != "im.szputy.com:9885" {
		t.Fatalf("source Host changed: %+v %v", grant, e)
	}
	_, _ = s.Revoke(ctx, v.UserID, v.SessionID, 0, false, 0)
	if _, e = s.Attachment(ctx, token); e == nil {
		t.Fatal("signed URL survived logout")
	}
}

func TestRevocationBeforeExchangeAndCorruptRegistryFailClosed(t *testing.T) {
	s, _, ctx := fixture(t)
	v := session("session-before-revoke-01", 3, 1)
	if _, err := s.Revoke(ctx, v.UserID, v.SessionID, 0, false, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Exchange(ctx, v, "late-chat", "late-im"); err == nil {
		t.Fatal("delayed exchange resurrected revoked SID")
	}
	v.SessionID = "session-corrupt-data-01"
	if err := s.Redis.Set(ctx, sessionKey(v.SessionID), "not-json", time.Hour).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Exchange(ctx, v, "chat", "im"); err == nil {
		t.Fatal("corrupt identity state treated as an absent session")
	}
	if _, err := s.WrapAttachment(ctx, v, "https://192.168.10.11:9885", "https://im.szputy.com:9885/openim38", false); err == nil {
		t.Fatal("read grant authorized whole bucket")
	}
	if _, err := s.WrapAttachment(ctx, v, "https://192.168.10.11:9885", "https://im.szputy.com:9885/openim38", true); err != nil {
		t.Fatal("signed form POST URL could not be wrapped", err)
	}
}
func TestTrustedEntryAndPathTraversal(t *testing.T) {
	t.Setenv("TRUSTED_ENTRY_PROXY_CIDRS", "172.30.38.1/32")
	r := httptest.NewRequest("GET", "https://example.invalid/", nil)
	r.RemoteAddr = "192.0.2.10:1234"
	r.Header.Set("X-Puty-Entry", "lan")
	r.Header.Set("X-GoSuxin-Client-IP", "127.0.0.1")
	if EntryOrigin(r) != "" {
		t.Fatal("untrusted source selected entry")
	}
	if ip, _ := ClientIP(r); ip != "192.0.2.10" {
		t.Fatal("spoofed source accepted")
	}
	r.RemoteAddr = "172.30.38.1:1234"
	if EntryOrigin(r) != "https://192.168.10.11:9885" {
		t.Fatal("trusted LAN entry rejected")
	}
	r.Header.Add("X-GoSuxin-Client-IP", "192.168.10.5")
	if _, e := ClientIP(r); e == nil {
		t.Fatal("duplicate source assertion accepted")
	}
	for _, path := range []string{"/resource/uploads/../secret", "/resource/uploads/%2e%2e/secret", "/resource/uploads/a\\b", "/other/uploads/file"} {
		if SafePath(path, "/resource/uploads/") {
			t.Fatal("unsafe path accepted", path)
		}
	}
}
func TestHMACBindsMethodPathAndExactBody(t *testing.T) {
	a := Sign("secret", "1700000000", "nonce", "POST", "/internal/hrm/exchange", []byte(`{"userID":"emp_1"}`))
	for _, b := range []string{Sign("secret", "1700000000", "nonce", "GET", "/internal/hrm/exchange", []byte(`{"userID":"emp_1"}`)), Sign("secret", "1700000000", "nonce", "POST", "/internal/hrm/revoke", []byte(`{"userID":"emp_1"}`)), Sign("secret", "1700000000", "nonce", "POST", "/internal/hrm/exchange", []byte(`{"userID":"emp_2"}`))} {
		if a == b {
			t.Fatal("unbound HMAC field")
		}
	}
}
