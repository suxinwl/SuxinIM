package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/datacenter"
	"github.com/suxinwl/GoSuxin/internal/service"
)

func (c *ControllerDatacenter) GetEmail(ctx context.Context, req *datacenter.GetEmailReq) (res *datacenter.GetEmailRes, err error) {
	res = &datacenter.GetEmailRes{R: service.Admindatacenter().GetEmail(ctx, req)}
	return
}
func (c *ControllerDatacenter) SaveEmail(ctx context.Context, req *datacenter.SaveEmailReq) (res *datacenter.SaveEmailRes, err error) {
	res = &datacenter.SaveEmailRes{R: service.Admindatacenter().SaveEmail(ctx, req)}
	return
}
func (c *ControllerDatacenter) GetCodestoreConfig(ctx context.Context, req *datacenter.GetCodestoreConfigReq) (res *datacenter.GetCodestoreConfigRes, err error) {
	res = &datacenter.GetCodestoreConfigRes{R: service.Admindatacenter().GetCodestoreConfig(ctx, req)}
	return
}
func (c *ControllerDatacenter) SaveCodeStoreConfig(ctx context.Context, req *datacenter.SaveCodeStoreConfigReq) (res *datacenter.SaveCodeStoreConfigRes, err error) {
	res = &datacenter.SaveCodeStoreConfigRes{R: service.Admindatacenter().SaveCodeStoreConfig(ctx, req)}
	return
}
func (c *ControllerDatacenter) UpConfigStatus(ctx context.Context, req *datacenter.UpConfigStatusReq) (res *datacenter.UpConfigStatusRes, err error) {
	res = &datacenter.UpConfigStatusRes{R: service.Admindatacenter().UpConfigStatus(ctx, req)}
	return
}
