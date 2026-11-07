package enterprise

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"openim38.local/identity"
)

type mediaEntryPolicyCall struct {
	EntryZone, ClientIP, LogicalClient, Action string
	Platform                                   int
}
type mediaEntryFixture struct {
	server *Server
	media  *testMedia
	actor  Actor
	room   Room
	allow  atomic.Bool
	mu     sync.Mutex
	calls  []mediaEntryPolicyCall
}

func newMediaEntryFixture(t *testing.T, sessionType int) *mediaEntryFixture {
	t.Helper()
	f := newPolicyFixture(t)
	v := f.v
	v.SessionID = "sid-media-entry-android-fixture"
	v.Platform, v.LogicalClient = 2, "android"
	if _, err := f.state.Exchange(t.Context(), v, "media-chat", "media-im"); err != nil {
		t.Fatal(err)
	}
	s, im, media, _ := fixture(t)
	x := &mediaEntryFixture{server: s, media: media, actor: Actor{UserID: v.UserID, SessionID: v.SessionID, Platform: v.Platform, LogicalClient: v.LogicalClient, Epoch: v.Epoch, ExpiresAt: v.ExpiresAt, IP: "192.168.14.8", IMToken: "media-im", ChatToken: "media-chat", EntryOrigin: "https://192.168.10.11:9885"}}
	x.allow.Store(true)
	im.members["media-entry-group"] = []User{{UserID: v.UserID}, {UserID: "fixture-peer"}}
	x.room = Room{RoomID: "media-entry-room", MediaRoom: "media-entry-transport", Kind: "call", State: "accepted", Joined: []string{v.UserID, "fixture-peer"}, CurrentLease: map[string]string{}, ParticipantState: map[string]string{v.UserID: "joined", "fixture-peer": "joined"}, Invitation: Invitation{InviterUserID: v.UserID, InviteeUserIDList: []string{"fixture-peer"}, SessionType: sessionType, MediaType: "audio"}}
	if sessionType == 3 {
		x.room.Invitation.GroupID = "media-entry-group"
	}
	if err := s.Store.Save(t.Context(), document("room:"+x.room.RoomID, "room", v.UserID, x.room), 0); err != nil {
		t.Fatal(err)
	}
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-HRM-Signature") != identity.Sign(strings.Repeat("k", 32), r.Header.Get("X-HRM-Timestamp"), r.Header.Get("X-HRM-Nonce"), r.Method, r.URL.Path, raw) {
			t.Error("media access body lost its HMAC")
		}
		var q mediaEntryPolicyCall
		if json.Unmarshal(raw, &q) != nil {
			t.Error("invalid access body")
		}
		x.mu.Lock()
		x.calls = append(x.calls, q)
		x.mu.Unlock()
		allowed := x.allow.Load() && q.EntryZone == "lan"
		p := identity.EffectivePolicy{Zone: q.EntryZone, LogicalClient: q.LogicalClient, Login: allowed, Preview: allowed, Save: allowed, SecurityVersion: v.Epoch, PolicyRevision: 3, NetworkRevision: 4, CurrentIP: q.ClientIP}
		d := identity.AccessDecision{Allowed: allowed, Zone: p.Zone, LogicalClient: p.LogicalClient, LoginAllowed: allowed, SaveAllowed: allowed, SecurityVersion: v.Epoch, EffectivePolicy: p}
		_ = json.NewEncoder(w).Encode(map[string]any{"errCode": 0, "data": d})
	}))
	t.Cleanup(policy.Close)
	t.Setenv("HRM_ACCESS_CHECK_URL", policy.URL+"/internal/im-access/check")
	return x
}

