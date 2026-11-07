package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/datacenter"
	"github.com/suxinwl/GoSuxin/internal/service"
)

func (c *ControllerDatacenter) GetMyFiles(ctx context.Context, req *datacenter.GetMyFilesReq) (res *datacenter.GetMyFilesRes, err error) {
	res = &datacenter.GetMyFilesRes{R: service.Admindatacenter().GetMyFiles(ctx, req)}
	return
}
func (c *ControllerDatacenter) Save(ctx context.Context, req *datacenter.SaveReq) (res *datacenter.SaveRes, err error) {
	res = &datacenter.SaveRes{R: service.Admindatacenter().Save(ctx, req)}
	return
}
func (c *ControllerDatacenter) DelDir(ctx context.Context, req *datacenter.DelDirReq) (res *datacenter.DelDirRes, err error) {
	res = &datacenter.DelDirRes{R: service.Admindatacenter().DelDir(ctx, req)}
	return
}
func (c *ControllerDatacenter) Del(ctx context.Context, req *datacenter.DelReq) (res *datacenter.DelRes, err error) {
	res = &datacenter.DelRes{R: service.Admindatacenter().Del(ctx, req)}
	return
}
func (c *ControllerDatacenter) UpImgPid(ctx context.Context, req *datacenter.UpImgPidReq) (res *datacenter.UpImgPidRes, err error) {
	res = &datacenter.UpImgPidRes{R: service.Admindatacenter().UpImgPid(ctx, req)}
	return
}
