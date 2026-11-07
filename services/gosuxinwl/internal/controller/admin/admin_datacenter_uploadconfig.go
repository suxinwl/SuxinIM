package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/datacenter"
	"github.com/suxinwl/GoSuxin/internal/service"
)

func (c *ControllerDatacenter) GetUploadconfig(ctx context.Context, req *datacenter.GetUploadconfigReq) (res *datacenter.GetUploadconfigRes, err error) {
	res = &datacenter.GetUploadconfigRes{R: service.Admindatacenter().GetUploadconfig(ctx, req)}
	return
}
func (c *ControllerDatacenter) SaveUploadconfig(ctx context.Context, req *datacenter.SaveUploadconfigReq) (res *datacenter.SaveUploadconfigRes, err error) {
	res = &datacenter.SaveUploadconfigRes{R: service.Admindatacenter().SaveUploadconfig(ctx, req)}
	return
}
func (c *ControllerDatacenter) TestConnection(ctx context.Context, req *datacenter.TestConnectionReq) (res *datacenter.TestConnectionRes, err error) {
	return &datacenter.TestConnectionRes{R: service.Admindatacenter().TestConnection(ctx, req)}, nil
}
