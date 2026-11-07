package enterprise

import (
	"context"
	"net/http"
	"openim38.local/identity"
	"os"
	"strings"
)

// Only the local ingress chooses a profile; client Host/XFF values are never
// promoted into an origin, object-storage target or ICE address.
func entryOrigin(r *http.Request) string {
	return identity.EntryOrigin(r)
}
func envOrigin(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return strings.TrimRight(v, "/")
	}
	return fallback
}
func (s *Server) authenticateMediaActor(ctx context.Context, a Actor, r *http.Request) error {
	if !identity.Required() {
		_, e := s.Auth.Authenticate(ctx, r, false)
		return e
	}
	if a.SessionID == "" {
		return unauthorized("media session has no HRM binding")
	}
	state, e := identity.Global()
	if e != nil {
		return unavailable("identity registry unavailable")
	}
	v, e := state.Live(ctx, a.SessionID)
	if e != nil || v.UserID != a.UserID || v.Epoch != a.Epoch || v.Platform != a.Platform {
		return unauthorized("media session is expired or revoked")
	}
	if identity.CheckSessionAccess(ctx, v, a.IP, "connect", "") != nil {
		return forbidden("HRM media access denied")
	}
	return nil
}
