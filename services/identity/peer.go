package identity

import (
	"context"
	"net"
	"strings"
)

// These values are set by the authenticated gateway, never by MsgData or a
// browser-supplied header. The locked RPC interceptor transports customHeader
// values as private metadata and restores the same context keys server-side.
const clientSessionKey = "openim38-hrm-client-session"
const clientAddressKey = "openim38-hrm-client-address"
const customHeaderKey = "customHeader"

func Employee(user string) bool { return strings.HasPrefix(user, "emp_") && len(user) > len("emp_") }

func WithClientSession(ctx context.Context, session Session, ip string) context.Context {
	ctx = WithEntryZone(ctx, EntryZoneFromContext(ctx))
	keys, _ := ctx.Value(customHeaderKey).([]string)
	keys = append([]string(nil), keys...)
	for _, key := range []string{clientSessionKey, clientAddressKey} {
		found := false
		for _, previous := range keys {
			found = found || previous == key
		}
		if !found {
			keys = append(keys, key)
		}
	}
	ctx = context.WithValue(ctx, customHeaderKey, keys)
	ctx = context.WithValue(ctx, clientSessionKey, []string{session.SessionID})
	return context.WithValue(ctx, clientAddressKey, []string{ip})
}

func (s *Store) AuthorizePeer(ctx context.Context, user, peer string) error {
	if !Required() || !Employee(user) || !Employee(peer) || user == peer {
		return ErrSession
	}
	sid, sidOK := ctx.Value(clientSessionKey).([]string)
	address, addressOK := ctx.Value(clientAddressKey).([]string)
	if !sidOK || len(sid) != 1 || len(sid[0]) < 16 || len(sid[0]) > 128 || !addressOK || len(address) != 1 || net.ParseIP(address[0]) == nil {
		return ErrSession
	}
	session, err := s.Live(ctx, sid[0])
	if err != nil || session.UserID != user || !Platform(session.Platform) {
		return ErrSession
	}
	for _, id := range []string{user, peer} {
		projection, err := s.Projection(ctx, id)
		if err != nil || projection.UserID != id || !projection.Enabled || projection.Version < 1 {
			return ErrSession
		}
	}
	// HRM checks both current eligibility and organization scope. The recipient
	// need not be online; their device/network policy is not the sender's policy.
	return CheckSessionAccess(ctx, session, address[0], "direct_chat", peer)
}

func AuthorizePeer(ctx context.Context, user, peer string) error {
	state, err := Global()
	if err != nil {
		return ErrSession
	}
	return state.AuthorizePeer(ctx, user, peer)
}