func (x *mediaEntryFixture) issue(t *testing.T, ctx context.Context, a Actor) (string, Document, mediaGrant, Actor) {
	t.Helper()
	value, err := x.server.issueMediaToken(ctx, a, x.room)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := x.server.Store.List(t.Context(), "media_grant", "")
	if err != nil || len(docs) != 1 {
		t.Fatal("missing persisted media grant", err)
	}
	var grant mediaGrant
	if err = json.Unmarshal(docs[0].Data, &grant); err != nil {
		t.Fatal(err)
	}
	sealed, err := x.server.Cipher.open(grant.Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	return value.(map[string]any)["token"].(string), docs[0], grant, sealed
}

// Construct exact legacy JSON without the new field, rather than relying on
// a zero-valued field to represent a genuinely old encrypted Actor payload.
func mediaActorZone(t *testing.T, a Actor, value any, omit bool) Actor {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(mustJSON(a), &body); err != nil {
		t.Fatal(err)
	}
	delete(body, "EntryZone")
	if !omit {
		body["EntryZone"] = value
	}
	var out Actor
	if err := json.Unmarshal(mustJSON(body), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMediaEntryZoneSealsOnlyIssuanceContext(t *testing.T) {
	for _, tc := range []struct{ name, contextZone, actorZone, want string }{{"lan", "lan", "internet", "lan"}, {"domain", "internet", "lan", "internet"}, {"missing", "", "lan", "internet"}} {
		t.Run(tc.name, func(t *testing.T) {
			x := newMediaEntryFixture(t, 1)
			ctx := context.Background()
			if tc.contextZone != "" {
				ctx = identity.WithEntryZone(ctx, tc.contextZone)
			}
			_, _, _, sealed := x.issue(t, ctx, mediaActorZone(t, x.actor, tc.actorZone, false))
			var body map[string]any
			_ = json.Unmarshal(mustJSON(sealed), &body)
			if body["EntryZone"] != tc.want || sealed.IP != "192.168.14.8" {
				t.Fatalf("untrusted actor field overrode issuance context: zone=%v IP=%q", body["EntryZone"], sealed.IP)
			}
		})
	}
}

func TestMediaEntryZoneSurvivesRestartAndStillRechecksPolicy(t *testing.T) {
	for _, sessionType := range []int{1, 3} {
		t.Run(map[int]string{1: "single", 3: "group"}[sessionType], func(t *testing.T) {
			x := newMediaEntryFixture(t, sessionType)
			x.issue(t, identity.WithEntryZone(context.Background(), "lan"), x.actor)
			// New Server, same durable grant and cipher: no original request remains.
			x.server = &Server{Store: x.server.Store, Auth: x.server.Auth, IM: x.server.IM, Media: x.server.Media, Cipher: x.server.Cipher, Now: x.server.Now}
			for i := 0; i < 2; i++ {
				if err := x.server.revalidateMediaSessions(identity.WithEntryZone(context.Background(), "internet")); err != nil {
					t.Fatal(err)
				}
			}
			room, _, err := load[Room](t.Context(), x.server.Store, "room:"+x.room.RoomID)
			if err != nil || room.ended() || !contains(room.Joined, x.actor.UserID) || x.media.removed != 0 {
				t.Fatal("LAN call revoked during background sweep", err)
			}
			x.mu.Lock()
			calls := append([]mediaEntryPolicyCall(nil), x.calls...)
			x.mu.Unlock()
			if len(calls) != 2 {
				t.Fatal("background checks were skipped", len(calls))
			}
			for _, call := range calls {
				if call.EntryZone != "lan" || call.ClientIP != "192.168.14.8" || call.LogicalClient != "android" || call.Platform != 2 || call.Action != "connect" {
					t.Fatalf("sealed context lost or changed account metadata: %+v", call)
				}
			}
			x.allow.Store(false)
			if err := x.server.revalidateMediaSessions(context.Background()); err != nil {
				t.Fatal(err)
			}
			room, _, err = load[Room](t.Context(), x.server.Store, "room:"+x.room.RoomID)
			if err != nil || contains(room.Joined, x.actor.UserID) || x.media.removed != 1 {
				t.Fatal("persisted LAN entry bypassed current revocation", err)
			}
		})
	}
}

func TestMediaEntryZoneLegacyAndInvalidStayPublic(t *testing.T) {
	for _, tc := range []struct {
		name, zone string
		omit       bool
	}{{"legacy", "", true}, {"invalid", "lan,internet", false}, {"public", "internet", false}} {
		t.Run(tc.name, func(t *testing.T) {
			x := newMediaEntryFixture(t, 1)
			_, doc, grant, sealed := x.issue(t, identity.WithEntryZone(context.Background(), "lan"), x.actor)
			var err error
			grant.Ciphertext, err = x.server.Cipher.seal(mediaActorZone(t, sealed, tc.zone, tc.omit))
			if err != nil {
				t.Fatal(err)
			}
			doc.Data = mustJSON(grant)
			if err = x.server.Store.Save(t.Context(), doc, doc.Version); err != nil {
				t.Fatal(err)
			}
			if err = x.server.revalidateMediaSessions(identity.WithEntryZone(context.Background(), "lan")); err != nil {
				t.Fatal(err)
			}
			x.mu.Lock()
			calls := append([]mediaEntryPolicyCall(nil), x.calls...)
			x.mu.Unlock()
			if len(calls) != 1 || calls[0].EntryZone != "internet" || x.media.removed != 1 {
				t.Fatal("legacy/invalid sealed fact inherited sweep caller's LAN", calls)
			}
		})
	}
}

func TestMediaEntryZoneCurrentHTTPOverridesSealedLAN(t *testing.T) {
	x := newMediaEntryFixture(t, 1)
	token, _, _, _ := x.issue(t, identity.WithEntryZone(context.Background(), "lan"), x.actor)
	for _, tc := range []struct {
		origin, entry, zone string
		want                int
	}{{"https://192.168.10.11:9885", "lan", "lan", 204}, {"https://im.szputy.com:9885", "internet", "internet", 403}, {"https://im.szputy.com:9885", "lan", "lan", 403}, {"https://192.168.10.11:9885", "lan", "", 403}} {
		r := httptest.NewRequest("GET", tc.origin+"/livekit/rtc?access_token="+token, nil)
		r.RemoteAddr = "127.0.0.1:1234"
		r.Header.Set("X-Real-IP", "192.168.14.8")
		r.Header.Set("X-Puty-Entry", tc.entry)
		if tc.zone != "" {
			r.Header.Set("X-Puty-Access-Zone", tc.zone)
		}
		w := httptest.NewRecorder()
		x.server.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("current ingress %s/%s gained sealed LAN: got %d want %d", tc.entry, tc.zone, w.Code, tc.want)
		}
	}
}
