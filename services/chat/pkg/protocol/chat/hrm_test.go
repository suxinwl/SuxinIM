package chat

import (
	"github.com/openimsdk/tools/checker"
	"testing"
)

func TestHRMAdminProvisioningAcceptsShadowAccountWithoutInventedContact(t *testing.T) {
	req := &AddUserAccountReq{User: &RegisterUserInfo{UserID: "emp_fixture", Account: "hrm0123456789abcdef"}}
	t.Setenv("HRM_SSO_REQUIRED", "true")
	if err := req.Check(); err != nil {
		t.Fatal("HRM account-only projection rejected by HTTP DTO", err)
	}
	registration := &RegisterUserReq{User: &RegisterUserInfo{UserID: "emp_fixture", Account: "hrm0123456789abcdef", Nickname: "Fixture"}, Platform: 10}
	if err := checker.Validate(registration); err != nil {
		t.Fatal("real gRPC checker rejected admin HRM account-only projection", err)
	}
	registration.Platform = 5
	if err := checker.Validate(registration); err == nil {
		t.Fatal("client platform bypassed registration contact rules")
	}
	req.User.UserID = "ordinary-user"
	if err := req.Check(); err == nil {
		t.Fatal("unmanaged account bypassed contact requirements")
	}
	req.User.UserID = "emp_fixture"
	t.Setenv("HRM_SSO_REQUIRED", "false")
	if err := req.Check(); err == nil {
		t.Fatal("standalone registration policy changed")
	}
}
