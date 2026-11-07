// ================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// You can delete these comments if you wish manually maintain this interface file.
// ================================================================================

package service

import (
	"context"
	"github.com/suxinwl/GoSuxin/api/admin/user"
	"github.com/suxinwl/GoSuxin/internal/model/entity"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

type (
	IAdminUser interface {
		// AuthenticatePassword validates HRM credentials without creating a host session.
		AuthenticatePassword(ctx context.Context, username, password, captchaID, captcha string) (*entity.Admin, error)
		// 实现登录业务处理
		Login(ctx context.Context, req *user.LoginReq) (res *gf.R)
		// 获取用户信息
		GetUserinfo(ctx context.Context, req *user.GetUserinfoReq) (res *gf.R)
		// 修改用户信息
		SaveInfo(ctx context.Context, req *user.SaveInfoReq) (res *gf.R)
		// 退出登录
		LoginOut(ctx context.Context, req *user.LoginOutReq) (res *gf.R)
		// 获取管理后台菜单
		GetMenu(ctx context.Context, req *user.GetMenuReq) (res *gf.R)
	}
)

var (
	localAdminUser IAdminUser
)

func AdminUser() IAdminUser {
	if localAdminUser == nil {
		panic("implement not found for interface IAdminUser, forgot register?")
	}
	return localAdminUser
}

func RegisterAdminUser(i IAdminUser) {
	localAdminUser = i
}
