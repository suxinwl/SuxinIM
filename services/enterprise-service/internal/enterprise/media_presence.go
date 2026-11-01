package enterprise

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	"google.golang.org/protobuf/encoding/protojson"
)

const mediaReconnectGrace = 30 * time.Second

// Presence is optional for deterministic domain tests. Production always uses
// the authenticated LiveKit RoomService, including after a service restart.
type MediaPresence interface {
	Participants(context.Context, string) ([]*livekit.ParticipantInfo, error)
}

type mediaWebhookReceiver interface {
	ReceiveWebhook(*http.Request) (*livekit.WebhookEvent, error)
}

func (l *LiveKit) Participants(ctx context.Context, room string) ([]*livekit.ParticipantInfo, error) {
	var result livekit.ListParticipantsResponse
	var raw json.RawMessage
	if err := l.call(ctx, "ListParticipants", room, map[string]string{"room": room}, &raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 { // An already deleted room has no participants.
		return nil, nil
	}
	// LiveKit returns protobuf enum names (e.g. ACTIVE), not Go enum integers.
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result.Participants, nil
}

// LiveKit authenticates webhook requests with a signed, expiring JWT whose
// sha256 claim covers the exact body bytes. A normal room join JWT is therefore
// insufficient to forge lifecycle notifications.
func (l *LiveKit) ReceiveWebhook(r *http.Request) (*livekit.WebhookEvent, error) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, bad("invalid media webhook body")
	}
	v, err := auth.ParseAPIToken(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if err != nil || v.APIKey() != l.Key {
		return nil, unauthorized("invalid media webhook signature")
	}
	claims, err := v.Verify(l.Secret)
	if err != nil {
		return nil, unauthorized("expired media webhook signature")
	}
	digest := sha256.Sum256(data)
	want := base64.StdEncoding.EncodeToString(digest[:])
	if subtle.ConstantTimeCompare([]byte(claims.Sha256), []byte(want)) != 1 {
		return nil, unauthorized("media webhook body checksum differs")
	}
	var event livekit.WebhookEvent
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, &event); err != nil {
		return nil, bad("invalid media webhook event")
	}
	return &event, nil
}

type mediaObservation struct {
	RoomID, MediaRoom, UserID, Lease string
	ParticipantSID                   string
	MissingSince                     int64
	LastEventAt                      int64
	LastEventID                      string
	RemovalPending                   bool
}

func observationKey(room, user string) string { return "media_observation:" + room + ":" + user }

func (s *Server) observeMedia(ctx context.Context, room Room, user string, change func(*mediaObservation)) (mediaObservation, error) {
	key := observationKey(room.RoomID, user)
	for tries := 0; tries < 16; tries++ {
		o, d, err := load[mediaObservation](ctx, s.Store, key)
		if errors.Is(err, ErrNotFound) {
			d = document(key, "media_observation", room.RoomID, o)
		} else if err != nil {
			return o, err
		}
		if o.MediaRoom != room.MediaRoom || o.Lease != room.CurrentLease[user] {
			o = mediaObservation{RoomID: room.RoomID, MediaRoom: room.MediaRoom, UserID: user, Lease: room.CurrentLease[user]}
		}
		change(&o)
		d.Data = mustJSON(o)
		if err := s.Store.Save(ctx, d, d.Version); errors.Is(err, ErrConflict) {
			continue
		} else {
			return o, err
		}
	}
	return mediaObservation{}, ErrConflict
}

