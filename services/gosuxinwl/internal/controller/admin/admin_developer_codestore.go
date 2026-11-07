package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/developer"
	"github.com/suxinwl/GoSuxin/internal/service"
)

func (c *ControllerDeveloper) GetCodeCate(ctx context.Context, req *developer.GetCodeCateReq) (res *developer.GetCodeCateRes, err error) {
	res = &developer.GetCodeCateRes{R: service.Admindeveloper().GetCodeCate(ctx, req)}
	return
}
func (c *ControllerDeveloper) CodeList(ctx context.Context, req *developer.CodeListReq) (res *developer.CodeListRes, err error) {
	res = &developer.CodeListRes{R: service.Admindeveloper().CodeList(ctx, req)}
	return
}
func (c *ControllerDeveloper) Login(ctx context.Context, req *developer.LoginReq) (res *developer.LoginRes, err error) {
	res = &developer.LoginRes{R: service.Admindeveloper().Login(ctx, req)}
	return
}
func (c *ControllerDeveloper) AsyncVersion(ctx context.Context, req *developer.AsyncVersionReq) (res *developer.AsyncVersionRes, err error) {
	res = &developer.AsyncVersionRes{R: service.Admindeveloper().AsyncVersion(ctx, req)}
	return
}
func (c *ControllerDeveloper) CheckPackName(ctx context.Context, req *developer.CheckPackNameReq) (res *developer.CheckPackNameRes, err error) {
	res = &developer.CheckPackNameRes{R: service.Admindeveloper().CheckPackName(ctx, req)}
	return
}
func (c *ControllerDeveloper) SavePackName(ctx context.Context, req *developer.SavePackNameReq) (res *developer.SavePackNameRes, err error) {
	res = &developer.SavePackNameRes{R: service.Admindeveloper().SavePackName(ctx, req)}
	return
}
func (c *ControllerDeveloper) Requirement(ctx context.Context, req *developer.RequirementReq) (res *developer.RequirementRes, err error) {
	res = &developer.RequirementRes{R: service.Admindeveloper().Requirement(ctx, req)}
	return
}
func (c *ControllerDeveloper) UpPrivateHouse(ctx context.Context, req *developer.UpPrivateHouseReq) (res *developer.UpPrivateHouseRes, err error) {
	res = &developer.UpPrivateHouseRes{R: service.Admindeveloper().UpPrivateHouse(ctx, req)}
	return
}
func (c *ControllerDeveloper) GetPackdirs(ctx context.Context, req *developer.GetPackdirsReq) (res *developer.GetPackdirsRes, err error) {
	res = &developer.GetPackdirsRes{R: service.Admindeveloper().GetPackdirs(ctx, req)}
	return
}
func (c *ControllerDeveloper) GetMenutree(ctx context.Context, req *developer.GetMenutreeReq) (res *developer.GetMenutreeRes, err error) {
	res = &developer.GetMenutreeRes{R: service.Admindeveloper().GetMenutree(ctx, req)}
	return
}
func (c *ControllerDeveloper) MenuTreeToJson(ctx context.Context, req *developer.MenuTreeToJsonReq) (res *developer.MenuTreeToJsonRes, err error) {
	res = &developer.MenuTreeToJsonRes{R: service.Admindeveloper().MenuTreeToJson(ctx, req)}
	return
}

// 发布插件代码到代码仓
func (c *ControllerDeveloper) UpPackToService(ctx context.Context, req *developer.UpPackToServiceReq) (res *developer.UpPackToServiceRes, err error) {
	res = &developer.UpPackToServiceRes{R: service.Admindeveloper().UpPackToService(ctx, req)}
	return
}

// 获取邮箱验证码
func (c *ControllerDeveloper) LoginCode(ctx context.Context, req *developer.LoginCodeReq) (res *developer.LoginCodeRes, err error) {
	res = &developer.LoginCodeRes{R: service.Admindeveloper().LoginCode(ctx, req)}
	return
}

// 免密登录
func (c *ControllerDeveloper) FreeLogin(ctx context.Context, req *developer.FreeLoginReq) (res *developer.FreeLoginRes, err error) {
	res = &developer.FreeLoginRes{R: service.Admindeveloper().FreeLogin(ctx, req)}
	return
}

// 注册账号
func (c *ControllerDeveloper) RegisterUser(ctx context.Context, req *developer.RegisterUserReq) (res *developer.RegisterUserRes, err error) {
	res = &developer.RegisterUserRes{R: service.Admindeveloper().RegisterUser(ctx, req)}
	return
}
