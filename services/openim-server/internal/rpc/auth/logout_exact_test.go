package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/openimsdk/open-im-server/v3/pkg/common/storage/controller"
	pbauth "github.com/openimsdk/protocol/auth"
	"github.com/openimsdk/protocol/constant"
	"github.com/openimsdk/protocol/msggateway"
	"github.com/openimsdk/tools/discovery"
	"github.com/openimsdk/tools/mw"
	"github.com/openimsdk/tools/tokenverify"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type exactLogoutDB struct {
	controller.AuthDatabase
	tokens []string
}

func (d *exactLogoutDB) BatchSetTokenMapByUidPid(_ context.Context, tokens []string) error {
	d.tokens = append(d.tokens, tokens...)
	return nil
}

type exactLogoutConn struct {
	tokens []string
	err    error
}

func (c *exactLogoutConn) Invoke(ctx context.Context, method string, args, reply any, opts ...grpc.CallOption) error {
	// Use the real fixed-version interceptor: it reconstructs metadata and was
	// the important compatibility trap when adding the exact-token constraint.
	return mw.RpcClientInterceptor(ctx, method, args, reply, &grpc.ClientConn{}, func(ctx context.Context, method string, req, reply any, _ *grpc.ClientConn, options ...grpc.CallOption) error {
		return c.capture(ctx, method, req)
	}, opts...)
}

func (c *exactLogoutConn) capture(ctx context.Context, method string, args any) error {
	if method != "/openim.msggateway.msgGateway/KickUserOffline" {
		return errors.New("unexpected gateway method")
	}
	request, ok := args.(*msggateway.KickUserOfflineReq)
	if !ok || request.PlatformID != 5 || len(request.KickUserIDList) != 1 || request.KickUserIDList[0] != "alice" {
		return errors.New("incorrect revoked-token identity")
	}
	md, _ := metadata.FromOutgoingContext(ctx)
	values := md.Get("openim38-revoke-token")
	if len(values) != 1 {
		return errors.New("exact-token constraint missing")
	}
	registered, _ := ctx.Value(constant.RpcCustomHeader).([]string)
	registeredExact := false
	for _, key := range registered {
		if key == "openim38-revoke-token" {
			registeredExact = true
		}
	}
	contextTokens, _ := ctx.Value("openim38-revoke-token").([]string)
	if !registeredExact || len(contextTokens) != 1 || contextTokens[0] != values[0] {
		return errors.New("OpenIM context interceptor would drop exact-token metadata")
	}
	c.tokens = append(c.tokens, values[0])
	return c.err
}
func (*exactLogoutConn) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, errors.New("unexpected stream")
}

type exactLogoutRegistry struct {
	discovery.SvcDiscoveryRegistry
	conn grpc.ClientConnInterface
}

func (r *exactLogoutRegistry) GetConns(context.Context, string, ...grpc.DialOption) ([]grpc.ClientConnInterface, error) {
	return []grpc.ClientConnInterface{r.conn}, nil
}

func TestKickTokensMarksOnlyRequestedTokenAndClosesExactTransport(t *testing.T) {
	const secret = "openim38-test-exact-revocation-secret"
	old, err := jwt.NewWithClaims(jwt.SigningMethodHS256, tokenverify.BuildClaims("alice", 5, 1)).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	db := &exactLogoutDB{}
	conn := &exactLogoutConn{}
	config := &Config{}
	config.Share.Secret = secret
	config.Share.RpcRegisterName.MessageGateway = "gateway"
	s := &authServer{authDatabase: db, RegisterCenter: &exactLogoutRegistry{conn: conn}, config: config}
	ctx := context.WithValue(context.Background(), constant.OperationID, "test-exact-logout")
	if _, err := s.KickTokens(ctx, &pbauth.KickTokensReq{Tokens: []string{old}}); err != nil {
		t.Fatal(err)
	}
	if len(db.tokens) != 1 || db.tokens[0] != old || len(conn.tokens) != 1 || conn.tokens[0] != old {
		t.Fatal("single-token revocation expanded to other login credentials")
	}
	// The durable enterprise queue must retry if a live gateway cannot be reached.
	conn.err = errors.New("gateway unavailable")
	if _, err := s.KickTokens(ctx, &pbauth.KickTokensReq{Tokens: []string{old}}); err == nil {
		t.Fatal("gateway failure swallowed after Redis invalidation")
	}
}
