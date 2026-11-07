package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/developer"
	"github.com/suxinwl/GoSuxin/internal/service"
)

// 打包
func (c *ControllerDeveloper) PackCode(ctx context.Context, req *developer.PackCodeReq) (res *developer.PackCodeRes, err error) {
	res = &developer.PackCodeRes{R: service.Admindeveloper().PackCode(ctx, req)}
	return
}

// 上传文件
func (c *ControllerDeveloper) Upfile(ctx context.Context, req *developer.UpfileReq) (res *developer.UpfileRes, err error) {
	res = &developer.UpfileRes{R: service.Admindeveloper().Upfile(ctx, req)}
	return
}

// 下载插件代码
func (c *ControllerDeveloper) DownCode(ctx context.Context, req *developer.DownCodeReq) (res *developer.DownCodeRes, err error) {
	res = &developer.DownCodeRes{R: service.Admindeveloper().DownCode(ctx, req)}
	return
}

// 安装插件
func (c *ControllerDeveloper) InstallCode(ctx context.Context, req *developer.InstallCodeReq) (res *developer.InstallCodeRes, err error) {
	res = &developer.InstallCodeRes{R: service.Admindeveloper().InstallCode(ctx, req)}
	return
}

// 卸载插件
func (c *ControllerDeveloper) UninstallCode(ctx context.Context, req *developer.UninstallCodeReq) (res *developer.UninstallCodeRes, err error) {
	res = &developer.UninstallCodeRes{R: service.Admindeveloper().UninstallCode(ctx, req)}
	return
}

// 安装本地插件
func (c *ControllerDeveloper) InstallLocalCode(ctx context.Context, req *developer.InstallLocalCodeReq) (res *developer.InstallLocalCodeRes, err error) {
	res = &developer.InstallLocalCodeRes{R: service.Admindeveloper().InstallLocalCode(ctx, req)}
	return
}

// 查找本地已经安装的包
func (c *ControllerDeveloper) GetInstallPack(ctx context.Context, req *developer.GetInstallPackReq) (res *developer.GetInstallPackRes, err error) {
	res = &developer.GetInstallPackRes{R: service.Admindeveloper().GetInstallPack(ctx, req)}
	return
}
