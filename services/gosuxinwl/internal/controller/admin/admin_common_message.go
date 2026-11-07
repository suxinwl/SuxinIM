package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/common"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

// 处理系统信息
func (c *ControllerCommon) MessageList(ctx context.Context, req *common.MessageListReq) (res *common.MessageListRes, err error) {
	res = &common.MessageListRes{
		R: gf.Success().SetMsg("系统消息列表").SetData(gf.Map{"items": make(gf.Slice, 0), "total": 0}),
	}
	return
}
