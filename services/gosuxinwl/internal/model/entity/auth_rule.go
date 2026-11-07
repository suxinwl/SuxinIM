// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// AuthRule is the golang structure for table auth_rule.
type AuthRule struct {
	Id                 uint        `json:"id"                 orm:"id"                 description:""`
	Uid                int         `json:"uid"                orm:"uid"                description:"添加用户"`
	Title              string      `json:"title"              orm:"title"              description:"菜单名称"`
	Des                string      `json:"des"                orm:"des"                description:"描述"`
	Locale             string      `json:"locale"             orm:"locale"             description:"中英文标题key"`
	Weigh              int         `json:"weigh"              orm:"weigh"              description:"排序"`
	Type               int         `json:"type"               orm:"type"               description:"类型 0=目录，1=菜单，2=按钮"`
	Pid                int         `json:"pid"                orm:"pid"                description:"上一级"`
	Icon               string      `json:"icon"               orm:"icon"               description:"图标"`
	Routepath          string      `json:"routepath"          orm:"routepath"          description:"路由地址"`
	Routename          string      `json:"routename"          orm:"routename"          description:"路由名称"`
	Component          string      `json:"component"          orm:"component"          description:"组件路径"`
	Redirect           string      `json:"redirect"           orm:"redirect"           description:"重定向地址"`
	Path               string      `json:"path"               orm:"path"               description:"接口路径"`
	Permission         string      `json:"permission"         orm:"permission"         description:"权限标识"`
	Status             int         `json:"status"             orm:"status"             description:"状态 0=启用1=禁用"`
	Isext              int         `json:"isext"              orm:"isext"              description:"是否外链 0=否1=是"`
	Keepalive          int         `json:"keepalive"          orm:"keepalive"          description:"是否缓存 0=否1=是"`
	Requiresauth       int         `json:"requiresauth"       orm:"requiresauth"       description:"是否需要登录鉴权 0=否1=是"`
	Hideinmenu         int         `json:"hideinmenu"         orm:"hideinmenu"         description:"是否在左侧菜单中隐藏该项 0=否1=是"`
	Hidechildreninmenu int         `json:"hidechildreninmenu" orm:"hidechildreninmenu" description:"强制在左侧菜单中显示单项 0=否1=是"`
	Activemenu         int         `json:"activemenu"         orm:"activemenu"         description:"高亮设置的菜单项 0=否1=是"`
	Noaffix            int         `json:"noaffix"            orm:"noaffix"            description:"如果设置为true，标签将不会添加到tab-bar中 0=否1=是"`
	Onlypage           int         `json:"onlypage"           orm:"onlypage"           description:"独立页面不需layout和登录，如登录页、数据大屏"`
	Createtime         *gtime.Time `json:"createtime"         orm:"createtime"         description:"创建时间"`
	Updatetime         *gtime.Time `json:"updatetime"         orm:"updatetime"         description:"更新时间"`
	Deletetime         *gtime.Time `json:"deletetime"         orm:"deletetime"         description:"删除时间"`
}
