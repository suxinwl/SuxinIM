// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// HomeQuickop is the golang structure of table gf_home_quickop for DAO operations like Where/Data.
type HomeQuickop struct {
	g.Meta     `orm:"table:gf_home_quickop, do:true"`
	Id         any //
	BusinessId any // 业务主账号id
	Uid        any // 添加人
	IsCommon   any // 公共1=是
	Type       any // 类型1=外部
	Name       any // 快捷名称
	PathUrl    any // 跳转路径
	Icon       any // 图标
	Weigh      any // 权重
}
