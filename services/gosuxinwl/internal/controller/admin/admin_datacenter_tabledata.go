package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/datacenter"
	"github.com/suxinwl/GoSuxin/internal/service"
)

func (c *ControllerDatacenter) TabledataList(ctx context.Context, req *datacenter.TabledataListReq) (res *datacenter.TabledataListRes, err error) {
	res = &datacenter.TabledataListRes{R: service.Admindatacenter().TabledataList(ctx, req)}
	return
}
func (c *ControllerDatacenter) TabledataSave(ctx context.Context, req *datacenter.TabledataSaveReq) (res *datacenter.TabledataSaveRes, err error) {
	res = &datacenter.TabledataSaveRes{R: service.Admindatacenter().TabledataSave(ctx, req)}
	return
}
func (c *ControllerDatacenter) TabledataDel(ctx context.Context, req *datacenter.TabledataDelReq) (res *datacenter.TabledataDelRes, err error) {
	res = &datacenter.TabledataDelRes{R: service.Admindatacenter().TabledataDel(ctx, req)}
	return
}
