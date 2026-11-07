// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// AuthDept is the golang structure of table gf_auth_dept for DAO operations like Where/Data.
type AuthDept struct {
	g.Meta         `orm:"table:gf_auth_dept, do:true"`
	Id             any         //
	BusinessId     any         // 业务主账号id
	AccountId      any         // 添加账号
	Name           any         // 部门名称
	Pid            any         // 上级部门
	Weigh          any         // 排序
	Status         any         // 状态:0=正常;1=禁用
	IMGroupEnabled any         // 部门群自动管理开关
	LeaderUID      any         // 部门负责人宿主UID
	Remark         any         // 备注
	Createtime     *gtime.Time // 创建时间
	Updatetime     *gtime.Time // 更新时间
	Deletetime     *gtime.Time // 删除时间
}
