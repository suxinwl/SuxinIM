package common

import (
	"context"
	"fmt"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/api/common/basetool"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/utility/extend/pan123"
)

func (c *ControllerBasetool) Pan123Asset(ctx context.Context, req *basetool.Pan123AssetReq) (res *basetool.Pan123AssetRes, err error) {
	r := g.RequestFromCtx(ctx)
	r.Response.Header().Set("Cache-Control", "no-store, private")
	r.Response.Header().Set("Referrer-Policy", "no-referrer")
	reference := fmt.Sprintf("%s%d/%d/%s", pan123.Prefix, req.UID, req.FileID, req.Name)
	if _, _, _, e := pan123.ParseReference(reference); e != nil {
		r.Response.WriteStatus(404)
		r.ExitAll()
		return
	}
	count, e := dao.Attachment.Ctx(ctx).Where("location", "pan123").Where("url", reference).Count()
	if e != nil {
		r.Response.WriteStatus(503)
		r.ExitAll()
		return
	}
	if count == 0 {
		r.Response.WriteStatus(404)
		r.ExitAll()
		return
	}
	p, e := pan123.Load(ctx)
	if e != nil {
		r.Response.WriteStatus(503)
		r.ExitAll()
		return
	}
	address, e := pan123.URL(ctx, p, reference)
	if e != nil {
		r.Response.WriteStatus(502, "云盘附件暂不可用")
		r.ExitAll()
		return
	}
	r.Response.RedirectTo(address, 302)
	r.ExitAll()
	return
}
