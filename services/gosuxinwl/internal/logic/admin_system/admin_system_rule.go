package adminsystem

import (
	"context"
	"github.com/suxinwl/GoSuxin/api/admin/system"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/internal/extend/clogic"
	"github.com/suxinwl/GoSuxin/internal/model/entity"
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// 获取菜单数据
func (s *sAdminsystem) RuleList(ctx context.Context, req *system.RuleListReq) (res *gf.R) {
	menuList, err := dao.AuthRule.Ctx(ctx).Fields("id,pid,type,title,locale,icon,permission,path,component,weigh,status,createtime").Order("weigh asc").All()
	if err != nil {
		res = gf.Failed().SetMsg("获取菜单数据失败").SetData(err)
		return
	}
	for _, val := range menuList {
		if val["title"].String() == "" {
			val["title"] = val["locale"]
		}
	}
	menuList = gf.GetTreeArray(menuList, 0, "")
	res = gf.Success().SetMsg("获取全部菜单列表").SetData(menuList)
	return
}

// 获取选项列表
func (s *sAdminsystem) RuleParent(ctx context.Context, req *system.RuleParentReq) (res *gf.R) {
	menuList, err := dao.AuthRule.Ctx(ctx).WhereIn("type", g.Slice{0, 1}).Where("id !=", req.Id).Fields("id,pid,title,locale,routepath").Order("weigh asc").All()
	if err != nil {
		res = gf.Failed().SetMsg("获取菜单数据失败").SetData(err)
		return
	}
	if menuList == nil {
		menuList = make(gf.OrmResult, 0)
	}
	for _, val := range menuList {
		if val["title"].String() == "" {
			val["title"] = val["locale"]
		}
	}
	menuTree := gf.GetMenuChildrenArray(menuList, 0, "pid")
	res = gf.Success().SetMsg("获取选项列表(父级)").SetData(gf.Map{"tree": menuTree, "list": menuList})
	return
}

// 获取权限选择的路由列表
func (s *sAdminsystem) RuleRoutes(ctx context.Context, req *system.RuleRoutesReq) (res *gf.R) {
	data := make([]gf.Map, 0)
	res = gf.Success().SetMsg("获取权限选择的路由列表").SetData(data)
	return
}

// 保存、编辑菜单
func (s *sAdminsystem) RuleSave(ctx context.Context, req *system.RuleSaveReq) (res *gf.R) {
	if req.Id == gf.ZeroValue {
		req.Uid = ctx.Value("uid")
		addId, err := dao.AuthRule.Ctx(ctx).Data(req).FieldsEx("id").InsertAndGetId()
		if err != nil {
			res = gf.Failed().SetMsg("添加菜单失败").SetData(err)
		} else {
			if addId != 0 {
				dao.AuthRule.Ctx(ctx).Where("id", addId).Update(map[string]interface{}{"weigh": addId})
			}
			res = gf.Success().SetMsg("添加成功！")
		}
	} else {
		result, err := dao.AuthRule.Ctx(ctx).Data(req).Where("id", req.Id).Update()
		if err != nil {
			res = gf.Failed().SetMsg("更新菜单失败").SetData(err)
		} else {
			res = gf.Success().SetMsg("更新成功！").SetData(result)
		}
	}
	return
}

// 更新状态
func (s *sAdminsystem) RuleStatus(ctx context.Context, req *system.RuleStatusReq) (res *gf.R) {
	result, err := dao.AuthRule.Ctx(ctx).Where("id", req.Id).Update(gf.Map{"status": req.Status})
	if err != nil {
		res = gf.Failed().SetMsg("更新失败").SetData(err)
		return
	}
	msg := "更新成功！"
	if result == nil {
		msg = "暂无数据更新"
	}
	res = gf.Success().SetMsg(msg)
	return
}

// 删除菜单
func (s *sAdminsystem) RuleDel(ctx context.Context, req *system.RuleDelReq) (res *gf.R) {
	if gf.Int64(req.Ids[0]) == 1 {
		res = gf.Failed().SetMsg("首页不允许删除")
		return
	}
	result, err := dao.AuthRule.Ctx(ctx).WhereIn("id", req.Ids).Delete()
	if err != nil {
		res = gf.Failed().SetMsg("删除菜单失败").SetData(err)
		return
	}
	//删除子类数据
	clogic.DelTreeData("auth_rule", req.Ids)
	res = gf.Success().SetMsg("删除菜单成功").SetData(result)
	return
}

// 获取菜单详情
func (s *sAdminsystem) RuleContent(ctx context.Context, req *system.RuleContentReq) (res *gf.R) {
	data := entity.AuthRule{}
	err := dao.AuthRule.Ctx(ctx).Where("id", req.Id).Scan(&data)
	if err != nil {
		res = gf.Failed().SetMsg("获取内容失败").SetData(err)
		return
	}
	res = gf.Success().SetMsg("获取菜单详情").SetData(data)
	return
}

// 清空回收站
func (s *sAdminsystem) EmptyRecyclebin(ctx context.Context, req *system.EmptyRecyclebinReq) (res *gf.R) {
	_, err := dao.AuthRule.Ctx(ctx).Unscoped().WhereNotNull("deletetime").Delete()
	if err != nil {
		res = gf.Failed().SetMsg("清空回收站失败").SetData(err)
		return
	}
	res = gf.Success().SetMsg("清空回收站成功")
	return
}
