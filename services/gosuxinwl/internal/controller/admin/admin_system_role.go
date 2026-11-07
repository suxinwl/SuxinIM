package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/system"
	"github.com/suxinwl/GoSuxin/internal/service"
)

func (c *ControllerSystem) RoleList(ctx context.Context, req *system.RoleListReq) (res *system.RoleListRes, err error) {
	res = &system.RoleListRes{R: service.Adminsystem().RoleList(ctx, req)}
	return
}

func (c *ControllerSystem) RoleParent(ctx context.Context, req *system.RoleParentReq) (res *system.RoleParentRes, err error) {
	res = &system.RoleParentRes{R: service.Adminsystem().RoleParent(ctx, req)}
	return
}
func (c *ControllerSystem) RoleMenuList(ctx context.Context, req *system.RoleMenuListReq) (res *system.RoleMenuListRes, err error) {
	res = &system.RoleMenuListRes{R: service.Adminsystem().RoleMenuList(ctx, req)}
	return
}
func (c *ControllerSystem) RoleSave(ctx context.Context, req *system.RoleSaveReq) (res *system.RoleSaveRes, err error) {
	res = &system.RoleSaveRes{R: service.Adminsystem().RoleSave(ctx, req)}
	return
}
func (c *ControllerSystem) RoleStatus(ctx context.Context, req *system.RoleStatusReq) (res *system.RoleStatusRes, err error) {
	res = &system.RoleStatusRes{R: service.Adminsystem().RoleStatus(ctx, req)}
	return
}

func (c *ControllerSystem) RoleDel(ctx context.Context, req *system.RoleDelReq) (res *system.RoleDelRes, err error) {
	res = &system.RoleDelRes{R: service.Adminsystem().RoleDel(ctx, req)}
	return
}