func (s *Server) livekitWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, bad("POST required"))
		return
	}
	receiver, ok := s.Media.(mediaWebhookReceiver)
	if !ok {
		writeError(w, &apiError{503, 1006, "media verifier unavailable"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	event, err := receiver.ReceiveWebhook(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.handleMediaEvent(r.Context(), event); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMediaEvent(ctx context.Context, event *livekit.WebhookEvent) error {
	if event.GetRoom().GetName() == "" {
		return nil
	}
	if event.Event != "participant_joined" && event.Event != "participant_left" && event.Event != "room_finished" {
		return nil
	}
	rooms, err := s.rooms(ctx)
	if err != nil {
		return err
	}
	for _, room := range rooms {
		// MediaRoom includes a random generation. A delayed callback for an old
		// room can never affect a later room with the same client-supplied ID.
		if room.ended() || room.MediaRoom != event.Room.Name {
			continue
		}
		for _, user := range room.Joined {
			participant := event.GetParticipant()
			if event.Event != "room_finished" && (participant.GetIdentity() != user || participant.GetAttributes()["enterpriseLease"] != room.CurrentLease[user]) {
				continue
			}
			_, err = s.observeMedia(ctx, room, user, func(o *mediaObservation) {
				if event.Id != "" && o.LastEventID == event.Id || event.CreatedAt < o.LastEventAt {
					return
				}
				// Reconnecting with the same lease can have a new participant SID.
				// A delayed left event from the old transport must not cancel it.
				if event.Event == "participant_left" && o.ParticipantSID != "" && o.ParticipantSID != participant.GetSid() {
					return
				}
				o.LastEventAt, o.LastEventID = event.CreatedAt, event.Id
				if event.Event == "participant_joined" {
					o.ParticipantSID = participant.GetSid()
					o.MissingSince = 0
				} else if o.MissingSince == 0 {
					o.MissingSince = s.now().UnixMilli()
				}
			})
			if err != nil {
				return err
			}
		}
		return nil
	}
	return nil
}

// Webhooks are hints, not a reliable queue. The authenticated participant list
// is authoritative, so missed/duplicate/late callbacks and restarts converge.
// Unavailable RoomService must never be interpreted as an empty room.
func (s *Server) reconcileMediaPresence(ctx context.Context) error {
	if s.Presence == nil || !s.lastPresenceCheck.IsZero() && s.now().Sub(s.lastPresenceCheck) < 10*time.Second {
		return nil
	}
	s.lastPresenceCheck = s.now()
	if err := s.retryMediaRemovals(ctx); err != nil {
		return err
	}
	rooms, err := s.rooms(ctx)
	if err != nil {
		return err
	}
	var firstError error
	for _, room := range rooms {
		if room.ended() || len(room.Joined) == 0 {
			continue
		}
		participants, err := s.Presence.Participants(ctx, room.MediaRoom)
		if err != nil {
			if firstError == nil {
				firstError = err
			}
			continue
		}
		for _, user := range room.Joined {
			lease := room.CurrentLease[user]
			// No issued token yet: issueMediaToken may be in progress. Its next
			// completed grant will be considered on the following reconciliation.
			if lease == "" {
				continue
			}
			var present *livekit.ParticipantInfo
			for _, p := range participants {
				if p.Identity == user && p.Attributes["enterpriseLease"] == lease && p.State != livekit.ParticipantInfo_DISCONNECTED {
					present = p
					break
				}
			}
			o, err := s.observeMedia(ctx, room, user, func(o *mediaObservation) {
				if present != nil {
					o.ParticipantSID, o.MissingSince = present.Sid, 0
				} else if o.MissingSince == 0 {
					o.MissingSince = s.now().UnixMilli()
				}
			})
			if err != nil {
				return err
			}
			if present != nil || s.now().UnixMilli()-o.MissingSince < mediaReconnectGrace.Milliseconds() {
				continue
			}
			// Persist the external side effect before changing membership so a
			// crash or RoomService failure cannot strand a late media transport.
			if _, err := s.observeMedia(ctx, room, user, func(o *mediaObservation) { o.RemovalPending = true }); err != nil {
				return err
			}
			changed := false
			updated, err := mutate[Room](ctx, s.Store, "room:"+room.RoomID, func(r *Room) error {
				changed = false
				if r.ended() || r.MediaRoom != room.MediaRoom || r.CurrentLease[user] != lease || !contains(r.Joined, user) {
					return nil
				}
				changed = true
				r.Joined = without(r.Joined, user)
				r.ParticipantState[user] = "left"
				delete(r.CurrentLease, user)
				r.event("OnRoomParticipantDisconnected", r.audience(), signalData(*r, user))
				if r.Kind == "call" && (r.Invitation.SessionType == 1 || len(r.Joined) == 0) {
					r.State, r.EndedAt, r.Reason = "ended", s.now().Unix(), "transport_disconnected"
					r.event("OnHangUp", r.audience(), signalData(*r, user))
				}
				return nil
			})
			if err != nil {
				return err
			}
			if !changed {
				continue
			}
			if updated.Kind == "call" {
				if err := s.releaseCall(ctx, user, room.RoomID); err != nil {
					return err
				}
			}
			if err := s.dispatchRoom(ctx, updated); err != nil {
				return err
			}
			if err := s.cleanRoom(ctx, updated); err != nil {
				return err
			}
		}
	}
	if err := s.retryMediaRemovals(ctx); err != nil && firstError == nil {
		firstError = err
	}
	return firstError
}

func (s *Server) retryMediaRemovals(ctx context.Context) error {
	docs, err := s.Store.List(ctx, "media_observation", "")
	if err != nil {
		return err
	}
	var firstError error
	for _, d := range docs {
		var o mediaObservation
		if err := json.Unmarshal(d.Data, &o); err != nil {
			return err
		}
		if !o.RemovalPending {
			continue
		}
		r, _, err := load[Room](ctx, s.Store, "room:"+o.RoomID)
		if err != nil {
			return err
		}
		newLease := r.CurrentLease[o.UserID]
		if r.MediaRoom == o.MediaRoom && newLease == o.Lease && contains(r.Joined, o.UserID) && !r.ended() {
			continue // The membership CAS has not committed yet.
		}
		if r.MediaRoom == o.MediaRoom && (newLease == "" || newLease == o.Lease || r.ended()) {
			if err := s.Media.Remove(ctx, o.MediaRoom, o.UserID); err != nil {
				if firstError == nil {
					firstError = err
				}
				continue
			}
		}
		// A newer lease is allowed to stay connected. Never remove its transport
		// while retrying cleanup belonging to an older participant generation.
		o.RemovalPending = false
		d.Data = mustJSON(o)
		if err := s.Store.Save(ctx, d, d.Version); err != nil && !errors.Is(err, ErrConflict) {
			return err
		}
	}
	return firstError
}
