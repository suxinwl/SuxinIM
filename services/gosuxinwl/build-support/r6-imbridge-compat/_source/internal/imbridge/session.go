package imbridge

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"
)

type Service struct {
	Store        *SQLStore
	Cache        Cache
	Remote       Remote
	Now          func() time.Time
	sessionLocks sync.Map
}

func (s *Service) lockPlatform(v Session) func() {
	value, _ := s.sessionLocks.LoadOrStore(platformKey(v), &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	return lock.Unlock
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
func (s *Service) newSession(ctx context.Context, uid int64, platform string) (Session, error) {
	p, e := s.Store.Eligible(ctx, uid)
	if e != nil {
		return Session{}, ErrDenied
	}
	id, e := token()
	if e != nil {
		return Session{}, e
	}
	plat, e := Platform(platform)
	if e != nil {
		return Session{}, e
	}
	if e = s.checkSessionAccess(ctx, uid, plat); e != nil {
		return Session{}, e
	}
	epoch, e := s.Store.Epoch(ctx, uid)
	if e != nil {
		return Session{}, e
	}
	return Session{ID: id, UserID: p.UserID, UID: uid, Platform: plat, Epoch: epoch, ExpiresAt: s.now().Add(SessionLifetime).Unix(), Profile: p.Profile}, nil
}
func (s *Service) IssueTicket(ctx context.Context, uid int64) (string, error) {
	v, e := s.newSession(ctx, uid, "web")
	if e != nil {
		return "", e
	}
	t, e := token()
	if e != nil {
		return "", e
	}
	if e = s.Cache.Put(ctx, "ticket:"+digest(t), Ticket{Session: v, Audience: "web"}, TicketLifetime); e != nil {
		return "", e
	}
	return t, nil
}
func (s *Service) Exchange(ctx context.Context, t, platform string) (LoginResult, error) {
	if len(t) != 64 || platform != "web" {
		return LoginResult{}, ErrExpired
	}
	var v Ticket
	if e := s.Cache.Take(ctx, "ticket:"+digest(t), &v); e != nil {
		return LoginResult{}, ErrExpired
	}
	if v.Audience != platform {
		return LoginResult{}, ErrExpired
	}
	if _, e := s.Store.Eligible(ctx, v.Session.UID); e != nil {
		return LoginResult{}, ErrDenied
	}
	ep, e := s.Store.Epoch(ctx, v.Session.UID)
	if e != nil || ep != v.Session.Epoch {
		return LoginResult{}, ErrExpired
	}
	return s.activate(ctx, t, v.Session)
}
func (s *Service) Login(ctx context.Context, uid int64, platform string) (LoginResult, error) {
	v, e := s.newSession(ctx, uid, platform)
	if e != nil {
		return LoginResult{}, e
	}
	t, e := token()
	if e != nil {
		return LoginResult{}, e
	}
	return s.activate(ctx, t, v)
}
func (s *Service) activate(ctx context.Context, t string, v Session) (LoginResult, error) {
	unlock := s.lockPlatform(v)
	defer unlock()
	if e := s.checkSessionAccess(ctx, v.UID, v.Platform); e != nil {
		return LoginResult{}, e
	}
	if _, e := s.Store.Eligible(ctx, v.UID); e != nil {
		return LoginResult{}, ErrDenied
	}
	if current, e := s.Store.Epoch(ctx, v.UID); e != nil || current != v.Epoch {
		return LoginResult{}, ErrExpired
	}
	raw, e := token()
	if e != nil {
		return LoginResult{}, e
	}
	hash := digest(raw)
	tokens, e := s.Remote.Exchange(ctx, t, v)
	if e != nil {
		return LoginResult{}, e
	}
	old, e := s.Cache.ReplacePlatform(ctx, v, hash, time.Unix(v.ExpiresAt, 0).Sub(s.now()))
	if e != nil {
		_ = s.Remote.Revoke(ctx, Revoke{UserID: v.UserID, SessionID: v.ID, Platform: v.Platform})
		return LoginResult{}, e
	}
	if old != "" && old != hash {
		var previous Session
		if s.Cache.Get(ctx, "sid:"+old, &previous) == nil {
			revoke := Revoke{UserID: previous.UserID, SessionID: previous.ID, Platform: previous.Platform}
			if e = s.Store.QueueRevoke(ctx, revoke); e != nil {
				_ = s.Cache.Delete(ctx, "sid:"+hash)
				_ = s.Remote.Revoke(ctx, Revoke{UserID: v.UserID, SessionID: v.ID})
				return LoginResult{}, e
			}
			_ = s.Cache.Delete(ctx, "sid:"+old)
			if e = s.Remote.Revoke(ctx, revoke); e != nil {
				_ = s.Cache.Delete(ctx, "sid:"+hash)
				_ = s.Remote.Revoke(ctx, Revoke{UserID: v.UserID, SessionID: v.ID})
				return LoginResult{}, e
			}
		}
	}
	return LoginResult{UserID: v.UserID, IMToken: tokens.IMToken, ChatToken: tokens.ChatToken, AttachmentToken: tokens.AttachmentToken, SessionToken: raw, ExpiresAt: v.ExpiresAt, Profile: v.Profile}, nil
}
func (s *Service) Validate(ctx context.Context, raw string) (Session, error) {
	if len(raw) != 64 {
		return Session{}, ErrExpired
	}
	var v Session
	if e := s.Cache.Get(ctx, "sid:"+digest(raw), &v); e != nil {
		return v, ErrExpired
	}
	if v.ExpiresAt <= s.now().Unix() {
		return v, ErrExpired
	}
	ok, e := s.Cache.CheckPlatform(ctx, v, digest(raw))
	if e != nil || !ok {
		return v, ErrExpired
	}
	epoch, e := s.Store.Epoch(ctx, v.UID)
	if e != nil || epoch != v.Epoch {
		return v, ErrExpired
	}
	p, e := s.Store.Eligible(ctx, v.UID)
	if e != nil || p.UserID != v.UserID {
		return v, ErrDenied
	}
	if e = s.checkSessionAccess(ctx, v.UID, v.Platform); e != nil {
		return v, e
	}
	v.Profile = p.Profile
	return v, nil
}
func (s *Service) Refresh(ctx context.Context, raw string) (LoginResult, error) {
	v, e := s.Validate(ctx, raw)
	if e != nil {
		return LoginResult{}, e
	}
	unlock := s.lockPlatform(v)
	defer unlock()
	v, e = s.Validate(ctx, raw)
	if e != nil {
		return LoginResult{}, e
	}
	if time.Unix(v.ExpiresAt, 0).Sub(s.now()) <= RefreshWindow {
		v.ExpiresAt = s.now().Add(SessionLifetime).Unix()
	}
	t, e := token()
	if e != nil {
		return LoginResult{}, e
	}
	tokens, e := s.Remote.Exchange(ctx, t, v)
	if e != nil {
		return LoginResult{}, e
	}
	ok, e := s.Cache.RefreshPlatform(ctx, v, digest(raw), time.Unix(v.ExpiresAt, 0).Sub(s.now()))
	if e != nil || !ok {
		_ = s.Remote.Revoke(ctx, Revoke{UserID: v.UserID, SessionID: v.ID})
		return LoginResult{}, ErrExpired
	}
	return LoginResult{UserID: v.UserID, IMToken: tokens.IMToken, ChatToken: tokens.ChatToken, AttachmentToken: tokens.AttachmentToken, SessionToken: raw, ExpiresAt: v.ExpiresAt, Profile: v.Profile}, nil
}
func (s *Service) Logout(ctx context.Context, raw string, all bool) error {
	// Logout remains possible after an employee is disabled; do not require
	// current HRM eligibility just to revoke an existing cached session.
	if len(raw) != 64 {
		return ErrExpired
	}
	var v Session
	if e := s.Cache.Get(ctx, "sid:"+digest(raw), &v); e != nil {
		return ErrExpired
	}
	unlock := s.lockPlatform(v)
	defer unlock()
	if v.ExpiresAt <= s.now().Unix() {
		return ErrExpired
	}
	current, e := s.Store.Epoch(ctx, v.UID)
	if e != nil || current != v.Epoch {
		return ErrExpired
	}
	active, e := s.Cache.CheckPlatform(ctx, v, digest(raw))
	if e != nil || !active {
		return ErrExpired
	}
	if all {
		return s.RevokeUID(ctx, v.UID)
	}
	r := Revoke{UserID: v.UserID, SessionID: v.ID, Platform: v.Platform}
	if e := s.Store.QueueRevoke(ctx, r); e != nil {
		return e
	}
	if e := s.Cache.Delete(ctx, "sid:"+digest(raw)); e != nil {
		return e
	}
	return s.Remote.Revoke(ctx, r)
}
func (s *Service) RevokeUID(ctx context.Context, uid int64) error {
	p, e := s.Store.ProfileForUID(ctx, uid)
	if e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			return nil
		}
		return e
	}
	epoch, e := s.Store.BumpEpoch(ctx, uid)
	if e != nil {
		return e
	}
	r := Revoke{UserID: p.UserID, All: true, Epoch: epoch}
	if e = s.Store.QueueRevoke(ctx, r); e != nil {
		return e
	}
	return s.Remote.Revoke(ctx, r)
}
func authStatus(err error) int {
	if errors.Is(err, ErrConflict) {
		return 409
	}
	if errors.Is(err, ErrInvalid) {
		return 400
	}
	if errors.Is(err, ErrExpired) {
		return 401
	}
	if errors.Is(err, ErrDenied) {
		return 403
	}
	return 503
}
