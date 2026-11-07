package system

import (
	"github.com/suxinwl/GoSuxin/api/admin/baseapi"
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// 账号列表
type AccountListReq struct {
	g.Meta `path:"/system/account/getList" tags:"getList" method:"get" summary:"账号列表"`
	baseapi.PageReq
	Name       string `p:"name" d:"" dc:"搜索名称"`
	Status     any    `p:"status" d:"" dc:"状态"`
	Createtime string `p:"createtime" d:"" dc:"创建时间"`
}
type AccountListRes struct {
	*gf.R
}

// 判断账号是否存在
type IsaccountexistReq struct {
	g.Meta   `path:"/system/account/isaccountexist" noAuth:"1" tags:"Isaccountexist" method:"post" summary:"判断账号是否存在"`
	Id       int64  `p:"id" d:"0" dc:"编辑时数据Id"`
	Username string `p:"username" v:"required#用户名不能为空" dc:"username"`
}
type IsaccountexistRes struct {
	*gf.R
}

// 保存数据
type AccountSaveReq struct {
	g.Meta    `path:"/system/account/save" tags:"Save" method:"post" summary:"保存、编辑账号"`
	Id        int64  `p:"id" d:"0" dc:"数据id，新增时为0"`
	Name      string `p:"name" v:"required#用户名不能为空" dc:"用户名"`
	Nickname  string `p:"nickname" dc:"昵称"`
	Roleid    any    `p:"roleid" dc:"选择角色"`
	Username  string `p:"username" v:"required#用户账号不能为空" dc:"登录账号"`
	Password  string `p:"password" dc:"登录密码"`
	Salt      string `p:"salt" dc:"密码盐"`
	Mobile    any    `p:"mobile" dc:"手机号码"`
	Tel       any    `p:"tel" dc:"座机"`
	Email     any    `p:"email" dc:"邮箱"`
	Avatar    any    `p:"avatar" dc:"头像"`
	Remark    string `p:"remark" dc:"备注"`
	Weigh     int    `p:"weigh" d:"0" dc:"排序"`
	DeptId    int    `p:"dept_id" d:"0" dc:"部门"`
	AccountId any    `p:"account_id" d:"0" dc:"排序"`
}
type AccountSaveRes struct {
	*gf.R
}

type AccountStatusReq struct {
	g.Meta `path:"/system/account/upStatus" tags:"upStatus" method:"post" summary:"更新状态"`
	Id     int64 `p:"id"  v:"required#id不能为空" dc:"更新id"`
	Status uint  `p:"status" v:"required#status不能为空" dc:"状态"`
}
type AccountStatusRes struct {
	*gf.R
}

type AccountDelReq struct {
	g.Meta `path:"/system/account/del" tags:"Del" method:"delete" summary:"删除"`
	Ids    g.Array `p:"ids"`
}
type AccountDelRes struct {
	*gf.R
}
