package group

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/openimsdk/open-im-server/v3/pkg/common/config"
	"github.com/openimsdk/open-im-server/v3/pkg/common/storage/common"
	"github.com/openimsdk/open-im-server/v3/pkg/common/storage/controller"
	"github.com/openimsdk/open-im-server/v3/pkg/common/storage/model"
	pb "github.com/openimsdk/protocol/group"
	"github.com/openimsdk/protocol/wrapperspb"
	"github.com/openimsdk/tools/mcontext"
)

type roleFixtureDB struct {
	controller.GroupDatabase
	members map[string]map[string]int32
	updates []*common.BatchUpdateGroupMember
}

func (f *roleFixtureDB) FindGroupMembers(_ context.Context, group string, users []string) ([]*model.GroupMember, error) {
	var result []*model.GroupMember
	for _, user := range users {
		if role, ok := f.members[group][user]; ok {
			result = append(result, &model.GroupMember{GroupID: group, UserID: user, RoleLevel: role})
		}
	}
	return result, nil
}

func (f *roleFixtureDB) UpdateGroupMembers(_ context.Context, rows []*common.BatchUpdateGroupMember) error {
	f.updates = rows
	for _, row := range rows {
		if role, ok := row.Map["role_level"].(int32); ok {
			f.members[row.GroupID][row.UserID] = role
		}
	}
	return nil
}

func (f *roleFixtureDB) TakeGroup(context.Context, string) (*model.Group, error) {
	// The notification is best effort and unrelated to permission enforcement.
	return nil, errors.New("synthetic notification lookup unavailable")
}

func roleServer() (*groupServer, *roleFixtureDB) {
	f := &roleFixtureDB{members: map[string]map[string]int32{
		"dept_fixture":   {"emp_owner": 100, "emp_admin": 60, "emp_member": 20},
		"normal_fixture": {"emp_owner": 100, "emp_admin": 60, "emp_member": 20},
	}}
	c := &Config{Share: config.Share{IMAdminUserID: []string{"imAdmin"}}}
	return &groupServer{db: f, config: c, notification: &NotificationSender{db: f, config: c}}, f
}

func roleRequest(group, user string, role int32) *pb.SetGroupMemberInfoReq {
	return &pb.SetGroupMemberInfoReq{Members: []*pb.SetGroupMemberInfo{{GroupID: group, UserID: user, RoleLevel: wrapperspb.Int32(role)}}}
}

func TestRoleScalarJSONMatchesPinnedSDKContract(t *testing.T) {
	var request pb.SetGroupMemberInfoReq
	if err := json.Unmarshal([]byte(`{"members":[{"groupID":"dept_fixture","userID":"emp_member","roleLevel":60}]}`), &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Members) != 1 || request.Members[0].RoleLevel.GetValue() != 60 {
		t.Fatal("SDK scalar role was not decoded")
	}
}

func TestDepartmentOwnerCanAssignAndRemoveAdministrator(t *testing.T) {
	t.Setenv("HRM_SSO_REQUIRED", "true")
	s, f := roleServer()
	ctx := mcontext.SetOpUserID(context.Background(), "emp_owner")
	for _, role := range []int32{60, 20} {
		if _, err := s.SetGroupMemberInfo(ctx, roleRequest("dept_fixture", "emp_member", role)); err != nil {
			t.Fatal("current department owner role change rejected", err)
		}
		if len(f.updates) != 1 || f.members["dept_fixture"]["emp_member"] != role || f.members["dept_fixture"]["emp_owner"] != 100 {
			t.Fatal("role update did not preserve owner or target")
		}
	}
}

func TestDepartmentRoleChangesRejectUnauthorizedOrStructuralUpdatesBeforeWrite(t *testing.T) {
	t.Setenv("HRM_SSO_REQUIRED", "true")
	for _, test := range []struct {
		name, actor, target string
		role                int32
		profile             bool
	}{
		{"administratorCannotPromote", "emp_admin", "emp_member", 60, false},
		{"ordinaryCannotPromote", "emp_member", "emp_admin", 60, false},
		{"outsiderCannotPromote", "emp_outside", "emp_member", 60, false},
		{"targetMustExist", "emp_owner", "emp_outside", 60, false},
		{"ownerCannotDemoteSelf", "emp_owner", "emp_owner", 20, false},
		{"adminCannotElevateSelf", "emp_admin", "emp_admin", 100, false},
		{"cannotAssignOwner", "emp_owner", "emp_member", 100, false},
		{"cannotMixHRMProfile", "emp_owner", "emp_member", 60, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, f := roleServer()
			q := roleRequest("dept_fixture", test.target, test.role)
			if test.profile {
				q.Members[0].Nickname = wrapperspb.String("synthetic")
			}
			if _, err := s.SetGroupMemberInfo(mcontext.SetOpUserID(context.Background(), test.actor), q); err == nil || len(f.updates) != 0 {
				t.Fatal("forbidden department update reached persistence")
			}
		})
	}
}

func TestDepartmentBatchAndProfileGuardsDoNotPartiallyPersist(t *testing.T) {
	t.Setenv("HRM_SSO_REQUIRED", "true")
	for _, field := range []string{"nickname", "faceURL", "ex", "duplicate", "otherGroup", "nil"} {
		t.Run(field, func(t *testing.T) {
			s, f := roleServer()
			q := roleRequest("dept_fixture", "emp_member", 60)
			switch field {
			case "nickname":
				q.Members[0].Nickname = wrapperspb.String("")
			case "faceURL":
				q.Members[0].FaceURL = wrapperspb.String("")
			case "ex":
				q.Members[0].Ex = wrapperspb.String("")
			case "duplicate":
				q.Members = append(q.Members, q.Members[0])
			case "otherGroup":
				f.members["dept_other"] = map[string]int32{"emp_owner": 20, "emp_member": 20, "emp_other_owner": 100}
				q.Members = append(q.Members, roleRequest("dept_other", "emp_member", 60).Members[0])
			case "nil":
				q.Members = append(q.Members, nil)
			}
			if _, err := s.SetGroupMemberInfo(mcontext.SetOpUserID(context.Background(), "emp_owner"), q); err == nil || len(f.updates) != 0 {
				t.Fatal("mixed/invalid update partially changed a group")
			}
		})
	}
}

func TestDepartmentOwnerCannotBeRemovedByRoleUpdateEvenByBridge(t *testing.T) {
	t.Setenv("HRM_SSO_REQUIRED", "true")
	for _, actor := range []string{"emp_owner", "emp_admin", "imAdmin"} {
		s, f := roleServer()
		if _, err := s.SetGroupMemberInfo(mcontext.SetOpUserID(context.Background(), actor), roleRequest("dept_fixture", "emp_owner", 20)); err == nil || len(f.updates) != 0 {
			t.Fatal("ownership changed outside the HRM transfer path")
		}
	}
}

func TestOrdinaryGroupOwnerAndTrustedHRMBridgeKeepRoleManagement(t *testing.T) {
	t.Setenv("HRM_SSO_REQUIRED", "true")
	for _, test := range []struct{ group, actor string }{{"normal_fixture", "emp_owner"}, {"dept_fixture", "imAdmin"}} {
		s, f := roleServer()
		if _, err := s.SetGroupMemberInfo(mcontext.SetOpUserID(context.Background(), test.actor), roleRequest(test.group, "emp_member", 60)); err != nil || len(f.updates) != 1 {
			t.Fatal("existing owner/bridge role operation blocked", err)
		}
	}
}
