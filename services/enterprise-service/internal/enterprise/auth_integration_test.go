package enterprise

import (
	"context"
	"errors"
	"net"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
)

type wireRPC struct {
	mu                sync.Mutex
	active            bool
	user              string
	userType          int64
	lastPlatform      string
	latest            Application
	added             bool
	revokedTokens     []string
	revokedChatTokens []string
}

func startWireRPC(t *testing.T, service string, state *wireRPC) (string, *grpc.Server) {
	t.Helper()
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	server := grpc.NewServer(grpc.ForceServerCodec(wireCodec{}))
	names := []string{"ParseToken", "CheckLoginForbidden", "LatestApplicationVersion", "AddApplicationVersion", "InvalidateToken"}
	if service == "openim.auth.Auth" {
		names = []string{"parseToken", "kickTokens"}
	}
	methods := []grpc.MethodDesc{}
	for _, methodName := range names {
		name := methodName
		methods = append(methods, grpc.MethodDesc{MethodName: name, Handler: func(_ any, ctx context.Context, decode func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
			var in []byte
			md, _ := metadata.FromIncomingContext(ctx)
			if len(md.Get("operationID")) != 1 {
				return nil, status.Error(codes.InvalidArgument, "operationID required by upstream middleware")
			}
			if e := decode(&in); e != nil {
				return nil, e
			}
			state.mu.Lock()
			defer state.mu.Unlock()
			f, e := wireFields(in)
			if e != nil {
				return nil, e
			}
			var out []byte
			switch name {
			case "InvalidateToken":
				if strField(f, 1) != state.user || len(md.Get("openim38-chat-revoke-token")) != 1 || len(md.Get("customHeader")) != 1 || md.Get("customHeader")[0] != "openim38-chat-revoke-token" {
					return nil, status.Error(codes.InvalidArgument, "exact Chat revocation metadata missing")
				}
				state.revokedChatTokens = append(state.revokedChatTokens, md.Get("openim38-chat-revoke-token")[0])
			case "kickTokens":
				for _, token := range f[1] {
					state.revokedTokens = append(state.revokedTokens, string(token))
				}
			case "ParseToken", "parseToken":
				if !state.active || strField(f, 1) == "" {
					return nil, status.Error(codes.Unauthenticated, "revoked")
				}
				out = fieldString(nil, 1, state.user)
				if name == "ParseToken" {
					out = fieldInt(out, 2, state.userType)
				} else {
					out = fieldInt(out, 2, 5)
					out = fieldInt(out, 4, time.Now().Add(time.Hour).Unix())
				}
			case "LatestApplicationVersion":
				if state.latest.Version != "" {
					v := state.latest
					msg := fieldString(nil, 1, "id")
					msg = fieldString(msg, 2, v.Platform)
					msg = fieldString(msg, 3, v.Version)
					msg = fieldString(msg, 4, v.URL)
					msg = fieldString(msg, 5, v.Text)
					if v.Force {
						msg = fieldInt(msg, 6, 1)
					}
					msg = fieldInt(msg, 7, 1)
					out = protowire.AppendBytes(protowire.AppendTag(nil, 1, protowire.BytesType), msg)
				}
			case "AddApplicationVersion":
				md, _ := metadata.FromIncomingContext(ctx)
				if len(md.Get("opuserid")) != 1 || len(md.Get("opusertype")) != 1 || md.Get("opusertype")[0] != "2" || len(md.Get("customHeader")) != 1 || md.Get("customHeader")[0] != "opUserType" {
					return nil, status.Error(codes.PermissionDenied, "admin metadata absent")
				}
				state.added = true
				state.latest = Application{Platform: strField(f, 1), Version: strField(f, 2), URL: strField(f, 3), Text: strField(f, 4), Force: intField(f, 5) == 1, Latest: true}
			}
			return &out, nil
		}})
	}
	server.RegisterService(&grpc.ServiceDesc{ServiceName: service, HandlerType: (*interface{})(nil), Methods: methods}, state)
	go server.Serve(ln)
	t.Cleanup(server.Stop)
	return ln.Addr().String(), server
}

func TestGRPCExactRevokeWireContainsOnlyRequestedToken(t *testing.T) {
	state := &wireRPC{active: true, user: "alice"}
	addr, _ := startWireRPC(t, "openim.auth.Auth", state)
	a, err := NewGRPCAuth(addr, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := a.RevokeIMToken(context.Background(), "old-session-token"); err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.revokedTokens) != 1 || state.revokedTokens[0] != "old-session-token" {
		t.Fatal("revocation did not use fixed protocol's exact token list")
	}
}

