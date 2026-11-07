// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// OperationLog is the golang structure of table gf_operation_log for DAO operations like Where/Data.
type OperationLog struct {
	g.Meta      `orm:"table:gf_operation_log, do:true"`
	Id          any         //
	Uid         any         // 用户id
	Method      any         // 请求方法
	Url         any         // 请求地址
	Ip          any         // 登录IP
	Address     any         // 地点
	Des         any         // 登录行为
	ReqHeaders  any         // 请求头
	ReqBody     any         // 请求体
	RespHeaders any         // 响应头
	RespBody    any         // 响应体
	Latency     any         // 耗时
	Status      any         // 状态:0=成功,1=失败
	Createtime  *gtime.Time // 创建时间
}
