package chat

import (
	"context"
	"github.com/openimsdk/chat/pkg/common/constant"
	"github.com/openimsdk/chat/pkg/common/mctx"
	pb "github.com/openimsdk/chat/pkg/protocol/chat"
	"github.com/openimsdk/protocol/wrapperspb"
	"testing"
)

func TestHRMDisablesIndependentCredentialsAndDirectProfileRPC(t *testing.T) {
	t.Setenv("HRM_SSO_REQUIRED", "true")
	s := &chatSvr{}
	ctx := context.Background()
	if _, e := s.Login(ctx, &pb.LoginReq{}); e == nil {
		t.Fatal("independent login allowed")
	}
	if _, e := s.ResetPassword(ctx, &pb.ResetPasswordReq{}); e == nil {
		t.Fatal("independent reset allowed")
	}
	if _, e := s.ChangePassword(ctx, &pb.ChangePasswordReq{}); e == nil {
		t.Fatal("independent password change allowed")
	}
	user := mctx.WithOpUserID(ctx, "emp_fixture", constant.NormalUser)
	for _, req := range []*pb.UpdateUserInfoReq{{UserID: "emp_fixture", Nickname: &wrapperspb.StringValue{Value: "changed"}}, {UserID: "emp_fixture", FaceURL: &wrapperspb.StringValue{Value: "https://untrusted.invalid/avatar"}}, {UserID: "emp_fixture", Account: &wrapperspb.StringValue{Value: "newaccount"}}} {
		if _, e := s.UpdateUserInfo(user, req); e == nil {
			t.Fatal("direct profile mutation accepted")
		}
	}
	if _, e := s.RegisterUser(user, &pb.RegisterUserReq{}); e == nil {
		t.Fatal("ordinary account registration allowed")
	}
	if _, e := s.AddUserAccount(user, &pb.AddUserAccountReq{}); e == nil {
		t.Fatal("ordinary direct account provisioning allowed")
	}
}
