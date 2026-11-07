package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/user"
	"github.com/suxinwl/GoSuxin/internal/service"
)

func (c *ControllerUser) Login(ctx context.Context, req *user.LoginReq) (res *user.LoginRes, err error) {
	res = &user.LoginRes{R: service.AdminUser().Login(ctx, req)}
	return
}
func (c *ControllerUser) LoginOut(ctx context.Context, req *user.LoginOutReq) (res *user.LoginOutRes, err error) {
	res = &user.LoginOutRes{R: service.AdminUser().LoginOut(ctx, req)}
	return
}

func (c *ControllerUser) GetUserinfo(ctx context.Context, req *user.GetUserinfoReq) (res *user.GetUserinfoRes, err error) {
	res = &user.GetUserinfoRes{R: service.AdminUser().GetUserinfo(ctx, req)}
	return
}

func (c *ControllerUser) GetMenu(ctx context.Context, req *user.GetMenuReq) (res *user.GetMenuRes, err error) {
	res = &user.GetMenuRes{R: service.AdminUser().GetMenu(ctx, req)}
	return
}

func (c *ControllerUser) SaveInfo(ctx context.Context, req *user.SaveInfoReq) (res *user.SaveInfoRes, err error) {
	res = &user.SaveInfoRes{R: service.AdminUser().SaveInfo(ctx, req)}
	return
}
