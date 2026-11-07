// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// AuthRule is the golang structure of table gf_auth_rule for DAO operations like Where/Data.
type AuthRule struct {
	g.Meta             `orm:"table:gf_auth_rule, do:true"`
	Id                 any         //
	Uid                any         // 添加用户
	Title              any         // 菜单名称
	Des                any         // 描述
	Locale             any         // 中英文标题key
	Weigh              any         // 排序
	Type               any         // 类型 0=目录，1=菜单，2=按钮
	Pid                any         // 上一级
	Icon               any         // 图标
	Routepath          any         // 路由地址
	Routename          any         // 路由名称
	Component          any         // 组件路径
	Redirect           any         // 重定向地址
	Path               any         // 接口路径
	Permission         any         // 权限标识
	Status             any         // 状态 0=启用1=禁用
	Isext              any         // 是否外链 0=否1=是
	Keepalive          any         // 是否缓存 0=否1=是
	Requiresauth       any         // 是否需要登录鉴权 0=否1=是
	Hideinmenu         any         // 是否在左侧菜单中隐藏该项 0=否1=是
	Hidechildreninmenu any         // 强制在左侧菜单中显示单项 0=否1=是
	Activemenu         any         // 高亮设置的菜单项 0=否1=是
	Noaffix            any         // 如果设置为true，标签将不会添加到tab-bar中 0=否1=是
	Onlypage           any         // 独立页面不需layout和登录，如登录页、数据大屏
	Createtime         *gtime.Time // 创建时间
	Updatetime         *gtime.Time // 更新时间
	Deletetime         *gtime.Time // 删除时间
}
