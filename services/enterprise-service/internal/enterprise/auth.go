package enterprise

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"openim38.local/identity"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
)

type Actor struct {
	UserID             string
	Platform           int
	Admin              bool
	IMToken, ChatToken string
	IP                 string
	ExpiresAt          int64
	LogicalClient      string
	SessionID          string
	Epoch              int64
	EntryOrigin        string
	EntryZone          string
}
type Authenticator interface {
	Authenticate(context.Context, *http.Request, bool) (Actor, error)
}

// Only the two small, versioned ParseToken wire contracts cross module boundaries.
// This avoids importing Chat's older protocol module into the IM module graph.
// Both upstream methods check their Redis token registry, not just JWT expiry.
type GRPCAuth struct{ chat, im *grpc.ClientConn }
type wireCodec struct{}

func (wireCodec) Name() string { return "proto" }
func (wireCodec) Marshal(v any) ([]byte, error) {
	p, ok := v.(*[]byte)
	if !ok {
		return nil, errors.New("expected protobuf wire bytes")
	}
	return *p, nil
}
func (wireCodec) Unmarshal(b []byte, v any) error {
	p, ok := v.(*[]byte)
	if !ok {
		return errors.New("expected protobuf wire bytes")
	}
	*p = append((*p)[:0], b...)
	return nil
}
func NewGRPCAuth(chat, im string) (*GRPCAuth, error) {
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithNoProxy(), grpc.WithDefaultCallOptions(grpc.ForceCodec(wireCodec{})), grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, conn *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		md, _ := metadata.FromOutgoingContext(ctx)
		md = md.Copy()
		if len(md.Get("operationID")) != 1 {
			md.Set("operationID", newID())
		}
		return invoke(metadata.NewOutgoingContext(ctx, md), method, req, reply, conn, opts...)
	})}
	c, e := grpc.NewClient(chat, opts...)
	if e != nil {
		return nil, e
	}
	i, e := grpc.NewClient(im, opts...)
	if e != nil {
		c.Close()
		return nil, e
	}
	return &GRPCAuth{chat: c, im: i}, nil
}
func (a *GRPCAuth) Close() { a.chat.Close(); a.im.Close() }
func fieldString(b []byte, n protowire.Number, s string) []byte {
	return protowire.AppendString(protowire.AppendTag(b, n, protowire.BytesType), s)
}
func fieldInt(b []byte, n protowire.Number, x int64) []byte {
	return protowire.AppendVarint(protowire.AppendTag(b, n, protowire.VarintType), uint64(x))
}
func wireFields(b []byte) (map[protowire.Number][][]byte, error) {
	out := map[protowire.Number][][]byte{}
	for len(b) > 0 {
		n, t, k := protowire.ConsumeTag(b)
		if k < 0 {
			return nil, protowire.ParseError(k)
		}
		b = b[k:]
		switch t {
		case protowire.BytesType:
			v, k := protowire.ConsumeBytes(b)
			if k < 0 {
				return nil, protowire.ParseError(k)
			}
			out[n] = append(out[n], append([]byte(nil), v...))
			b = b[k:]
		case protowire.VarintType:
			v, k := protowire.ConsumeVarint(b)
			if k < 0 {
				return nil, protowire.ParseError(k)
			}
			out[n] = append(out[n], protowire.AppendVarint(nil, v))
			b = b[k:]
		default:
			k := protowire.ConsumeFieldValue(n, t, b)
			if k < 0 {
				return nil, protowire.ParseError(k)
			}
			b = b[k:]
		}
	}
	return out, nil
}
func strField(f map[protowire.Number][][]byte, n protowire.Number) string {
	if len(f[n]) == 0 {
		return ""
	}
	return string(f[n][0])
}
func intField(f map[protowire.Number][][]byte, n protowire.Number) int64 {
	if len(f[n]) == 0 {
		return 0
	}
	v, _ := protowire.ConsumeVarint(f[n][0])
	return int64(v)
}
func (a *GRPCAuth) Authenticate(ctx context.Context, r *http.Request, adminOnly bool) (Actor, error) {
	ctx = identity.WithRequestEntry(ctx, r)
	actor := Actor{IMToken: r.Header.Get("token"), ChatToken: r.Header.Get("chatToken")}
	ip := r.Header.Get("X-Real-IP")
	if net.ParseIP(ip) == nil {
		ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	actor.IP = ip
	if identity.Required() {
		trustedIP, e := identity.ClientIP(r)
		if e != nil {
			return actor, unauthorized("client address is unavailable")
		}
		actor.IP = trustedIP
	}
	if actor.ChatToken == "" && adminOnly {
		actor.ChatToken = r.Header.Get("token")
	}
	if actor.ChatToken == "" {
		return actor, unauthorized("Chat token required")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	in := fieldString(nil, 1, actor.ChatToken)
	var out []byte
	if err := a.chat.Invoke(ctx, "/openim.admin.admin/ParseToken", &in, &out); err != nil {
		return actor, authRPCError(err, "Chat session expired or revoked")
	}
	f, err := wireFields(out)
	if err != nil {
		return actor, err
	}
	actor.UserID = strField(f, 1)
	actor.Admin = intField(f, 2) == 2
	if actor.UserID == "" {
		return actor, unauthorized("invalid Chat identity")
	}
	if adminOnly {
		if !actor.Admin {
			return actor, forbidden("administrator required")
		}
		return actor, nil
	}
	in = fieldString(nil, 1, actor.IP)
	in = fieldString(in, 2, actor.UserID)
	out = nil
	if err = a.chat.Invoke(ctx, "/openim.admin.admin/CheckLoginForbidden", &in, &out); err != nil {
		return actor, authRPCError(err, "account access is blocked")
	}
	if actor.IMToken == "" {
		return actor, unauthorized("OpenIM token required")
	}
	in = fieldString(nil, 1, actor.IMToken)
	out = nil
	if err = a.im.Invoke(ctx, "/openim.auth.Auth/parseToken", &in, &out); err != nil {
		return actor, authRPCError(err, "OpenIM session expired or revoked")
	}
	f, err = wireFields(out)
	if err != nil {
		return actor, err
	}
	if strField(f, 1) != actor.UserID {
		return actor, unauthorized("Chat and OpenIM identities differ")
	}
	actor.Platform = int(intField(f, 2))
	actor.ExpiresAt = intField(f, 4)
	if p := r.Header.Get("platformID"); p != "" && p != strconv.Itoa(actor.Platform) {
		return actor, unauthorized("platform does not match token")
	}
	if actor.Platform == 10 {
		return actor, forbidden("management IM tokens cannot be used as client sessions")
	}
	if identity.Required() {
		state, e := identity.Global()
		if e != nil {
			return actor, unavailable("identity registry unavailable")
		}
		session, e := state.Pair(ctx, actor.ChatToken, actor.IMToken, actor.UserID, actor.Platform)
		if e != nil {
			return actor, unauthorized("HRM session is expired or revoked")
		}
		actor.SessionID = session.SessionID
		actor.Epoch = session.Epoch
		actor.ExpiresAt = session.ExpiresAt
		actor.LogicalClient = session.LogicalClient
		actor.EntryOrigin = entryOrigin(r)
		if e = identity.CheckSessionAccess(ctx, session, actor.IP, "connect", ""); e != nil {
			if errors.Is(e, identity.ErrAccessDenied) {
				return actor, forbidden("HRM access denied")
			}
			return actor, unavailable("current access policy unavailable")
		}
	}
	return actor, nil
}
func authRPCError(err error, message string) error {
	slog.Warn("upstream authentication RPC rejected", "code", status.Code(err).String(), "reason", status.Convert(err).Message())
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.ResourceExhausted, codes.Canceled, codes.Internal:
		return &apiError{503, 1006, "authentication service temporarily unavailable"}
	default:
		return unauthorized(message)
	}
}
func actorMetadata(ctx context.Context, a Actor) context.Context {
	return metadata.NewOutgoingContext(ctx, metadata.Pairs("opuserid", a.UserID, "customHeader", "opUserType", "opUserType", strconv.Itoa(func() int {
		if a.Admin {
			return 2
		}
		return 1
	}()), "operationid", newID()))
}

type apiError struct {
	Status, Code int
	Message      string
}

func (e *apiError) Error() string { return e.Message }
func bad(s string) error          { return &apiError{400, 1001, s} }
func unauthorized(s string) error { return &apiError{401, 1501, s} }
func forbidden(s string) error    { return &apiError{403, 1002, s} }
func missing(s string) error      { return &apiError{404, 1004, s} }
func conflict(s string) error     { return &apiError{409, 1005, s} }
func checkActor(a Actor, requested string) error {
	if requested != "" && requested != a.UserID {
		return forbidden(fmt.Sprintf("operation identity must be %s", a.UserID))
	}
	return nil
}
