package group

import (
	"context"
	"github.com/openimsdk/open-im-server/v3/pkg/authverify"
	"github.com/openimsdk/protocol/constant"
	pbgroup "github.com/openimsdk/protocol/group"
	"github.com/openimsdk/tools/errs"
	"github.com/openimsdk/tools/mcontext"
	"openim38.local/identity"
	"strings"
)

func (s *groupServer) guardHRMGroup(ctx context.Context, id string) error {
	if identity.Required() && strings.HasPrefix(id, "dept_") && !authverify.IsAppManagerUid(ctx, s.config.Share.IMAdminUserID) {
		return errs.ErrNoPermission.WrapMsg("department groups are managed by HRM")
	}
	return nil
}

func hrmDepartmentGroup(id string) bool {
	return identity.Required() && strings.HasPrefix(id, "dept_")
}

func (s *groupServer) guardHRMGroupMemberInfo(ctx context.Context, member *pbgroup.SetGroupMemberInfo) error {
	err := s.guardHRMGroup(ctx, member.GroupID)
	if err == nil {
		return nil
	}
	// HRM still owns membership, ownership and profile fields. The native group
	// owner may delegate moderation without changing any of those identities;
	// SetGroupMemberInfo checks the actual persisted owner/member roles below.
	if identity.Employee(mcontext.GetOpUserID(ctx)) && identity.Employee(member.UserID) &&
		member.RoleLevel != nil && (member.RoleLevel.Value == constant.GroupAdmin || member.RoleLevel.Value == constant.GroupOrdinaryUsers) &&
		member.Nickname == nil && member.FaceURL == nil && member.Ex == nil {
		return nil
	}
	return err
}
