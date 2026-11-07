package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/datacenter"
	"github.com/suxinwl/GoSuxin/internal/service"
)

// 管理后台上传附件接口
func (c *ControllerDatacenter) Upload(ctx context.Context, req *datacenter.UploadReq) (res *datacenter.UploadRes, err error) {
	res = &datacenter.UploadRes{R: service.Admindatacenter().Upload(ctx, req)}
	return
}
