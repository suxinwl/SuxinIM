package system

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// 菜单列表
type RuleListReq struct {
	g.Meta `path:"/system/rule/getList" tags:"getList" method:"get" summary:"菜单列表"`
}
type RuleListRes struct {
	*gf.R
}

// 获取选项列表
type RuleParentReq struct {
	g.Meta `path:"/system/rule/getParent" noAuth:"1" tags:"getParent" method:"get" summary:"获取选项列表"`
	Id     int64 `p:"id" d:"0" dc:"父级菜单Id"`
}
type RuleParentRes struct {
	*gf.R
}

// 获取权限选择的路由列表
type RuleRoutesReq struct {
	g.Meta `path:"/system/rule/getRoutes" noAuth:"1" tags:"getRoutes" method:"get" summary:"获取权限选择的路由列表"`
}
type RuleRoutesRes struct {
	*gf.R
}

type RuleSaveReq struct {
	g.Meta             `path:"/system/rule/save" tags:"Save" method:"post" summary:"保存、编辑菜单"`
	Id                 int64  `p:"id" d:"0" dc:"数据id，新增时为0"`
	Uid                any    `p:"uid" d:"0"`
	Activemenu         int    `p:"activemenu" d:"0"`
	Component          string `p:"component"`
	Des                string `p:"des"`
	Hidechildreninmenu int    `p:"hidechildreninmenu" d:"0"`
	Hideinmenu         int    `p:"hideinmenu" d:"0"`
	Icon               string `p:"icon"`
	Isext              int    `p:"isext" d:"0"`
	Keepalive          int    `p:"keepalive" d:"0"`
	Locale             string `p:"locale"`
	Noaffix            int    `p:"noaffix" d:"0"`
	Onlypage           int    `p:"onlypage" d:"0"`
	Path               string `p:"path"`
	Permission         string `p:"permission"`
	Pid                int    `p:"pid" d:"0"`
	Redirect           string `p:"redirect"`
	Requiresauth       int    `p:"requiresauth" d:"0"`
	Routename          string `p:"routename"`
	Routepath          string `p:"routepath"`
	Title              string `p:"title" v:"required#菜单名称不能为空" dc:"菜单名称"`
	Type               int    `p:"type" d:"0"`
	Weigh              int    `p:"weigh" d:"1"`
}
type RuleSaveRes struct {
	*gf.R
}

type RuleStatusReq struct {
	g.Meta `path:"/system/rule/upStatus" tags:"upStatus" method:"post" summary:"更新状态"`
	Id     int64 `p:"id"  v:"required#id不能为空" dc:"更新id"`
	Status uint  `p:"status" v:"required#status不能为空" dc:"状态"`
}
type RuleStatusRes struct {
	*gf.R
}

type RuleDelReq struct {
	g.Meta `path:"/system/rule/del" tags:"Del" method:"delete" summary:"删除"`
	Ids    g.Array `p:"ids"`
}
type RuleDelRes struct {
	*gf.R
}

type EmptyRecyclebinReq struct {
	g.Meta `path:"/system/rule/emptyRecyclebin" tags:"EmptyRecyclebin" method:"delete" summary:"清空回收站"`
}
type EmptyRecyclebinRes struct {
	*gf.R
}

type RuleContentReq struct {
	g.Meta `path:"/system/rule/getContent" tags:"getContent" method:"get" summary:"获取内容详情"`
	Id     int64 `p:"id" d:"0" v:"required#id不能为空" dc:"查找数据id"`
}
type RuleContentRes struct {
	*gf.R
}
