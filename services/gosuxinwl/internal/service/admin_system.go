// ================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// You can delete these comments if you wish manually maintain this interface file.
// ================================================================================

package service

import (
	"context"
	"github.com/suxinwl/GoSuxin/api/admin/system"
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
)

type (
	IAdminsystem interface {
		// 获取账号数据
		AccountList(ctx context.Context, req *system.AccountListReq) (res *gf.R)
		// 判断账号是否存在
		Isaccountexist(ctx context.Context, req *system.IsaccountexistReq) (res *gf.R)
		// 添加、编辑账号数据
		AccountSave(ctx context.Context, req *system.AccountSaveReq) (res *gf.R)
		// 更新账号状态
		AccountStatus(ctx context.Context, req *system.AccountStatusReq) (res *gf.R)
		// 删除账号数据
		AccountDel(ctx context.Context, req *system.AccountDelReq) (res *gf.R)
		// 获取部门数据
		DeptList(ctx context.Context, req *system.DeptListReq) (res *gf.R)
		// 获取选项列表
		DeptParent(ctx context.Context, req *system.DeptParentReq) (res *gf.R)
		// 保存、编辑部门
		DeptSave(ctx context.Context, req *system.DeptSaveReq) (res *gf.R)
		DeptLeaderOptions(ctx context.Context, req *system.DeptLeaderOptionsReq) (res *gf.R)
		DeptIMGroup(ctx context.Context, req *system.DeptIMGroupReq) (res *gf.R)
		// 更新状态
		DeptStatus(ctx context.Context, req *system.DeptStatusReq) (res *gf.R)
		// 删除部门
		DeptDel(ctx context.Context, req *system.DeptDelReq) (res *gf.R)
		// 移动部门
		DragDept(ctx context.Context, req *system.DragDeptReq) (res *gf.R)
		// 1.1获取账号数据
		GetLogin(ctx context.Context, req *system.GetLoginReq) (res *gf.R)
		// 1.2删除上个月登录日志
		DelLastLogin(ctx context.Context, in *system.DelLastLoginReq) (res *gf.R)
		// 2.1获取操作日志列表
		GetOperation(ctx context.Context, req *system.GetOperationReq) (res *gf.R)
		// 2.2删除上个月操作日志
		DelLastOperation(ctx context.Context, in *system.DelLastOperationReq) (res *gf.R)
		// 2.3获取操作日志内容
		GetOperationDetail(ctx context.Context, req *system.GetOperationDetailReq) (res *gf.R)
		// 2.4 操作日志写入
		OperationLog(r *ghttp.Request)
		// 获取角色数据
		RoleList(ctx context.Context, req *system.RoleListReq) (res *gf.R)
		// 获取选项列表
		RoleParent(ctx context.Context, req *system.RoleParentReq) (res *gf.R)
		// 编辑角色表单获取菜单
		RoleMenuList(ctx context.Context, req *system.RoleMenuListReq) (res *gf.R)
		// 保存、编辑角色
		RoleSave(ctx context.Context, req *system.RoleSaveReq) (res *gf.R)
		// 更新状态
		RoleStatus(ctx context.Context, req *system.RoleStatusReq) (res *gf.R)
		// 删除菜单
		RoleDel(ctx context.Context, req *system.RoleDelReq) (res *gf.R)
		// 获取菜单数据
		RuleList(ctx context.Context, req *system.RuleListReq) (res *gf.R)
		// 获取选项列表
		RuleParent(ctx context.Context, req *system.RuleParentReq) (res *gf.R)
		// 获取权限选择的路由列表
		RuleRoutes(ctx context.Context, req *system.RuleRoutesReq) (res *gf.R)
		// 保存、编辑菜单
		RuleSave(ctx context.Context, req *system.RuleSaveReq) (res *gf.R)
		// 更新状态
		RuleStatus(ctx context.Context, req *system.RuleStatusReq) (res *gf.R)
		// 删除菜单
		RuleDel(ctx context.Context, req *system.RuleDelReq) (res *gf.R)
		// 获取菜单详情
		RuleContent(ctx context.Context, req *system.RuleContentReq) (res *gf.R)
		// 清空回收站
		EmptyRecyclebin(ctx context.Context, req *system.EmptyRecyclebinReq) (res *gf.R)
	}
)

var (
	localAdminsystem IAdminsystem
)

func Adminsystem() IAdminsystem {
	if localAdminsystem == nil {
		panic("implement not found for interface IAdminsystem, forgot register?")
	}
	return localAdminsystem
}

func RegisterAdminsystem(i IAdminsystem) {
	localAdminsystem = i
}
