// ================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// You can delete these comments if you wish manually maintain this interface file.
// ================================================================================

package service

import (
	"context"
	"github.com/suxinwl/GoSuxin/api/admin/developer"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

type (
	IAdmindeveloper interface {
		// 获取公共仓分类
		GetCodeCate(ctx context.Context, req *developer.GetCodeCateReq) (res *gf.R)
		// 获取公共仓数据
		CodeList(ctx context.Context, req *developer.CodeListReq) (res *gf.R)
		// 登录社区账号
		Login(ctx context.Context, req *developer.LoginReq) (res *gf.R)
		// 更新私有仓地址
		UpPrivateHouse(ctx context.Context, req *developer.UpPrivateHouseReq) (res *gf.R)
		// 检查框架版本更新
		AsyncVersion(ctx context.Context, req *developer.AsyncVersionReq) (res *gf.R)
		// 检查插件标识是否可用
		CheckPackName(ctx context.Context, req *developer.CheckPackNameReq) (res *gf.R)
		// 提交标识占用
		SavePackName(ctx context.Context, req *developer.SavePackNameReq) (res *gf.R)
		// 发布插件需求到社区
		Requirement(ctx context.Context, req *developer.RequirementReq) (res *gf.R)
		// 获取文件路径
		GetPackdirs(ctx context.Context, req *developer.GetPackdirsReq) (res *gf.R)
		// 获取后台菜单
		GetMenutree(ctx context.Context, req *developer.GetMenutreeReq) (res *gf.R)
		// 菜单id转JSON数据
		MenuTreeToJson(ctx context.Context, req *developer.MenuTreeToJsonReq) (res *gf.R)
		// 发布插件到代码仓
		UpPackToService(ctx context.Context, req *developer.UpPackToServiceReq) (res *gf.R)
		// 获取邮箱验证码
		LoginCode(ctx context.Context, req *developer.LoginCodeReq) (res *gf.R)
		// 免密登录
		FreeLogin(ctx context.Context, req *developer.FreeLoginReq) (res *gf.R)
		// 注册账号
		RegisterUser(ctx context.Context, req *developer.RegisterUserReq) (res *gf.R)
		// 打包插件
		PackCode(ctx context.Context, req *developer.PackCodeReq) (res *gf.R)
		// 上传文件到代码仓
		Upfile(ctx context.Context, req *developer.UpfileReq) (res *gf.R)
		// 下载插件代码到本地-安装使用
		DownCode(ctx context.Context, req *developer.DownCodeReq) (res *gf.R)
		// 安装插件
		InstallCode(ctx context.Context, req *developer.InstallCodeReq) (res *gf.R)
		// 卸载插件
		UninstallCode(ctx context.Context, req *developer.UninstallCodeReq) (res *gf.R)
		// 安装本地插件
		InstallLocalCode(ctx context.Context, req *developer.InstallLocalCodeReq) (res *gf.R)
		// 查找本地已经安装的包
		GetInstallPack(ctx context.Context, req *developer.GetInstallPackReq) (res *gf.R)
	}
)

var (
	localAdmindeveloper IAdmindeveloper
)

func Admindeveloper() IAdmindeveloper {
	if localAdmindeveloper == nil {
		panic("implement not found for interface IAdmindeveloper, forgot register?")
	}
	return localAdmindeveloper
}

func RegisterAdmindeveloper(i IAdmindeveloper) {
	localAdmindeveloper = i
}
