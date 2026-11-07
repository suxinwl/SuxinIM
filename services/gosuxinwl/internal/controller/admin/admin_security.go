package admin

import (
	"context"
	"errors"
	"github.com/suxinwl/GoSuxin/api/admin/system"
	"github.com/suxinwl/GoSuxin/api/admin/user"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
	"github.com/suxinwl/GoSuxin/internal/accountsecurity"
	"github.com/suxinwl/GoSuxin/internal/imbridge"
	"github.com/suxinwl/GoSuxin/utility/auth"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

type ControllerSecurity struct{}

func NewSecurity() *ControllerSecurity { return &ControllerSecurity{} }

func (c *ControllerSecurity) EffectivePolicy(ctx context.Context, r *user.EffectivePolicyReq) (*user.EffectivePolicyRes, error) {
	p, e := imbridge.CurrentHostPolicy(ctx, gconv.Int64(ctx.Value("uid")))
	if e != nil {
		return &user.EffectivePolicyRes{R: gf.Failed().SetMsg("暂时无法确认访问权限")}, nil
	}
	return &user.EffectivePolicyRes{R: gf.Success().SetData(p)}, nil
}
func securityReply(v any, e error) *system.SecurityRes {
	if e != nil {
		message := "账号安全操作未通过，请刷新后重试"
		if errors.Is(e, accountsecurity.ErrConflict) || errors.Is(e, accountsecurity.ErrInvalid) || errors.Is(e, accountsecurity.ErrDenied) {
			message = e.Error()
		}
		return &system.SecurityRes{R: gf.Failed().SetMsg(message)}
	}
	return &system.SecurityRes{R: gf.Success().SetData(v)}
}
func managedSecurity(ctx context.Context, id int64, role bool) (*accountsecurity.Store, error) {
	s, e := accountsecurity.Default(ctx)
	if e != nil {
		return nil, e
	}
	if e = s.AssertManagedTarget(ctx, gconv.Int64(ctx.Value("uid")), id, role); e != nil {
		return nil, e
	}
	return s, nil
}
func (c *ControllerSecurity) AccountSecurity(ctx context.Context, r *system.AccountSecurityReq) (*system.SecurityRes, error) {
	s, e := managedSecurity(ctx, r.ID, false)
	if e != nil {
		return securityReply(nil, e), nil
	}
	v, e := s.State(ctx, r.ID)
	return securityReply(v, e), nil
}
func (c *ControllerSecurity) AccountResetPassword(ctx context.Context, r *system.AccountResetPasswordReq) (*system.SecurityRes, error) {
	s, e := managedSecurity(ctx, r.ID, false)
	if e == nil {
		e = s.ChangePassword(ctx, r.ID, gconv.Int64(ctx.Value("uid")), "", r.Password, "", true)
	}
	if e == nil {
		_ = auth.RemoveToken(ctx, gconv.String(r.ID))
	}
	return securityReply(map[string]bool{"reset": e == nil, "mustChangePassword": true}, e), nil
}
func (c *ControllerSecurity) AccountUnlock(ctx context.Context, r *system.AccountUnlockReq) (*system.SecurityRes, error) {
	s, e := managedSecurity(ctx, r.ID, false)
	if e == nil {
		e = s.Unlock(ctx, r.ID, gconv.Int64(ctx.Value("uid")))
	}
	return securityReply(map[string]bool{"unlocked": e == nil}, e), nil
}
func (c *ControllerSecurity) LockPolicy(ctx context.Context, r *system.LockPolicyReq) (*system.SecurityRes, error) {
	s, e := accountsecurity.Default(ctx)
	if e != nil {
		return securityReply(nil, e), nil
	}
	v, e := s.LockPolicy(ctx)
	return securityReply(v, e), nil
}
func (c *ControllerSecurity) SaveLockPolicy(ctx context.Context, r *system.SaveLockPolicyReq) (*system.SecurityRes, error) {
	s, e := accountsecurity.Default(ctx)
	if e != nil {
		return securityReply(nil, e), nil
	}
	v, e := s.SaveLockPolicy(ctx, r.LockPolicy, gconv.Int64(ctx.Value("uid")))
	return securityReply(v, e), nil
}
func (c *ControllerSecurity) EmployeePolicy(ctx context.Context, r *system.EmployeePolicyReq) (*system.SecurityRes, error) {
	s, e := managedSecurity(ctx, r.ID, false)
	if e != nil {
		return securityReply(nil, e), nil
	}
	v, e := s.PolicyView(ctx, r.ID)
	return securityReply(v, e), nil
}
func (c *ControllerSecurity) SaveEmployeePolicy(ctx context.Context, r *system.SaveEmployeePolicyReq) (*system.SecurityRes, error) {
	s, e := managedSecurity(ctx, r.ID, false)
	if e != nil {
		return securityReply(nil, e), nil
	}
	v, e := s.SavePolicy(ctx, accountsecurity.Policy{Subject: "person", SubjectID: r.ID, Matrix: r.Matrix, Revision: r.Revision}, gconv.Int64(ctx.Value("uid")))
	return securityReply(v, e), nil
}
func (c *ControllerSecurity) RolePolicy(ctx context.Context, r *system.RolePolicyReq) (*system.SecurityRes, error) {
	s, e := managedSecurity(ctx, r.ID, true)
	if e != nil {
		return securityReply(nil, e), nil
	}
	v, e := s.ReadPolicy(ctx, "role", r.ID)
	return securityReply(v, e), nil
}
func (c *ControllerSecurity) SaveRolePolicy(ctx context.Context, r *system.SaveRolePolicyReq) (*system.SecurityRes, error) {
	s, e := managedSecurity(ctx, r.ID, true)
	if e != nil {
		return securityReply(nil, e), nil
	}
	v, e := s.SavePolicy(ctx, accountsecurity.Policy{Subject: "role", SubjectID: r.ID, Matrix: r.Matrix, Revision: r.Revision}, gconv.Int64(ctx.Value("uid")))
	return securityReply(v, e), nil
}
