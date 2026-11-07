package group

import (
	"context"
	"testing"

	"github.com/openimsdk/open-im-server/v3/pkg/common/config"
	pb "github.com/openimsdk/protocol/group"
	"github.com/openimsdk/tools/mcontext"
)

func TestHRMDepartmentMembershipCannotBeChangedBySDK(t *testing.T) {
	t.Setenv("HRM_SSO_REQUIRED", "true")
	s := &groupServer{config: &Config{Share: config.Share{IMAdminUserID: []string{"imAdmin"}}}}
	ctx := mcontext.SetOpUserID(context.Background(), "emp_fixture")
	if _, err := s.JoinGroup(ctx, &pb.JoinGroupReq{GroupID: "dept_fixture"}); err == nil {
		t.Fatal("direct join bypassed HRM membership")
	}
	if _, err := s.QuitGroup(ctx, &pb.QuitGroupReq{GroupID: "dept_fixture"}); err == nil {
		t.Fatal("direct quit bypassed HRM membership")
	}
	if err := s.guardHRMGroup(ctx, "ordinary-working-group"); err != nil {
		t.Fatal("ordinary chat group blocked", err)
	}
	admin := mcontext.SetOpUserID(context.Background(), "imAdmin")
	if err := s.guardHRMGroup(admin, "dept_fixture"); err != nil {
		t.Fatal("trusted HRM reconciliation blocked", err)
	}
}
