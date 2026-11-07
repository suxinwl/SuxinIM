// ================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// You can delete these comments if you wish manually maintain this interface file.
// ================================================================================

package service

import (
	"context"
	"github.com/suxinwl/GoSuxin/api/admin/install"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

type (
	IAdmininstall interface {
		// 获取安装配置数据
		GetConfig(ctx context.Context, req *install.GetConfigReq) (res *gf.R)
		// 执行安装操作
		Save(ctx context.Context, req *install.SaveReq) (res *gf.R)
	}
)

var (
	localAdmininstall IAdmininstall
)

func Admininstall() IAdmininstall {
	if localAdmininstall == nil {
		panic("implement not found for interface IAdmininstall, forgot register?")
	}
	return localAdmininstall
}

func RegisterAdmininstall(i IAdmininstall) {
	localAdmininstall = i
}
