package enterprise

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"google.golang.org/protobuf/encoding/protojson"
)

type presenceFixture struct {
	users []*livekit.ParticipantInfo
	err   error
}

type retryRemovalFixture struct {
	*testMedia
	failures int
}

func (m *retryRemovalFixture) Remove(ctx context.Context, room, user string) error {
	if m.failures > 0 {
		m.failures--
		return errors.New("injected remove outage")
	}
	return m.testMedia.Remove(ctx, room, user)
}

func (p *presenceFixture) Participants(context.Context, string) ([]*livekit.ParticipantInfo, error) {
	return p.users, p.err
}

func activeMediaFixture(t *testing.T, kind string, session int) (*Server, *testMedia, *presenceFixture, *time.Time, Room) {
	t.Helper()
	s, _, media, now := fixture(t)
	r := Room{RoomID: "lifecycle-room", MediaRoom: "ent_lifecycle_unique_generation", Kind: kind, State: "active", Invitation: Invitation{InviterUserID: "alice", InviteeUserIDList: []string{"bob"}, SessionType: session, GroupID: "g", MediaType: "video"}, MeetingInfo: MeetingInfo{HostUserID: "alice", InviteeUserIDList: []string{"bob"}, EndTime: (*now).Add(time.Hour).Unix()}, Joined: []string{"alice", "bob"}, ParticipantState: map[string]string{"alice": "accepted", "bob": "accepted"}, AcceptedPlatform: map[string]int{"alice": 5, "bob": 5}}
	ctx := context.Background()
	if err := s.Store.Save(ctx, document("room:"+r.RoomID, "room", "alice", r), 0); err != nil {
		t.Fatal(err)
	}
	for _, user := range r.Joined {
		if _, err := s.issueMediaToken(ctx, actor(user), r); err != nil {
			t.Fatal(err)
		}
		if kind == "call" {
			if err := s.acquireCall(ctx, user, r.RoomID); err != nil {
				t.Fatal(err)
			}
		}
	}
	r, _, _ = load[Room](ctx, s.Store, "room:"+r.RoomID)
	p := &presenceFixture{}
	for _, user := range r.Joined {
		p.users = append(p.users, &livekit.ParticipantInfo{Identity: user, Sid: "sid-" + user, State: livekit.ParticipantInfo_ACTIVE, Attributes: map[string]string{"enterpriseLease": r.CurrentLease[user]}})
	}
	s.Presence = p
	return s, media, p, now, r
}

func signedLifecycle(t *testing.T, media *testMedia, event *livekit.WebhookEvent) (*http.Request, []byte) {
	t.Helper()
	b, err := protojson.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(b)
	token, err := auth.NewAccessToken(media.Key, media.Secret).SetValidFor(time.Minute).SetSha256(base64.StdEncoding.EncodeToString(digest[:])).ToJWT()
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/internal/livekit/webhook", strings.NewReader(string(b)))
	r.Header.Set("Authorization", token)
	return r, b
}

func TestLiveKitWebhookSignatureAndGeneration(t *testing.T) {
	s, media, p, now, room := activeMediaFixture(t, "call", 1)
	event := &livekit.WebhookEvent{Id: "joined-1", Event: "participant_joined", CreatedAt: now.Unix(), Room: &livekit.Room{Name: room.MediaRoom}, Participant: p.users[1]}
	r, body := signedLifecycle(t, media, event)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatalf("valid LiveKit event: %d %s", w.Code, w.Body.String())
	}
	// Exact duplicate cannot create another observation or remove membership.
	r, _ = signedLifecycle(t, media, event)
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatal(w.Body.String())
	}
	for _, invalid := range []string{"unsigned", "changed body", "join token"} {
		r, _ = signedLifecycle(t, media, event)
		switch invalid {
		case "unsigned":
			r.Header.Del("Authorization")
		case "changed body":
			r.Body = http.NoBody
		case "join token":
			token, _ := media.Token(room.MediaRoom, "bob", nil, room.CurrentLease["bob"], "")
			r.Header.Set("Authorization", token)
		}
		w = httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatalf("%s accepted: %d (original bytes %d)", invalid, w.Code, len(body))
		}
	}
	// Validly signed old generation/old lease still cannot change current state.
	event.Event, event.Id = "participant_left", "old-generation"
	event.Room.Name = "ent_previous_generation"
	r, _ = signedLifecycle(t, media, event)
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	event.Room.Name, event.Id = room.MediaRoom, "old-lease"
	event.Participant = &livekit.ParticipantInfo{Identity: "bob", Sid: "old-sid", Attributes: map[string]string{"enterpriseLease": "previous-lease"}}
	r, _ = signedLifecycle(t, media, event)
	w = httptest.NewRecorder()
	s.ServeHTTP(w, r)
	o, _, err := load[mediaObservation](context.Background(), s.Store, observationKey(room.RoomID, "bob"))
	if err != nil || o.MissingSince != 0 || o.LastEventID != "joined-1" {
		t.Fatalf("stale callback changed current participant: %+v %v", o, err)
	}
}

