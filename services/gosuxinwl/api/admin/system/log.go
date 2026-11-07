package system

import (
	"github.com/suxinwl/GoSuxin/api/admin/baseapi"
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// 登录日志
type GetLoginReq struct {
	g.Meta `path:"/system/log/getLogin" tags:"GetLogin" method:"get" summary:"获取登录日志列表"`
	baseapi.PageReq
	User       string `p:"user" d:"" dc:"用户名"`
	Ip         string `p:"ip" d:"" dc:"ip地址"`
	Status     any    `p:"status" d:"" dc:"状态"`
	Createtime string `p:"createtime" d:"" dc:"创建时间"`
}
type GetLoginRes struct {
	*gf.R
}
type DelLastLoginReq struct {
	g.Meta `path:"/system/log/delLastLogin" tags:"DelLastLogin" method:"delete" summary:"删除上个月登录日志"`
}
type DelLastLoginRes struct {
	*gf.R
}

// 操作日志
type GetOperationReq struct {
	g.Meta `path:"/system/log/getOperation" tags:"GetOperation" method:"get" summary:"获取操作日志列表"`
	baseapi.PageReq
	User       string `p:"user" d:"" dc:"用户名"`
	Ip         string `p:"ip" d:"" dc:"ip地址"`
	Status     any    `p:"status" d:"" dc:"状态"`
	Createtime string `p:"createtime" d:"" dc:"创建时间"`
}
type GetOperationRes struct {
	*gf.R
}
type DelLastOperationReq struct {
	g.Meta `path:"/system/log/delLastOperation" tags:"DelLastOperation" method:"delete" summary:"删除上个月操作日志"`
}
type DelLastOperationRes struct {
	*gf.R
}

type GetOperationDetailReq struct {
	g.Meta `path:"/system/log/getOperationDetail" tags:"getContent" method:"get" summary:"获取操作日志内容"`
	Id     int64 `p:"id" v:"required#id不能为空" dc:"查找数据id"`
}
type GetOperationDetailRes struct {
	*gf.R
}
