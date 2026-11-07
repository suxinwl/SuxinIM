package enterprise

import (
	"context"
	"errors"
	"net/http"

	"openim38.local/identity"
)

// This is a live, read-only decision for local encrypted-store unlocks. Neither
// a resource proof nor an ambient cookie replaces the native dual-token pair.
func (s *Server) sessionState(ctx context.Context, a Actor, r *http.Request) (any, error) {
	if !identity.Required() {
		return nil, unavailable("HRM session authority required")
	}
	if len(r.Header.Values("token")) != 1 || len(r.Header.Values("chatToken")) != 1 || r.Header.Get("token") == "" || r.Header.Get("chatToken") == "" {
		return nil, unauthorized("dual session tokens required")
	}
	state, err := identity.Global()
	if err != nil {
		return nil, unavailable("identity registry unavailable")
	}
	v, err := state.Pair(ctx, r.Header.Get("chatToken"), r.Header.Get("token"), a.UserID, a.Platform)
	if err != nil || v.SessionID != a.SessionID || v.Epoch != a.Epoch {
		return nil, unauthorized("HRM session is expired or revoked")
	}
	ip, err := identity.ClientIP(r)
	if err != nil {
		return nil, forbidden("client address is unavailable")
	}
	d, err := identity.CheckSessionDecision(ctx, v, ip, "connect", "")
	if errors.Is(err, identity.ErrAccessDenied) {
		return nil, forbidden("HRM access denied")
	}
	if err != nil {
		return nil, unavailable("current access policy unavailable")
	}
	// The complete resolver response must describe this exact session. An older
	// host missing the new fields cannot accidentally unlock a local vault.
	p := d.EffectivePolicy
	if !d.LoginAllowed || !p.Login || !p.Preview || p.LogicalClient != v.LogicalClient || d.LogicalClient != v.LogicalClient || p.SecurityVersion != v.Epoch || p.Zone != d.Zone || (p.Zone != "lan" && p.Zone != "internet") || p.Save != d.SaveAllowed || p.PolicyRevision != d.PolicyRevision || p.NetworkRevision != d.NetworkRevision {
		return nil, unavailable("current access policy incomplete")
	}
	return map[string]any{"userID": v.UserID, "sessionID": v.SessionID, "epoch": v.Epoch, "platform": v.Platform, "logicalClient": v.LogicalClient, "expiresAt": v.ExpiresAt, "loginAllowed": true, "saveAllowed": d.SaveAllowed, "effectivePolicy": p}, nil
}