func TestLiveKitDisconnectGraceReconnectAndLateCallbacks(t *testing.T) {
	s, _, p, now, room := activeMediaFixture(t, "call", 1)
	ctx := context.Background()
	if err := s.reconcileMediaPresence(ctx); err != nil {
		t.Fatal(err)
	}
	left := &livekit.WebhookEvent{Id: "left-1", Event: "participant_left", CreatedAt: now.Unix(), Room: &livekit.Room{Name: room.MediaRoom}, Participant: p.users[1]}
	if err := s.handleMediaEvent(ctx, left); err != nil {
		t.Fatal(err)
	}
	p.users = p.users[:1]
	*now = now.Add(20 * time.Second)
	if err := s.reconcileMediaPresence(ctx); err != nil {
		t.Fatal(err)
	}
	r, _, _ := load[Room](ctx, s.Store, "room:"+room.RoomID)
	if !contains(r.Joined, "bob") || r.ended() {
		t.Fatal("short disconnection ended the call")
	}
	rejoined := &livekit.ParticipantInfo{Identity: "bob", Sid: "sid-bob-new", State: livekit.ParticipantInfo_ACTIVE, Attributes: map[string]string{"enterpriseLease": room.CurrentLease["bob"]}}
	p.users = append(p.users, rejoined)
	joined := &livekit.WebhookEvent{Id: "joined-2", Event: "participant_joined", CreatedAt: now.Unix(), Room: &livekit.Room{Name: room.MediaRoom}, Participant: rejoined}
	if err := s.handleMediaEvent(ctx, joined); err != nil {
		t.Fatal(err)
	}
	// A late old transport's leave event may even be timestamped later. SID and
	// authoritative presence stop it from disconnecting the replacement.
	left.Id, left.CreatedAt = "late-old-left", now.Add(time.Second).Unix()
	if err := s.handleMediaEvent(ctx, left); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(40 * time.Second)
	if err := s.reconcileMediaPresence(ctx); err != nil {
		t.Fatal(err)
	}
	r, _, _ = load[Room](ctx, s.Store, "room:"+room.RoomID)
	if !contains(r.Joined, "bob") || r.ended() {
		t.Fatal("late event removed successfully reconnected participant")
	}
}

func TestLiveKitMissingWebhookRestartAndRoomServiceOutage(t *testing.T) {
	s, media, p, now, room := activeMediaFixture(t, "call", 1)
	ctx := context.Background()
	if err := s.reconcileMediaPresence(ctx); err != nil {
		t.Fatal(err)
	}
	// No webhook at all: the participant list must eventually release busy state.
	p.users = p.users[:1]
	*now = now.Add(10 * time.Second)
	if err := s.reconcileMediaPresence(ctx); err != nil {
		t.Fatal(err)
	}
	p.err = errors.New("RoomService unavailable")
	*now = now.Add(time.Minute)
	if err := s.reconcileMediaPresence(ctx); err == nil {
		t.Fatal("expected upstream outage")
	}
	r, _, _ := load[Room](ctx, s.Store, "room:"+room.RoomID)
	if r.ended() || !contains(r.Joined, "bob") {
		t.Fatal("upstream outage treated as absence")
	}
	p.err = nil
	// New service instance shares only Mongo-equivalent persisted state; neither
	// an in-memory timer nor a new webhook is needed to finish the disconnection.
	s2 := &Server{Store: s.Store, Auth: s.Auth, IM: s.IM, Media: media, Presence: p, Cipher: s.Cipher, Now: s.Now}
	if err := s2.reconcileMediaPresence(ctx); err != nil {
		t.Fatal(err)
	}
	r, _, _ = load[Room](ctx, s.Store, "room:"+room.RoomID)
	if !r.ended() || r.Reason != "transport_disconnected" || contains(r.Joined, "bob") || r.CurrentLease["bob"] != "" || media.deleted == 0 {
		t.Fatalf("lost participant remained active after restart: %+v", r.view())
	}
	for _, user := range []string{"alice", "bob"} {
		if err := s2.acquireCall(ctx, user, "next-call"); err != nil {
			t.Fatalf("%s permanently busy after disconnect: %v", user, err)
		}
	}
}

