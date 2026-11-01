package chat

import (
	"context"
	"errors"
	"github.com/openimsdk/chat/pkg/common/db/database"
	chatdb "github.com/openimsdk/chat/pkg/common/db/table/chat"
	adminpb "github.com/openimsdk/chat/pkg/protocol/admin"
	chatpb "github.com/openimsdk/chat/pkg/protocol/chat"
	chatclient "github.com/openimsdk/chat/pkg/rpclient/chat"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"testing"
)

type resetDatabase struct {
	database.ChatDatabaseInterface
	updated string
}

func (d *resetDatabase) TakeCredentialByAccount(context.Context, string) (*chatdb.Credential, error) {
	return &chatdb.Credential{UserID: "verified-user"}, nil
}
func (d *resetDatabase) UpdatePasswordAndDeleteVerifyCode(_ context.Context, user, _, _ string) error {
	d.updated = user
	return nil
}

type resetAdmin struct {
	adminpb.AdminClient
	invalidated string
	fail        bool
}

func (a *resetAdmin) InvalidateToken(_ context.Context, req *adminpb.InvalidateTokenReq, _ ...grpc.CallOption) (*adminpb.InvalidateTokenResp, error) {
	a.invalidated = req.UserID
	if a.fail {
		return nil, errors.New("revocation unavailable")
	}
	return &adminpb.InvalidateTokenResp{}, nil
}

type resetTransport struct{ headers metadata.MD }

func (*resetTransport) Method() string { return "/openim.chat.chat/ResetPassword" }
func (t *resetTransport) SetHeader(m metadata.MD) error {
	t.headers = metadata.Join(t.headers, m)
	return nil
}
func (t *resetTransport) SendHeader(m metadata.MD) error { return t.SetHeader(m) }
func (*resetTransport) SetTrailer(metadata.MD) error     { return nil }
func TestVerifiedPasswordResetInvalidatesChatBeforeReturningIdentity(t *testing.T) {
	for _, fail := range []bool{false, true} {
		db := &resetDatabase{}
		admin := &resetAdmin{fail: fail}
		server := &chatSvr{Database: db, Admin: chatclient.NewAdminClient(admin), Code: verifyCode{SuperCode: "test-only"}}
		transport := &resetTransport{}
		ctx := grpc.NewContextWithServerTransportStream(context.Background(), transport)
		_, err := server.ResetPassword(ctx, &chatpb.ResetPasswordReq{Email: "fixture@openim38.test", Password: "new-digest", VerifyCode: "test-only"})
		if db.updated != "verified-user" || admin.invalidated != "verified-user" {
			t.Fatal("reset did not invalidate verified user's sessions")
		}
		if fail {
			if err == nil || len(transport.headers.Get("openim38-reset-user-id")) > 0 {
				t.Fatal("reported success before revocation")
			}
		} else if err != nil || transport.headers.Get("openim38-reset-user-id")[0] != "verified-user" {
			t.Fatalf("identity transfer failed: %v", err)
		}
	}
}
