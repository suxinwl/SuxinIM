// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// AuthRole is the golang structure of table gf_auth_role for DAO operations like Where/Data.
type AuthRole struct {
	g.Meta     `orm:"table:gf_auth_role, do:true"`
	Id         any         //
	BusinessId any         // 业务主账号id
	AccountId  any         // 添加用户id
	Pid        any         // 父级
	Name       any         // 名称
	Rules      any         // 规则ID 所拥有的权限包括父级
	Menu       any         // 选择的id，用于编辑赋值
	Btns       any         // 按钮id，用于编辑赋值
	Status     any         // 状态1=禁用
	DataAccess any         // 数据权限0=自己1=自己及子权限，2=全部
	Remark     any         // 描述
	Weigh      any         // 排序
	Createtime *gtime.Time // 添加时间
}