func TestLiveKitGroupDisconnectRetainsOthersAndMeetingSchedule(t *testing.T) {
	for _, kind := range []string{"call", "meeting"} {
		t.Run(kind, func(t *testing.T) {
			s, _, p, now, room := activeMediaFixture(t, kind, 3)
			ctx := context.Background()
			p.users = p.users[:1]
			if err := s.reconcileMediaPresence(ctx); err != nil {
				t.Fatal(err)
			}
			*now = now.Add(31 * time.Second)
			if err := s.reconcileMediaPresence(ctx); err != nil {
				t.Fatal(err)
			}
			r, _, _ := load[Room](ctx, s.Store, "room:"+room.RoomID)
			if r.ended() || !contains(r.Joined, "alice") || contains(r.Joined, "bob") {
				t.Fatal("one disconnected participant ended the room for everyone")
			}
			if kind == "meeting" && r.MeetingInfo.EndTime != room.MeetingInfo.EndTime {
				t.Fatal("disconnection changed meeting schedule")
			}
		})
	}
}

func TestLiveKitRemovalRetrySurvivesRestartAndNeverKicksNewLease(t *testing.T) {
	s, media, p, now, room := activeMediaFixture(t, "meeting", 3)
	broken := &retryRemovalFixture{testMedia: media, failures: 1}
	s.Media = broken
	ctx := context.Background()
	p.users = p.users[:1]
	if err := s.reconcileMediaPresence(ctx); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(31 * time.Second)
	if err := s.reconcileMediaPresence(ctx); err == nil {
		t.Fatal("injected removal should be retried, not forgotten")
	}
	o, _, err := load[mediaObservation](ctx, s.Store, observationKey(room.RoomID, "bob"))
	if err != nil || !o.RemovalPending {
		t.Fatal("failed media removal was not persisted")
	}
	s2 := &Server{Store: s.Store, Auth: s.Auth, IM: s.IM, Media: broken, Presence: p, Cipher: s.Cipher, Now: s.Now}
	if err := s2.reconcileMediaPresence(ctx); err != nil {
		t.Fatal(err)
	}
	o, _, _ = load[mediaObservation](ctx, s.Store, observationKey(room.RoomID, "bob"))
	if o.RemovalPending || media.removed != 1 {
		t.Fatal("pending removal did not recover after restart")
	}
	// Reproduce a delayed cleanup belonging to an older lease after a legitimate
	// participant has rejoined. Identity alone is not enough to authorize a kick.
	o.RemovalPending = true
	d, _ := s.Store.Get(ctx, observationKey(room.RoomID, "bob"))
	d.Data = mustJSON(o)
	if err := s.Store.Save(ctx, d, d.Version); err != nil {
		t.Fatal(err)
	}
	_, err = mutate[Room](ctx, s.Store, "room:"+room.RoomID, func(r *Room) error {
		r.Joined = unique(append(r.Joined, "bob"))
		r.CurrentLease["bob"] = "new-valid-lease"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s2.retryMediaRemovals(ctx); err != nil || media.removed != 1 {
		t.Fatal("older cleanup kicked a newer participant lease")
	}
}

func TestLiveKitParticipantListUsesProtoJSONAndFailsClosed(t *testing.T) {
	status := http.StatusOK
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/twirp/livekit.RoomService/ListParticipants" {
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		switch status {
		case 200:
			_, _ = w.Write([]byte(`{"participants":[{"sid":"PA_test","identity":"bob","state":"ACTIVE","attributes":{"enterpriseLease":"lease"},"joinedAt":"12345"}]}`))
		case 404:
			_, _ = w.Write([]byte(`{"code":"not_found"}`))
		default:
			_, _ = w.Write([]byte(`{"code":"unavailable"}`))
		}
	}))
	defer api.Close()
	lk, err := NewLiveKit(api.URL, "wss://test.invalid/livekit", "key", strings.Repeat("s", 40))
	if err != nil {
		t.Fatal(err)
	}
	users, err := lk.Participants(context.Background(), "room")
	if err != nil || len(users) != 1 || users[0].State != livekit.ParticipantInfo_ACTIVE || users[0].JoinedAt != 12345 {
		t.Fatalf("protobuf JSON participant list: %+v %v", users, err)
	}
	status = 404
	if users, err = lk.Participants(context.Background(), "room"); err != nil || len(users) != 0 {
		t.Fatalf("deleted room: %v", err)
	}
	status = 503
	if _, err = lk.Participants(context.Background(), "room"); err == nil {
		t.Fatal("unavailable server reported an empty room")
	}
}
