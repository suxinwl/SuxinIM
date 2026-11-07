package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/system"
	"github.com/suxinwl/GoSuxin/internal/service"
)

func (c *ControllerSystem) DeptList(ctx context.Context, req *system.DeptListReq) (res *system.DeptListRes, err error) {
	res = &system.DeptListRes{R: service.Adminsystem().DeptList(ctx, req)}
	return
}

func (c *ControllerSystem) DeptParent(ctx context.Context, req *system.DeptParentReq) (res *system.DeptParentRes, err error) {
	res = &system.DeptParentRes{R: service.Adminsystem().DeptParent(ctx, req)}
	return
}

func (c *ControllerSystem) DeptSave(ctx context.Context, req *system.DeptSaveReq) (res *system.DeptSaveRes, err error) {
	res = &system.DeptSaveRes{R: service.Adminsystem().DeptSave(ctx, req)}
	return
}
func (c *ControllerSystem) DeptLeaderOptions(ctx context.Context, req *system.DeptLeaderOptionsReq) (*system.DeptLeaderOptionsRes, error) {
	return &system.DeptLeaderOptionsRes{R: service.Adminsystem().DeptLeaderOptions(ctx, req)}, nil
}
func (c *ControllerSystem) DeptIMGroup(ctx context.Context, req *system.DeptIMGroupReq) (*system.DeptIMGroupRes, error) {
	return &system.DeptIMGroupRes{R: service.Adminsystem().DeptIMGroup(ctx, req)}, nil
}
func (c *ControllerSystem) DeptStatus(ctx context.Context, req *system.DeptStatusReq) (res *system.DeptStatusRes, err error) {
	res = &system.DeptStatusRes{R: service.Adminsystem().DeptStatus(ctx, req)}
	return
}
func (c *ControllerSystem) DeptDel(ctx context.Context, req *system.DeptDelReq) (res *system.DeptDelRes, err error) {
	res = &system.DeptDelRes{R: service.Adminsystem().DeptDel(ctx, req)}
	return
}

// 移动部门
func (c *ControllerSystem) DragDept(ctx context.Context, req *system.DragDeptReq) (res *system.DragDeptRes, err error) {
	res = &system.DragDeptRes{R: service.Adminsystem().DragDept(ctx, req)}
	return
}
