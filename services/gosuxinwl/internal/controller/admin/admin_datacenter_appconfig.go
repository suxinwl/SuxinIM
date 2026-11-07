package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/datacenter"
	"github.com/suxinwl/GoSuxin/internal/service"
)

func (c *ControllerDatacenter) GetConfig(ctx context.Context, req *datacenter.GetConfigReq) (res *datacenter.GetConfigRes, err error) {
	res = &datacenter.GetConfigRes{R: service.Admindatacenter().GetConfig(ctx, req)}
	return
}
func (c *ControllerDatacenter) SaveConfig(ctx context.Context, req *datacenter.SaveConfigReq) (res *datacenter.SaveConfigRes, err error) {
	res = &datacenter.SaveConfigRes{R: service.Admindatacenter().SaveConfig(ctx, req)}
	return
}
