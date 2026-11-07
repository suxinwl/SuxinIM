package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/dashboard"
	"github.com/suxinwl/GoSuxin/internal/service"
)

// 获取首页快捷数据
func (c *ControllerDashboard) GetQuick(ctx context.Context, req *dashboard.GetQuickReq) (res *dashboard.GetQuickRes, err error) {
	res = &dashboard.GetQuickRes{R: service.AdminDashboard().GetQuick(ctx, req)}
	return
}

// 保存快捷数据
func (c *ControllerDashboard) SaveQuick(ctx context.Context, req *dashboard.SaveQuickReq) (res *dashboard.SaveQuickRes, err error) {
	res = &dashboard.SaveQuickRes{R: service.AdminDashboard().SaveQuick(ctx, req)}
	return
}
