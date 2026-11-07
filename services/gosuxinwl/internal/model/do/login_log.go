// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// LoginLog is the golang structure of table gf_login_log for DAO operations like Where/Data.
type LoginLog struct {
	g.Meta     `orm:"table:gf_login_log, do:true"`
	Id         any         //
	Uid        any         // 用户id
	Username   any         // 登录账号
	Ip         any         // 登录IP
	Address    any         // 地点
	Des        any         // 登录行为
	Os         any         // 操作系统
	Browser    any         // 浏览器类型
	ErrorMsg   any         // 登录失败原因
	Status     any         // 状态:0=成功,1=失败
	Createtime *gtime.Time // 创建时间
}
