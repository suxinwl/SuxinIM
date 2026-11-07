package adminsystem

import (
	"context"
	"github.com/suxinwl/GoSuxin/api/admin/system"
	"github.com/suxinwl/GoSuxin/internal/accountsecurity"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/internal/extend/clogic"
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/container/gmap"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
)

// 获取角色数据
func (s *sAdminsystem) RoleList(ctx context.Context, req *system.RoleListReq) (res *gf.R) {
	uid := ctx.Value("uid") //当前用户ID
	user_role_ids, _ := dao.AuthRoleAccess.Ctx(ctx).Where("uid", uid).Array("role_id")
	role_chil_ids := clogic.GetAllChilIds("auth_role", user_role_ids) //批量获取子节点id
	all_role_id := gf.MergeArr(user_role_ids, role_chil_ids)
	//获取账号权限
	account_id, _ := clogic.GetDataAuthor(ctx)
	account_id = append(account_id, 0)
	//获取自己权限组-显示自己所在的权限组
	my_role_account_id, _ := dao.AuthRole.Ctx(ctx).WhereIn("id", user_role_ids).Value("account_id")
	account_id = append(account_id, my_role_account_id)
	//组合搜索条件
	whereMap := gmap.New()
	whereMap.Set("id IN(?)", all_role_id) //in 查询
	whereMap.Set("account_id IN(?)", account_id)
	if req.Name != "" {
		whereMap.Set("name like ?", "%"+gconv.String(req.Name)+"%")
	}
	if !g.IsEmpty(req.Status) {
		whereMap.Set("status =?", req.Status)
	}
	if times := gf.FindTimeCondition(req.Createtime); times != nil {
		whereMap.Set("createtime between ? and ?", times)
	}
	roleList, _ := dao.AuthRole.Ctx(ctx).Where(whereMap).Order("weigh asc").All()
	//获取最大一级的pid
	max_role_id, _ := dao.AuthRole.Ctx(ctx).Where(whereMap).Order("id asc").Value("pid")
	roleList = gf.GetTreeArray(roleList, gf.Int64(max_role_id), "")
	if roleList == nil {
		roleList = make(gf.OrmResult, 0)
	}
	res = gf.Success().SetMsg("获取拥有角色列表").SetData(gf.Map{"list": roleList, "max_pid": max_role_id})
	return
}

// 获取选项列表
func (s *sAdminsystem) RoleParent(ctx context.Context, req *system.RoleParentReq) (res *gf.R) {
	uid := ctx.Value("uid")
	user_role_ids, _ := dao.AuthRoleAccess.Ctx(ctx).Where("uid", uid).Array("role_id")
	role_chil_ids := clogic.GetAllChilIds("auth_role", user_role_ids) //批量获取子节点id
	all_role_id := gf.MergeArr(user_role_ids, role_chil_ids)
	//获取数据权限

	account_id, _ := clogic.GetDataAuthor(ctx)
	account_id = append(account_id, 0)
	//获取自己权限组-显示自己所在的权限组
	my_role_account_id, _ := dao.AuthRole.Ctx(ctx).WhereIn("id", user_role_ids).Value("account_id")
	account_id = append(account_id, my_role_account_id)
	//查找条件
	whereMap := gmap.New()
	whereMap.Set("id IN(?)", all_role_id)        //in 查询
	whereMap.Set("account_id IN(?)", account_id) //账号数据权限
	if req.Id != gf.ZeroValue {
		whereMap.Set("id !=?", req.Id)
	}
	roleList, _ := dao.AuthRole.Ctx(ctx).Where(whereMap).Fields("id,pid,name").Order("weigh asc").All()
	//获取最大一级的pid
	max_role_id, _ := dao.AuthRole.Ctx(ctx).Where(whereMap).Order("id asc").Value("pid")
	roleList = gf.GetTreeArray(roleList, gf.Int64(max_role_id), "")
	if roleList == nil {
		roleList = make(gf.OrmResult, 0)
	}
	res = gf.Success().SetMsg("获取选项列表(父级)").SetData(roleList)
	return
}

// 编辑角色表单获取菜单
func (s *sAdminsystem) RoleMenuList(ctx context.Context, req *system.RoleMenuListReq) (res *gf.R) {
	roles, rules, access := hostRoleModels(ctx)
	return roleMenuChoices(ctx, roles, rules, access, gf.Int64(ctx.Value("uid")), req.Pid)
}

// 保存、编辑角色
func (s *sAdminsystem) RoleSave(ctx context.Context, req *system.RoleSaveReq) (res *gf.R) {
	roles, rules, access := hostRoleModels(ctx)
	security, err := accountsecurity.Default(ctx)
	if err != nil {
		return gf.Failed().SetMsg("账号安全服务暂不可用")
	}
	id, err := saveRoleRecord(ctx, roles, rules, access, gf.Int64(ctx.Value("uid")), req, security)
	if err != nil {
		return gf.Failed().SetMsg(err.Error())
	}
	return gf.Success().SetMsg("角色保存成功").SetData(id)
}

// 更新状态
func (s *sAdminsystem) RoleStatus(ctx context.Context, req *system.RoleStatusReq) (res *gf.R) {
	security, err := accountsecurity.Default(ctx)
	if err != nil {
		return gf.Failed().SetMsg("账号安全服务暂不可用")
	}
	if err := mutateRoleRecords(ctx, dao.AuthRole.Ctx(ctx), dao.AuthRoleAccess.Ctx(ctx), gf.Int64(ctx.Value("uid")), []int64{req.Id}, false, req.Status, security); err != nil {
		return gf.Failed().SetMsg(err.Error())
	}
	return gf.Success().SetMsg("角色状态更新成功")
}

// 删除菜单
func (s *sAdminsystem) RoleDel(ctx context.Context, req *system.RoleDelReq) (res *gf.R) {
	ids, err := roleIDs(req.Ids)
	security, se := accountsecurity.Default(ctx)
	if se != nil {
		return gf.Failed().SetMsg("账号安全服务暂不可用")
	}
	if err == nil {
		err = mutateRoleRecords(ctx, dao.AuthRole.Ctx(ctx), dao.AuthRoleAccess.Ctx(ctx), gf.Int64(ctx.Value("uid")), ids, true, 0, security)
	}
	if err != nil {
		return gf.Failed().SetMsg(err.Error())
	}
	return gf.Success().SetMsg("角色删除成功")
}
