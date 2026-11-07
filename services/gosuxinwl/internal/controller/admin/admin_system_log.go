package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/system"
	"github.com/suxinwl/GoSuxin/internal/service"
)

func (c *ControllerSystem) GetLogin(ctx context.Context, req *system.GetLoginReq) (res *system.GetLoginRes, err error) {
	res = &system.GetLoginRes{R: service.Adminsystem().GetLogin(ctx, req)}
	return
}
func (c *ControllerSystem) DelLastLogin(ctx context.Context, req *system.DelLastLoginReq) (res *system.DelLastLoginRes, err error) {
	res = &system.DelLastLoginRes{R: service.Adminsystem().DelLastLogin(ctx, req)}
	return
}
func (c *ControllerSystem) GetOperation(ctx context.Context, req *system.GetOperationReq) (res *system.GetOperationRes, err error) {
	res = &system.GetOperationRes{R: service.Adminsystem().GetOperation(ctx, req)}
	return
}
func (c *ControllerSystem) DelLastOperation(ctx context.Context, req *system.DelLastOperationReq) (res *system.DelLastOperationRes, err error) {
	res = &system.DelLastOperationRes{R: service.Adminsystem().DelLastOperation(ctx, req)}
	return
}
func (c *ControllerSystem) GetOperationDetail(ctx context.Context, req *system.GetOperationDetailReq) (res *system.GetOperationDetailRes, err error) {
	res = &system.GetOperationDetailRes{R: service.Adminsystem().GetOperationDetail(ctx, req)}
	return
}
