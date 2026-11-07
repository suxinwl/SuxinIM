// ================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// You can delete these comments if you wish manually maintain this interface file.
// ================================================================================

package service

import (
	"context"
	"github.com/suxinwl/GoSuxin/api/admin/dashboard"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

type (
	IAdminDashboard interface {
		// 获取快捷操作数据
		GetQuick(ctx context.Context, req *dashboard.GetQuickReq) (res *gf.R)
		// 保存快捷操作数据
		SaveQuick(ctx context.Context, req *dashboard.SaveQuickReq) (res *gf.R)
	}
)

var (
	localAdminDashboard IAdminDashboard
)

func AdminDashboard() IAdminDashboard {
	if localAdminDashboard == nil {
		panic("implement not found for interface IAdminDashboard, forgot register?")
	}
	return localAdminDashboard
}

func RegisterAdminDashboard(i IAdminDashboard) {
	localAdminDashboard = i
}
