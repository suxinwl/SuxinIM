package user

import (
	"context"
	"github.com/openimsdk/open-im-server/v3/pkg/common/config"
	pb "github.com/openimsdk/protocol/user"
	"github.com/openimsdk/tools/mcontext"
	"testing"
)

func TestHRMDirectSDKCannotChangeEmployeeIdentity(t *testing.T) {
	t.Setenv("HRM_SSO_REQUIRED", "true")
	s := &userServer{config: &Config{Share: config.Share{IMAdminUserID: []string{"imAdmin"}}}}
	ctx := mcontext.SetOpUserID(context.Background(), "emp_fixture")
	if _, e := s.UpdateUserInfo(ctx, &pb.UpdateUserInfoReq{}); e == nil {
		t.Fatal("legacy direct user update allowed")
	}
	if _, e := s.UpdateUserInfoEx(ctx, &pb.UpdateUserInfoExReq{}); e == nil {
		t.Fatal("direct extended update allowed")
	}
}
