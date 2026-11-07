package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/system"
	"github.com/suxinwl/GoSuxin/internal/service"
)

func (c *ControllerSystem) AccountList(ctx context.Context, req *system.AccountListReq) (res *system.AccountListRes, err error) {
	res = &system.AccountListRes{R: service.Adminsystem().AccountList(ctx, req)}
	return
}
func (c *ControllerSystem) Isaccountexist(ctx context.Context, req *system.IsaccountexistReq) (res *system.IsaccountexistRes, err error) {
	res = &system.IsaccountexistRes{R: service.Adminsystem().Isaccountexist(ctx, req)}
	return
}
func (c *ControllerSystem) AccountSave(ctx context.Context, req *system.AccountSaveReq) (res *system.AccountSaveRes, err error) {
	res = &system.AccountSaveRes{R: service.Adminsystem().AccountSave(ctx, req)}
	return
}
func (c *ControllerSystem) AccountStatus(ctx context.Context, req *system.AccountStatusReq) (res *system.AccountStatusRes, err error) {
	res = &system.AccountStatusRes{R: service.Adminsystem().AccountStatus(ctx, req)}
	return
}
func (c *ControllerSystem) AccountDel(ctx context.Context, req *system.AccountDelReq) (res *system.AccountDelRes, err error) {
	res = &system.AccountDelRes{R: service.Adminsystem().AccountDel(ctx, req)}
	return
}
