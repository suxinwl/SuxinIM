package enterprise

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type callPresence struct {
	RoomID        string
	ReservedUntil int64
}

// A per-user CAS reservation closes the race where two invitations are accepted
// concurrently in different rooms. A crashed pre-commit reservation expires.
func (s *Server) acquireCall(ctx context.Context, user, roomID string) error {
	key := "call_presence:" + user
	for i := 0; i < 16; i++ {
		p, d, err := load[callPresence](ctx, s.Store, key)
		if err == nil {
			if p.RoomID == roomID {
				return nil
			}
			old, _, e := load[Room](ctx, s.Store, "room:"+p.RoomID)
			if e != nil && !errors.Is(e, ErrNotFound) {
				return e
			}
			active := e == nil && !old.ended() && contains(old.Joined, user)
			reserved := s.now().Unix() < p.ReservedUntil && (errors.Is(e, ErrNotFound) || e == nil && !old.ended())
			if active || reserved {
				return conflict("user is already in another call")
			}
		} else if !errors.Is(err, ErrNotFound) {
			return err
		} else {
			d = document(key, "call_presence", user, p)
		}
		p = callPresence{RoomID: roomID, ReservedUntil: s.now().Add(10 * time.Second).Unix()}
		d.Data, _ = json.Marshal(p)
		err = s.Store.Save(ctx, d, d.Version)
		if errors.Is(err, ErrConflict) {
			continue
		}
		return err
	}
	return ErrConflict
}
func (s *Server) releaseCall(ctx context.Context, user, roomID string) error {
	p, d, err := load[callPresence](ctx, s.Store, "call_presence:"+user)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if p.RoomID != roomID {
		return nil
	}
	err = s.Store.Delete(ctx, d.ID, d.Version)
	if errors.Is(err, ErrConflict) {
		return nil
	}
	return err
}
func (s *Server) releaseUnjoinedCall(ctx context.Context, user, roomID string) {
	r, _, err := load[Room](ctx, s.Store, "room:"+roomID)
	if err == nil && contains(r.Joined, user) && !r.ended() {
		return
	}
	if err == nil || errors.Is(err, ErrNotFound) {
		_ = s.releaseCall(ctx, user, roomID)
	}
}
