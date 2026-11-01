package admin

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/openimsdk/chat/pkg/common/constant"
	"github.com/openimsdk/chat/pkg/common/db/database"
	"github.com/openimsdk/chat/pkg/common/tokenverify"
	pb "github.com/openimsdk/chat/pkg/protocol/admin"
	"github.com/openimsdk/tools/mw"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type exactChatDB struct {
	database.AdminDatabaseInterface
	tokens       map[string]map[string]int32
	fail         bool
	allDeletes   int
	exactDeletes int
}

func (d *exactChatDB) GetTokens(_ context.Context, user string) (map[string]int32, error) {
	return d.tokens[user], nil
}
func (d *exactChatDB) DeleteTokenValue(_ context.Context, user, token string) error {
	if d.fail {
		return errors.New("injected registry outage")
	}
	d.exactDeletes++
	delete(d.tokens[user], token)
	return nil
}
func (d *exactChatDB) DeleteToken(_ context.Context, user string) error {
	d.allDeletes++
	delete(d.tokens, user)
	return nil
}

func TestExactChatLogoutUsesRealInterceptorsAndPreservesNewAndOtherDeviceTokens(t *testing.T) {
	verifier := &tokenverify.Token{Secret: "synthetic-chat-exact-logout", Expires: time.Hour}
	issue := func(user string) string {
		v, _, e := verifier.CreateToken(user, tokenverify.TokenUser)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	old, newer, mobile := issue("alice"), issue("alice"), issue("alice")
	if old == newer || old == mobile || newer == mobile {
		t.Fatal("same-second login credentials collide")
	}
	db := &exactChatDB{tokens: map[string]map[string]int32{"alice": {old: 0, newer: 0, mobile: 0}}}
	server := grpc.NewServer(mw.GrpcServer())
	pb.RegisterAdminServer(server, &adminServer{Database: db, Token: verifier})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithNoProxy(), mw.GrpcClient())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := pb.NewAdminClient(conn)
	base := context.WithValue(context.Background(), "operationID", "test-exact-chat-logout")
	// The real tools interceptor rebuilds outgoing metadata from these values.
	ctx := context.WithValue(base, constant.RpcCustomHeader, []string{"openim38-chat-revoke-token"})
	ctx = context.WithValue(ctx, "openim38-chat-revoke-token", []string{old})
	db.fail = true
	if _, err := client.InvalidateToken(ctx, &pb.InvalidateTokenReq{UserID: "alice"}); err == nil {
		t.Fatal("registry failure swallowed")
	}
	db.fail = false
	for i := 0; i < 2; i++ {
		if _, err := client.InvalidateToken(ctx, &pb.InvalidateTokenReq{UserID: "alice"}); err != nil {
			t.Fatal(err)
		}
	}
	if db.allDeletes != 0 || db.exactDeletes != 2 {
		t.Fatal("exact revocation used the all-user operation")
	}
	if _, err := client.ParseToken(base, &pb.ParseTokenReq{Token: old}); err == nil {
		t.Fatal("logged-out Chat token still authenticates")
	}
	for _, value := range []string{newer, mobile} {
		if _, err := client.ParseToken(base, &pb.ParseTokenReq{Token: value}); err != nil {
			t.Fatal("new login or other device was revoked")
		}
	}
	if _, err := client.InvalidateToken(ctx, &pb.InvalidateTokenReq{UserID: "bob"}); err == nil {
		t.Fatal("credential revoked against another user's registry")
	}
	// Original password-change/ban behavior remains a full-user invalidation.
	if _, err := client.InvalidateToken(base, &pb.InvalidateTokenReq{UserID: "alice"}); err != nil {
		t.Fatal(err)
	}
	if db.allDeletes != 1 {
		t.Fatal("full-user invalidation changed")
	}
	if _, err := client.ParseToken(base, &pb.ParseTokenReq{Token: newer}); err == nil {
		t.Fatal("full-user invalidation no longer works")
	}
}

func TestExactChatLogoutRejectsAmbiguousMetadata(t *testing.T) {
	verifier := &tokenverify.Token{Secret: "synthetic-chat-exact-logout", Expires: time.Hour}
	db := &exactChatDB{tokens: map[string]map[string]int32{}}
	server := &adminServer{Database: db, Token: verifier}
	for _, values := range [][]string{{""}, {"a", "b"}, {"malformed"}} {
		ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{"openim38-chat-revoke-token": values})
		if _, err := server.InvalidateToken(ctx, &pb.InvalidateTokenReq{UserID: "alice"}); err == nil {
			t.Fatal("invalid exact-revocation metadata accepted")
		}
	}
	if db.allDeletes != 0 || db.exactDeletes != 0 {
		t.Fatal("malformed exact request modified registry")
	}
}
