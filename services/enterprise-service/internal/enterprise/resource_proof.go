package enterprise

import (
	"context"
	"net/http"
	"time"

	"openim38.local/identity"
)

const resourceCookie = "__Host-puty-im-resource"
const resourceProofHeader = "X-Puty-Resource-Proof"

func setResourceCookie(w http.ResponseWriter, proof string, expires int64) {
	http.SetCookie(w, &http.Cookie{Name: resourceCookie, Value: proof, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: time.Unix(expires, 0), MaxAge: int(time.Until(time.Unix(expires, 0)).Seconds())})
}

// A copied grant is not sufficient. The browser's host-only cookie or a
// native caller's separate proof/dual-token pair must own this exact live SID.
// Explicit proofs take precedence over ambient cookies on shared origins.
func (s *Server) attachmentReadSession(ctx context.Context, state *identity.Store, r *http.Request) (identity.Session, error) {
	origin := entryOrigin(r)
	if origin == "" {
		return identity.Session{}, identity.ErrSession
	}
	if values := r.Header.Values(resourceProofHeader); len(values) != 0 {
		if len(values) != 1 {
			return identity.Session{}, identity.ErrSession
		}
		proofSession, err := state.ValidateResourceProof(ctx, values[0], origin)
		if err != nil {
			return identity.Session{}, err
		}
		// A trusted native broker adds its own current dual-token pair. A late
		// renderer proof from another account/SID cannot override that identity.
		if r.Header.Get("token") != "" || r.Header.Get("chatToken") != "" {
			native, err := s.attachmentNativeSession(ctx, state, r)
			if err != nil || native.SessionID != proofSession.SessionID || native.UserID != proofSession.UserID || native.Epoch != proofSession.Epoch || native.Platform != proofSession.Platform || native.LogicalClient != proofSession.LogicalClient {
				return identity.Session{}, identity.ErrSession
			}
		}
		return proofSession, nil
	}
	if r.Header.Get("token") != "" || r.Header.Get("chatToken") != "" {
		return s.attachmentNativeSession(ctx, state, r)
	}
	var value string
	count := 0
	for _, cookie := range r.Cookies() {
		if cookie.Name == resourceCookie {
			count++
			value = cookie.Value
		}
	}
	if count != 1 {
		return identity.Session{}, identity.ErrSession
	}
	return state.ValidateResourceProof(ctx, value, origin)
}

func (s *Server) attachmentNativeSession(ctx context.Context, state *identity.Store, r *http.Request) (identity.Session, error) {
	if s.Auth == nil || len(r.Header.Values("token")) != 1 || len(r.Header.Values("chatToken")) != 1 || r.Header.Get("token") == "" || r.Header.Get("chatToken") == "" {
		return identity.Session{}, identity.ErrSession
	}
	a, err := s.Auth.Authenticate(ctx, r, false)
	if err != nil {
		return identity.Session{}, identity.ErrSession
	}
	v, err := state.Live(ctx, a.SessionID)
	if err != nil || v.UserID != a.UserID || v.Platform != a.Platform || v.Epoch != a.Epoch {
		return identity.Session{}, identity.ErrSession
	}
	return v, nil
}

func (s *Server) requireAttachmentReadSession(ctx context.Context, state *identity.Store, grant identity.Attachment, r *http.Request) (identity.Session, error) {
	v, err := s.attachmentReadSession(ctx, state, r)
	if err != nil || v.SessionID != grant.SessionID || v.UserID != grant.UserID || v.Epoch != grant.Epoch || v.Platform != grant.Platform || v.LogicalClient != grant.LogicalClient {
		return identity.Session{}, unauthorized("current attachment session proof required")
	}
	return v, nil
}
