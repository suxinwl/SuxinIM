package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/install"
	"github.com/suxinwl/GoSuxin/internal/service"
)

func (c *ControllerInstall) GetConfig(ctx context.Context, req *install.GetConfigReq) (res *install.GetConfigRes, err error) {
	res = &install.GetConfigRes{R: service.Admininstall().GetConfig(ctx, req)}
	return
}
func (c *ControllerInstall) Save(ctx context.Context, req *install.SaveReq) (res *install.SaveRes, err error) {
	res = &install.SaveRes{R: service.Admininstall().Save(ctx, req)}
	return
}
