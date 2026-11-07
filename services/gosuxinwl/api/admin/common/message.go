package common

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// 信息列表
type MessageListReq struct {
	g.Meta `path:"/common/message/getList" tags:"getList" method:"get" summary:"信息列表"`
	Type   any `p:"type" d:"" dc:"消息类型"`
}
type MessageListRes struct {
	*gf.R
}
