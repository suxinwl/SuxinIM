package enterprise

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/livekit/protocol/auth"
)

func TestParticipantTokenCarriesAuthoritativeProfileAndPublishPolicy(t *testing.T) {
	s, _, media, _ := fixture(t)
	r := Room{Kind: "meeting", MeetingInfo: MeetingInfo{HostUserID: "alice", OnlyHostShareScreen: true}}
	metadata, err := s.participantMetadata(context.Background(), "bob", r)
	if err != nil {
		t.Fatal(err)
	}
	token, err := media.Token("media-room", "bob", r.sources("bob"), "lease", metadata)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := auth.ParseAPIToken(token)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := verifier.Verify(media.Secret)
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		UserInfo User `json:"userInfo"`
	}
	if err := json.Unmarshal([]byte(claims.Metadata), &info); err != nil || info.UserInfo.UserID != "bob" || info.UserInfo.Nickname != "Name bob" {
		t.Fatalf("participant identity not hydrated from IM: %+v %v", info, err)
	}
	if claims.Video.CanPublish == nil || *claims.Video.CanPublish || len(claims.Video.CanPublishSources) != 0 {
		t.Fatal("member bypassed disabled microphone/camera/screen policy")
	}
	if len(r.sources("alice")) != 4 {
		t.Fatal("host publication unexpectedly restricted")
	}
	r.MeetingInfo.ParticipantCanUnmuteSelf = true
	if got := r.sources("bob"); len(got) != 1 || got[0] != "microphone" {
		t.Fatalf("unlocked member got extra media sources: %v", got)
	}
}

func TestLiveKitDeleteRoomUsesCreatePermissionAndAbsentParticipantIsSafe(t *testing.T) {
	const secret = "test-livekit-server-secret-with-enough-entropy"
	checkedDelete := false
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		v, e := auth.ParseAPIToken(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if e != nil {
			t.Error(e)
			w.WriteHeader(401)
			return
		}
		claims, e := v.Verify(secret)
		if e != nil {
			t.Error(e)
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/twirp/livekit.RoomService/DeleteRoom":
			checkedDelete = true
			if claims.Video == nil || !claims.Video.RoomCreate {
				w.WriteHeader(401)
				_, _ = w.Write([]byte(`{"code":"permission_denied"}`))
				return
			}
			_, _ = w.Write([]byte(`{}`))
		case "/twirp/livekit.RoomService/UpdateParticipant", "/twirp/livekit.RoomService/GetParticipant":
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"code":"not_found","msg":"no active transport"}`))
		default:
			t.Errorf("unexpected method %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer api.Close()
	lk, e := NewLiveKit(api.URL, "wss://test.invalid/livekit", "test", secret)
	if e != nil {
		t.Fatal(e)
	}
	if e = lk.Delete(context.Background(), "room"); e != nil || !checkedDelete {
		t.Fatalf("delete permission: %v", e)
	}
	if e = lk.Permissions(context.Background(), "room", "alice", []string{"camera"}); e != nil {
		t.Fatalf("pre-join permission update failed: %v", e)
	}
	if e = lk.Mute(context.Background(), "room", "alice", "audio", true); e != nil {
		t.Fatalf("absent participant mute failed: %v", e)
	}
}
