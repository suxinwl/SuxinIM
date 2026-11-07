package adminsystem

import (
	"context"
	"github.com/suxinwl/GoSuxin/api/admin/system"
	"github.com/suxinwl/GoSuxin/internal/accountsecurity"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/internal/extend/clogic"
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/container/gmap"
	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
)

// 获取账号数据
func (s *sAdminsystem) AccountList(ctx context.Context, req *system.AccountListReq) (res *gf.R) {
	//组合搜索条件
	whereMap := gmap.New()
	account_id, filter := clogic.GetDataAuthor(ctx)
	if filter { //需要权限过滤
		whereMap.Set("account_id IN(?)", account_id)
	}
	if req.Name != "" {
		whereMap.Set("name like ?", "%"+gconv.String(req.Name)+"%")
	}
	if !g.IsEmpty(req.Status) {
		whereMap.Set("status =?", req.Status)
	}
	if times := gf.FindTimeCondition(req.Createtime); times != nil {
		whereMap.Set("createtime between ? and ?", times)
	}
	MDB := dao.Admin.Ctx(ctx).Where(whereMap)
	//whereOr条件
	if filter {
		MDB = MDB.WhereOr("id", ctx.Value("uid"))
	}
	totalCount, _ := MDB.Clone().Count()
	list, err := MDB.Fields("id,name,nickname,username,avatar,tel,mobile,email,dept_id,remark,status,createtime").Page(req.Page, req.PageSize).Order("id desc").All()
	if err != nil {
		res = gf.Failed().SetMsg("获取账号数据失败").SetData(err)
		return
	}
	for _, val := range list {
		security,se:=accountsecurity.Default(ctx);if se!=nil{return gf.Failed().SetMsg("账号安全状态暂不可用")};state,se:=security.State(ctx,val["id"].Int64());if se!=nil{return gf.Failed().SetMsg("账号安全状态暂不可用")};val["security"]=gvar.New(state)
		roleid, _ := dao.AuthRoleAccess.Ctx(ctx).Where("uid", val["id"]).Array("role_id")
		rolename, _ := dao.AuthRole.Ctx(ctx).WhereIn("id", roleid).Array("name")
		val["rolename"] = gvar.New(rolename)
		val["roleid"] = gvar.New(roleid)
		depname, _ := dao.AuthDept.Ctx(ctx).Where("id", val["dept_id"]).Value("name")
		val["depname"] = depname
	}
	res = gf.Success().SetMsg("获取账号数据列表").SetData(gf.Map{
		"page":     req.Page,
		"pageSize": req.PageSize,
		"total":    totalCount,
		"items":    list})
	return
}

// 判断账号是否存在
func (s *sAdminsystem) Isaccountexist(ctx context.Context, req *system.IsaccountexistReq) (res *gf.R) {
	if req.Id != gf.ZeroValue {
		res1, err := dao.Admin.Ctx(ctx).Where("id !=", req.Id).Where("username", req.Username).Value("id")
		if err != nil {
			res = gf.Failed().SetMsg("验证失败").SetData(err)
		} else if res1 != nil {
			res = gf.Failed().SetMsg("账号已存在").SetData(err)
		} else {
			res = gf.Success().SetMsg("验证通过").SetData(res1)
		}
	} else {
		res2, err := dao.Admin.Ctx(ctx).Where("username", req.Username).Value("id")
		if err != nil {
			res = gf.Failed().SetMsg("验证失败").SetData(err)
		} else if res2 != nil {
			res = gf.Failed().SetMsg("账号已存在").SetData(err)
		} else {
			res = gf.Success().SetMsg("验证通过").SetData(res2)
		}
	}
	return
}

// 添加、编辑账号数据
func (s *sAdminsystem) AccountSave(ctx context.Context, req *system.AccountSaveReq) (res *gf.R) {
	security, err := accountsecurity.Default(ctx)
	if err != nil {
		return gf.Failed().SetMsg("账号安全服务暂不可用")
	}
	var roles []int64
	if req.Roleid != nil {
		roles, err = roleIDs(req.Roleid)
		if err != nil {
			return gf.Failed().SetMsg(err.Error())
		}
		if roles == nil {
			roles = []int64{}
		}
	}
	fields := map[string]any{"name": req.Name, "username": req.Username, "dept_id": req.DeptId, "tel": gconv.String(req.Tel), "mobile": gconv.String(req.Mobile), "email": gconv.String(req.Email), "remark": req.Remark}
	if !g.IsEmpty(req.Avatar) {
		fields["avatar"] = gconv.String(req.Avatar)
	} else if req.Id == 0 {
		fields["avatar"] = "/resource/static/brand/default-avatar.png?v=puty2"
	}
	id, err := security.SaveAccount(ctx, gconv.Int64(req.Id), gconv.Int64(ctx.Value("uid")), fields, roles, req.Password)
	if err != nil {
		return gf.Failed().SetMsg("账号保存未通过，请检查角色授权及输入内容；已有密码请使用独立重置操作")
	}
	return gf.Success().SetMsg("账号已保存").SetData(map[string]any{"id": id})
}

func (s *sAdminsystem) AccountStatus(ctx context.Context, req *system.AccountStatusReq) (res *gf.R) {
	security, err := accountsecurity.Default(ctx)
	if err == nil {
		err = security.SetAccountStatus(ctx, req.Id, gconv.Int64(ctx.Value("uid")), req.Status, false)
	}
	if err != nil {
		return gf.Failed().SetMsg("账号状态修改未通过")
	}
	return gf.Success().SetMsg("账号状态已更新")
}

func (s *sAdminsystem) AccountDel(ctx context.Context, req *system.AccountDelReq) (res *gf.R) {
	security, err := accountsecurity.Default(ctx)
	if err != nil {
		return gf.Failed().SetMsg("账号安全服务暂不可用")
	}
	for _, value := range req.Ids {
		if err = security.SetAccountStatus(ctx, gconv.Int64(value), gconv.Int64(ctx.Value("uid")), 1, true); err != nil {
			return gf.Failed().SetMsg("账号删除未通过")
		}
	}
	return gf.Success().SetMsg("账号已删除")
}
