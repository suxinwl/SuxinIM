package enterprise

import (
	"context"
	"errors"
	"google.golang.org/grpc/metadata"
	"net/http"
	"strconv"
	"time"
)

type SessionRevoker interface {
	RevokeIMToken(context.Context, string) error
	RevokeChatToken(context.Context, string, string) error
}

// SessionAuth checks our durable single-token logout barrier before consulting
// upstream registries. Even an upstream outage or a new Chat token cannot reuse
// the IM credential that has already been explicitly logged out.
type SessionAuth struct {
	Inner Authenticator
	Store Store
}

func revokedSessionKey(imToken string) string { return "session_revocation:" + tokenHash(imToken) }
func sessionPairHash(chat, im string) string  { return tokenHash(chat + "\x00" + im) }

func (a *SessionAuth) Authenticate(ctx context.Context, r *http.Request, admin bool) (Actor, error) {
	if !admin && r.Header.Get("token") != "" {
		_, err := a.Store.Get(ctx, revokedSessionKey(r.Header.Get("token")))
		if err == nil {
			return Actor{}, unauthorized("session explicitly logged out")
		}
		if !errors.Is(err, ErrNotFound) {
			return Actor{}, err
		}
	}
	return a.Inner.Authenticate(ctx, r, admin)
}

func (a *GRPCAuth) RevokeIMToken(ctx context.Context, token string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	in := fieldString(nil, 1, token) // locked auth.kickTokensReq repeated tokens
	var out []byte
	if err := a.im.Invoke(ctx, "/openim.auth.Auth/kickTokens", &in, &out); err != nil {
		return &apiError{503, 1006, "IM session revocation temporarily unavailable"}
	}
	return nil
}

func (a *GRPCAuth) RevokeChatToken(ctx context.Context, userID, token string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// No OpenIM tools client interceptor sits on this wire connection. Preserve
	// the explicit custom-header registration for Chat's real server middleware.
	ctx = metadata.AppendToOutgoingContext(ctx, "customHeader", "openim38-chat-revoke-token", "openim38-chat-revoke-token", token)
	in := fieldString(nil, 1, userID) // fixed InvalidateTokenReq.UserID
	var out []byte
	if err := a.chat.Invoke(ctx, "/openim.admin.admin/InvalidateToken", &in, &out); err != nil {
		return &apiError{503, 1006, "Chat session revocation temporarily unavailable"}
	}
	return nil
}

type sessionRevocation struct {
	UserID       string
	Platform     int
	PairHash     string
	ExpiresAt    int64
	Ciphertext   []byte
	Complete     bool
	ChatComplete bool
}

func (s *Server) sessionLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, &apiError{405, 1001, "POST required"})
		return
	}
	im, chat := r.Header.Get("token"), r.Header.Get("chatToken")
	if im == "" || chat == "" {
		writeError(w, unauthorized("Chat and OpenIM tokens required"))
		return
	}
	if s.SessionRevoker == nil || s.Cipher == nil {
		writeError(w, &apiError{503, 1006, "session revoker unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	key := revokedSessionKey(im)
	revoked, _, err := load[sessionRevocation](ctx, s.Store, key)
	if errors.Is(err, ErrNotFound) {
		a, authErr := s.Auth.Authenticate(ctx, r, false)
		if authErr != nil {
			writeError(w, authErr)
			return
		}
		encrypted, sealErr := s.Cipher.seal(a)
		if sealErr != nil {
			writeError(w, sealErr)
			return
		}
		revoked = sessionRevocation{UserID: a.UserID, Platform: a.Platform, PairHash: sessionPairHash(chat, im), ExpiresAt: a.ExpiresAt, Ciphertext: encrypted}
		if err := s.Store.Save(ctx, document(key, "session_revocation", a.UserID, revoked), 0); err != nil && !errors.Is(err, ErrConflict) {
			writeError(w, err)
			return
		}
		// A concurrent request may have won the insert. Read its authoritative
		// identity and pair before continuing the idempotent revocation.
		revoked, _, err = load[sessionRevocation](ctx, s.Store, key)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	if revoked.PairHash != sessionPairHash(chat, im) || r.Header.Get("platformID") != "" && r.Header.Get("platformID") != strconv.Itoa(revoked.Platform) {
		writeError(w, unauthorized("logout credentials differ from the session"))
		return
	}
	if !revoked.Complete || !revoked.ChatComplete {
		if err := s.finishSessionRevocation(ctx, key, revoked); err != nil {
			writeError(w, err)
			return
		}
	}
	writeJSON(w, 200, map[string]any{"revoked": true})
}

func (s *Server) finishSessionRevocation(ctx context.Context, key string, revoked sessionRevocation) error {
	a, err := s.Cipher.open(revoked.Ciphertext)
	if err != nil {
		return err
	}
	// Always remove media access even while the upstream revocation is retrying.
	// SessionAuth rejects this exact IM credential from the durable record above.
	imErr := s.SessionRevoker.RevokeIMToken(ctx, a.IMToken)
	chatErr := s.SessionRevoker.RevokeChatToken(ctx, a.UserID, a.ChatToken)
	if err := s.revalidateMediaSessions(ctx); err != nil {
		return err
	}
	if imErr != nil {
		return imErr
	}
	if chatErr != nil {
		return chatErr
	}
	_, err = mutate[sessionRevocation](ctx, s.Store, key, func(v *sessionRevocation) error {
		v.Complete = true
		v.ChatComplete = true
		return nil
	})
	return err
}

func (s *Server) retrySessionRevocations(ctx context.Context) error {
	if s.SessionRevoker == nil || s.Cipher == nil || !s.lastRevocationCheck.IsZero() && s.now().Sub(s.lastRevocationCheck) < 10*time.Second {
		return nil
	}
	s.lastRevocationCheck = s.now()
	ds, err := s.Store.List(ctx, "session_revocation", "")
	if err != nil {
		return err
	}
	for _, d := range ds {
		v, _, err := load[sessionRevocation](ctx, s.Store, d.ID)
		if err != nil {
			return err
		}
		if !v.Complete || !v.ChatComplete {
			if err := s.finishSessionRevocation(ctx, d.ID, v); err != nil {
				return err
			}
		}
	}
	return nil
}
