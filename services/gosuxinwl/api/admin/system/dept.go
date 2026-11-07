package system

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

type DeptListReq struct {
	g.Meta     `path:"/system/dept/getList" tags:"Getlist" method:"get" summary:"获取部门数据"`
	Name       string `p:"name" d:"" dc:"搜索名称"`
	Status     any    `p:"status" d:"" dc:"状态"`
	Createtime string `p:"createtime" d:"" dc:"创建时间"`
}
type DeptListRes struct {
	*gf.R
}

// 获取选项列表
type DeptParentReq struct {
	g.Meta `path:"/system/dept/getParent" noAuth:"1" tags:"getParent" method:"get" summary:"获取选项列表"`
	Id     int64 `p:"id" d:"0" dc:"编辑时自己Id"`
}
type DeptParentRes struct {
	*gf.R
}

// 保存数据
type DeptSaveReq struct {
	g.Meta         `path:"/system/dept/save" tags:"Save" method:"post" summary:"保存、编辑部门"`
	Id             int64  `p:"id" d:"0" dc:"数据id，新增时为0"`
	Name           string `p:"name" v:"required#部门名称不能为空" dc:"部门名称"`
	Pid            int    `p:"pid" d:"0" dc:"上级部门id"`
	Remark         string `p:"remark" dc:"备注"`
	Status         *int   `p:"status" dc:"状态，编辑未传则保留"`
	Weigh          int    `p:"weigh" d:"0" dc:"排序"`
	IMGroupEnabled *int   `p:"im_group_enabled" dc:"部门群自动管理开关0或1"`
	LeaderUID      *int64 `p:"leader_uid" dc:"部门负责人宿主账号ID"`
}
type DeptSaveRes struct {
	*gf.R
}

type DeptLeaderOptionsReq struct {
	g.Meta   `path:"/system/dept/getLeaderOptions" method:"get" summary:"已开通通讯的部门负责人候选"`
	Keyword  string `p:"keyword"`
	DeptID   int64  `p:"dept_id" d:"0"`
	Id       int64  `p:"id" d:"0" dc:"编辑部门ID别名"`
	ParentID int64  `p:"parent_id" d:"0" dc:"新部门依据父部门过滤"`
	Page     int    `p:"page" d:"1"`
	PageSize int    `p:"pageSize" d:"20"`
}
type DeptLeaderOptionsRes struct{ *gf.R }
type DeptIMGroupReq struct {
	g.Meta  `path:"/system/dept/upIMGroup" method:"post" summary:"更新部门群开关"`
	Id      int64 `p:"id" v:"required"`
	Enabled *int  `p:"im_group_enabled"`
}
type DeptIMGroupRes struct{ *gf.R }

type DeptStatusReq struct {
	g.Meta `path:"/system/dept/upStatus" tags:"upStatus" method:"post" summary:"更新状态"`
	Id     int64 `p:"id"  v:"required#id不能为空" dc:"更新id"`
	Status uint  `p:"status" v:"required#status不能为空" dc:"状态"`
}
type DeptStatusRes struct {
	*gf.R
}

type DeptDelReq struct {
	g.Meta `path:"/system/dept/del" tags:"Del" method:"delete" summary:"删除"`
	Ids    g.Array `p:"ids"`
}
type DeptDelRes struct {
	*gf.R
}

type DragDeptReq struct {
	g.Meta `path:"/system/dept/dragDept" tags:"dragDept" method:"post" summary:"移动部门"`
	Id     int64 `p:"id" v:"required#id不能为空" dc:"更新id"`
	Pid    int64 `p:"pid" v:"required#pid不能为空" dc:"父级id"`
}
type DragDeptRes struct {
	*gf.R
}
