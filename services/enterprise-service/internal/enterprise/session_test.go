package enterprise

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type revokerFixture struct {
	tokens       []string
	failures     int
	chatTokens   []string
	chatFailures int
}

func (r *revokerFixture) RevokeChatToken(_ context.Context, user, token string) error {
	r.chatTokens = append(r.chatTokens, user+":"+token)
	if r.chatFailures > 0 {
		r.chatFailures--
		return &apiError{503, 1006, "injected Chat outage"}
	}
	return nil
}

func (r *revokerFixture) RevokeIMToken(_ context.Context, token string) error {
	r.tokens = append(r.tokens, token)
	if r.failures > 0 {
		r.failures--
		return &apiError{503, 1006, "injected upstream outage"}
	}
	return nil
}

func TestExplicitLogoutRevokesExactSessionAndIsIdempotent(t *testing.T) {
	s, _, _, _ := fixture(t)
	s.Auth = &SessionAuth{Inner: s.Auth, Store: s.Store}
	revoker := &revokerFixture{}
	s.SessionRevoker = revoker
	if w := post(s, "alice", "/enterprise/v1/office/tag/find/user", map[string]any{}); w.Code != 200 {
		t.Fatalf("active session rejected: %s", w.Body.String())
	}
	w := post(s, "alice", "/enterprise/v1/session/logout", map[string]any{})
	if data := mustData(t, w); data["revoked"] != true {
		t.Fatal("logout did not complete")
	}
	if w := post(s, "alice", "/enterprise/v1/office/tag/find/user", map[string]any{}); w.Code != 401 {
		t.Fatal("logged-out pair still accepted")
	}
	// Same IM credential with a different Chat token is equally unusable.
	r := httptest.NewRequest("POST", "/enterprise/v1/office/tag/find/user", strings.NewReader(`{}`))
	r.Header.Set("token", "im-alice")
	r.Header.Set("chatToken", "other-chat")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("new Chat token reused a logged-out IM credential")
	}
	// A new same-platform login is not the target of the older logout request.
	r = httptest.NewRequest("POST", "/enterprise/v1/office/tag/find/user", strings.NewReader(`{}`))
	r.Header.Set("token", "new-im-alice")
	r.Header.Set("chatToken", "alice")
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal("new same-platform session was revoked")
	}
	mustData(t, post(s, "alice", "/enterprise/v1/session/logout", map[string]any{}))
	if len(revoker.tokens) != 1 || revoker.tokens[0] != "im-alice" {
		t.Fatalf("logout was not exact and idempotent (calls %d)", len(revoker.tokens))
	}
	if len(revoker.chatTokens) != 1 || revoker.chatTokens[0] != "alice:alice" {
		t.Fatal("Chat logout was not exact and idempotent")
	}
	if w := post(s, "", "/enterprise/v1/session/logout", map[string]any{}); w.Code != 401 {
		t.Fatal("unauthenticated logout accepted")
	}
}

func TestChatLogoutFailureRetriesAfterRestartAndUpgradesOldCompleteRecords(t *testing.T) {
	s, _, _, _ := fixture(t)
	s.Auth = &SessionAuth{Inner: s.Auth, Store: s.Store}
	revoker := &revokerFixture{chatFailures: 1}
	s.SessionRevoker = revoker
	if w := post(s, "alice", "/enterprise/v1/session/logout", map[string]any{}); w.Code != 503 {
		t.Fatal("Chat failure reported as successful logout")
	}
	if w := post(s, "alice", "/enterprise/v1/office/tag/find/user", map[string]any{}); w.Code != 401 {
		t.Fatal("Chat outage bypassed durable barrier")
	}
	// Simulate a record completed by the previous IM-only implementation.
	key := revokedSessionKey("im-alice")
	if _, err := mutate[sessionRevocation](context.Background(), s.Store, key, func(v *sessionRevocation) error { v.Complete = true; return nil }); err != nil {
		t.Fatal(err)
	}
	s.lastRevocationCheck = time.Time{}
	if err := s.retrySessionRevocations(context.Background()); err != nil {
		t.Fatal(err)
	}
	v, _, err := load[sessionRevocation](context.Background(), s.Store, key)
	if err != nil || !v.ChatComplete || !v.Complete || len(revoker.chatTokens) != 2 {
		t.Fatal("legacy completed record failed to finish exact Chat revocation")
	}
	mustData(t, post(s, "alice", "/enterprise/v1/session/logout", map[string]any{}))
	if len(revoker.chatTokens) != 2 {
		t.Fatal("completed pair was revoked again")
	}
}

func TestLogoutOutageKeepsDurableBarrierAndRetriesAcrossRestart(t *testing.T) {
	s, _, media, _ := fixture(t)
	inner := s.Auth
	s.Auth = &SessionAuth{Inner: inner, Store: s.Store}
	revoker := &revokerFixture{failures: 1}
	s.SessionRevoker = revoker
	if w := post(s, "alice", "/enterprise/v1/session/logout", map[string]any{}); w.Code != 503 {
		t.Fatalf("expected upstream outage: %d %s", w.Code, w.Body.String())
	}
	if w := post(s, "alice", "/enterprise/v1/office/tag/find/user", map[string]any{}); w.Code != 401 {
		t.Fatal("partially revoked session bypassed durable barrier")
	}
	s2 := &Server{Store: s.Store, Auth: &SessionAuth{Inner: inner, Store: s.Store}, SessionRevoker: revoker, IM: s.IM, Media: media, Cipher: s.Cipher, Now: s.Now}
	if err := s2.retrySessionRevocations(context.Background()); err != nil {
		t.Fatal(err)
	}
	mustData(t, post(s2, "alice", "/enterprise/v1/session/logout", map[string]any{}))
	if len(revoker.tokens) != 2 {
		t.Fatal("background retry did not finish the interrupted revocation")
	}
	// Possessing a different pair cannot retrieve or complete someone else's
	// record, even when the caller supplies the already revoked IM credential.
	r := httptest.NewRequest(http.MethodPost, "/enterprise/v1/session/logout", strings.NewReader(`{}`))
	r.Header.Set("token", "im-alice")
	r.Header.Set("chatToken", "bob")
	w := httptest.NewRecorder()
	s2.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("different pair authorized an idempotent logout")
	}
}

func TestExplicitLogoutClosesCurrentMediaAndRejectsOldRoomGrant(t *testing.T) {
	s, media, _, _, room := activeMediaFixture(t, "meeting", 3)
	s.Auth = &SessionAuth{Inner: s.Auth, Store: s.Store}
	s.SessionRevoker = &revokerFixture{}
	oldToken, err := media.Token(room.MediaRoom, "bob", nil, room.CurrentLease["bob"], "")
	if err != nil {
		t.Fatal(err)
	}
	mustData(t, post(s, "bob", "/enterprise/v1/session/logout", map[string]any{}))
	current, _, err := load[Room](context.Background(), s.Store, "room:"+room.RoomID)
	if err != nil || contains(current.Joined, "bob") || !contains(current.Joined, "alice") || media.removed == 0 {
		t.Fatal("logout did not remove only the current media participant")
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/livekit/rtc?access_token="+oldToken, nil)
	s.ServeHTTP(w, r)
	if w.Code != 401 && w.Code != 403 {
		t.Fatalf("old media grant reconnected after logout: %d", w.Code)
	}
}
