package system

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

type RoleListReq struct {
	g.Meta     `path:"/system/role/getList" tags:"getList" method:"get" summary:"角色管理数据"`
	Name       string `p:"name" d:"" dc:"搜索名称"`
	Status     any    `p:"status" d:"" dc:"状态"`
	Createtime string `p:"createtime" d:"" dc:"创建时间"`
}
type RoleListRes struct {
	*gf.R
}

// 获取选项列表
type RoleParentReq struct {
	g.Meta `path:"/system/role/getParent" noAuth:"1" tags:"getParent" method:"get" summary:"获取选项列表"`
	Id     int64 `p:"id" d:"0" dc:"编辑时自己Id"`
}
type RoleParentRes struct {
	*gf.R
}

// 获取选项列表
type RoleMenuListReq struct {
	g.Meta `path:"/system/role/getMenuList" noAuth:"1" tags:"getMenuList" method:"get" summary:"获取选项列表"`
	Pid    int64 `p:"pid" d:"0" dc:"父级id"`
}
type RoleMenuListRes struct {
	*gf.R
}

type RoleSaveReq struct {
	g.Meta     `path:"/system/role/save" tags:"Save" method:"post" summary:"保存、编辑角色"`
	Id         int64  `p:"id" d:"0" dc:"数据id，新增时为0"`
	Pid        int64  `p:"pid" d:"0"`
	Name       string `p:"name" v:"required#角色名称不能为空" dc:"角色名称"`
	Remark     string `p:"remark" d:""`
	Weigh      int    `p:"weigh" d:"1"`
	DataAccess int    `p:"data_access" d:"0" dc:"数据权限"`
	Menu       any    `p:"menu" `
	Rules      any    `p:"rules" `
	Bbtns      any    `p:"btns" `
	AccountId  int64  `p:"account_id" d:"0"`
}

type RoleSaveRes struct {
	*gf.R
}

type RoleStatusReq struct {
	g.Meta `path:"/system/role/upStatus" tags:"upStatus" method:"post" summary:"更新状态"`
	Id     int64 `p:"id"  v:"required#id不能为空" dc:"更新id"`
	Status uint  `p:"status" v:"required#status不能为空" dc:"状态"`
}
type RoleStatusRes struct {
	*gf.R
}

type RoleDelReq struct {
	g.Meta `path:"/system/role/del" tags:"Del" method:"delete" summary:"删除"`
	Ids    g.Array `p:"ids"`
}

type RoleDelRes struct {
	*gf.R
}