func TestGRPCExactChatRevokeWirePreservesPrivateMetadata(t *testing.T) {
	state := &wireRPC{active: true, user: "alice"}
	addr, _ := startWireRPC(t, "openim.admin.admin", state)
	a, err := NewGRPCAuth(addr, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := a.RevokeChatToken(context.Background(), "alice", "old-chat-only"); err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.revokedChatTokens) != 1 || state.revokedChatTokens[0] != "old-chat-only" {
		t.Fatal("exact Chat credential was not sent")
	}
}
func TestGRPCWireAuthenticationChecksBothActiveRegistriesAndIdentity(t *testing.T) {
	chatState := &wireRPC{active: true, user: "alice", userType: 1}
	imState := &wireRPC{active: true, user: "alice"}
	chatAddr, chatServer := startWireRPC(t, "openim.admin.admin", chatState)
	imAddr, _ := startWireRPC(t, "openim.auth.Auth", imState)
	a, e := NewGRPCAuth(chatAddr, imAddr)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	r := httptest.NewRequest("POST", "http://enterprise/test", nil)
	r.Header.Set("chatToken", "chat-token")
	r.Header.Set("token", "im-token")
	r.Header.Set("platformID", "5")
	ctx := context.Background()
	if actor, e := a.Authenticate(ctx, r, false); e != nil || actor.UserID != "alice" || actor.Platform != 5 {
		t.Fatalf("parse wire mismatch: %+v %v", actor, e)
	}
	imState.mu.Lock()
	imState.active = false
	imState.mu.Unlock()
	if _, e = a.Authenticate(ctx, r, false); e == nil {
		t.Fatal("revoked IM token accepted")
	}
	imState.mu.Lock()
	imState.active = true
	imState.user = "bob"
	imState.mu.Unlock()
	if _, e = a.Authenticate(ctx, r, false); e == nil {
		t.Fatal("mismatched identities accepted")
	}
	imState.mu.Lock()
	imState.user = "alice"
	imState.mu.Unlock()
	chatState.mu.Lock()
	chatState.active = false
	chatState.mu.Unlock()
	if _, e = a.Authenticate(ctx, r, false); e == nil {
		t.Fatal("revoked Chat token accepted")
	}
	chatState.mu.Lock()
	chatState.active = true
	chatState.userType = 2
	chatState.mu.Unlock()
	r.Header.Del("token")
	if actor, e := a.Authenticate(ctx, r, true); e != nil || !actor.Admin {
		t.Fatalf("admin-only auth mismatch: %+v %v", actor, e)
	}
	chatServer.Stop()
	_, e = a.Authenticate(ctx, r, true)
	var ae *apiError
	if !errors.As(e, &ae) || ae.Status != 503 {
		t.Fatalf("outage treated as revocation: %v", e)
	}
}
func TestGRPCCatalogUsesOfficialWireAndAdminContext(t *testing.T) {
	state := &wireRPC{active: true, user: "admin", userType: 2}
	addr, _ := startWireRPC(t, "openim.admin.admin", state)
	a, e := NewGRPCAuth(addr, addr)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	c := &GRPCCatalog{Auth: a}
	if _, e = c.Latest(context.Background(), "android"); !errors.Is(e, ErrNotFound) {
		t.Fatalf("empty catalog: %v", e)
	}
	v := Application{Platform: "android", Version: "2.0.0", URL: "https://test.invalid/a.apk", Text: "release", Force: true}
	if e = c.Add(context.Background(), actor("admin"), v); e != nil {
		t.Fatal(e)
	}
	got, e := c.Latest(context.Background(), "android")
	if e != nil || !got.Force || got.Version != v.Version || got.URL != v.URL || !state.added {
		t.Fatalf("wire catalog mismatch: %+v %v", got, e)
	}
}
func TestMongoCASPersistsAcrossClients(t *testing.T) {
	uri := os.Getenv("ENTERPRISE_TEST_MONGO_URI")
	if uri == "" {
		t.Skip("set ENTERPRISE_TEST_MONGO_URI to run isolated MongoDB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, e := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if e != nil {
		t.Fatal(e)
	}
	defer client.Disconnect(context.Background())
	name := "enterprise_test_" + newID()
	if !strings.HasPrefix(name, "enterprise_test_") {
		t.Fatal("unsafe test database")
	}
	defer client.Database(name).Drop(context.Background())
	store, e := NewMongoStore(ctx, client, name)
	if e != nil {
		t.Fatal(e)
	}
	d := document("one", "test", "alice", map[string]int{"count": 1})
	if e = store.Save(ctx, d, 0); e != nil {
		t.Fatal(e)
	}
	if e = store.Save(ctx, d, 0); !errors.Is(e, ErrConflict) {
		t.Fatal("duplicate insert succeeded")
	}
	got, e := store.Get(ctx, "one")
	if e != nil || got.Version != 1 {
		t.Fatalf("persist: %+v %v", got, e)
	}
	if e = store.Save(ctx, got, got.Version); e != nil {
		t.Fatal(e)
	}
	if e = store.Save(ctx, got, got.Version); !errors.Is(e, ErrConflict) {
		t.Fatal("stale replace succeeded")
	}
	second, e := NewMongoStore(ctx, client, name)
	if e != nil {
		t.Fatal(e)
	}
	got, e = second.Get(ctx, "one")
	if e != nil || got.Version != 2 {
		t.Fatalf("restart persistence: %+v %v", got, e)
	}
}
