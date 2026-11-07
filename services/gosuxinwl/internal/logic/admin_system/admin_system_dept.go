package adminsystem

import (
	"context"
	"github.com/suxinwl/GoSuxin/api/admin/system"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/internal/imbridge"
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/container/gmap"
	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
)

// 获取部门数据
func (s *sAdminsystem) DeptList(ctx context.Context, req *system.DeptListReq) (res *gf.R) {
	if _, err := departmentStore(ctx); err != nil {
		return gf.Failed().SetMsg("部门通讯设置初始化失败")
	}
	//组合搜索条件
	whereMap := gmap.New()
	if req.Name != "" {
		whereMap.Set("name like ?", "%"+gconv.String(req.Name)+"%")
	}
	if !g.IsEmpty(req.Status) {
		whereMap.Set("status =?", req.Status)
	}
	if times := gf.FindTimeCondition(req.Createtime); times != nil {
		whereMap.Set("createtime between ? and ?", times)
	}
	list, err := dao.AuthDept.Ctx(ctx).Where(whereMap).Order("weigh asc").All()
	if err != nil {
		res = gf.Failed().SetMsg("获取部门数据失败").SetData(err)
		return
	}
	if len(list) > 0 {
		for _, row := range list {
			row["leader_name"] = gvar.New("")
			if uid := row["leader_uid"].Int64(); uid > 0 {
				name, _ := dao.Admin.Ctx(ctx).Where("id", uid).Where("status", 0).Value("name")
				row["leader_name"] = name
			}
		}
		list = gf.GetTreeArray(list, 0, "")
	}
	res = gf.Success().SetMsg("获取部门数据列表").SetData(list)
	return
}

// 获取选项列表
func (s *sAdminsystem) DeptParent(ctx context.Context, req *system.DeptParentReq) (res *gf.R) {
	list, err := dao.AuthDept.Ctx(ctx).Where("status", 0).Where("id !=", req.Id).Fields("id,pid,name,remark").Order("weigh asc").All()
	if err != nil {
		res = gf.Failed().SetMsg("获取父级数据失败").SetData(err)
		return
	}
	if list == nil {
		list = make(gf.OrmResult, 0)
	}
	list = gf.GetMenuChildrenArray(list, 0, "pid")
	res = gf.Success().SetMsg("获取选项列表(父级)").SetData(list)
	return
}

// 保存、编辑部门
func (s *sAdminsystem) DeptSave(ctx context.Context, req *system.DeptSaveReq) (res *gf.R) {
	unlock := imbridge.DepartmentMutationLock()
	defer unlock()
	store, err := departmentStore(ctx)
	if err != nil {
		return gf.Failed().SetMsg("部门通讯设置初始化失败")
	}
	enabled, leader, status := 0, int64(0), 0
	if req.Id > 0 {
		row, e := dao.AuthDept.Ctx(ctx).Where("id", req.Id).One()
		if e != nil || len(row) == 0 {
			return gf.Failed().SetMsg("部门不存在或已删除")
		}
		enabled, leader = row["im_group_enabled"].Int(), row["leader_uid"].Int64()
		status = row["status"].Int()
	}
	if req.IMGroupEnabled != nil {
		enabled = *req.IMGroupEnabled
	}
	if req.LeaderUID != nil {
		leader = *req.LeaderUID
	}
	if req.Status != nil {
		status = *req.Status
	}
	if enabled != 0 && enabled != 1 || leader < 0 {
		return gf.Failed().SetMsg("部门群开关或负责人参数无效")
	}
	if status != 0 && status != 1 {
		return gf.Failed().SetMsg("部门状态参数无效")
	}
	if enabled == 1 {
		if status != 0 {
			return gf.Failed().SetMsg("停用的部门不能开启部门群")
		}
		id := req.Id
		if id == 0 {
			id = int64(req.Pid)
		}
		if e := store.ValidateDepartmentGroup(ctx, id, leader); e != nil {
			return gf.Failed().SetMsg(e.Error())
		}
	}
	data := g.Map{"name": req.Name, "pid": req.Pid, "remark": req.Remark, "status": status,
		"weigh": req.Weigh, "im_group_enabled": enabled, "leader_uid": leader}
	if req.Id == gf.ZeroValue {
		business := int64(0)
		if req.Pid > 0 {
			parent, e := dao.AuthDept.Ctx(ctx).Where("id", req.Pid).Where("status", 0).One()
			if e != nil || len(parent) == 0 {
				return gf.Failed().SetMsg("上级部门不存在或已停用")
			}
			business = parent["business_id"].Int64()
		}
		data["business_id"] = business
		addId, err := dao.AuthDept.Ctx(ctx).Data(data).InsertAndGetId()
		if err != nil {
			res = gf.Failed().SetMsg("添加部门失败").SetData(err)
		} else {
			if addId != 0 {
				dao.AuthDept.Ctx(ctx).Where("id", addId).Update(map[string]interface{}{"weigh": addId})
			}
			res = gf.Success().SetMsg("添加成功！")
		}
	} else {
		result, err := dao.AuthDept.Ctx(ctx).Data(data).Where("id", req.Id).Update()
		if err != nil {
			res = gf.Failed().SetMsg("更新部门失败").SetData(err)
		} else {
			res = gf.Success().SetMsg("更新成功！").SetData(result)
		}
	}
	imbridge.NotifyDepartmentsChanged()
	return
}

func departmentStore(ctx context.Context) (*imbridge.SQLStore, error) {
	db := dao.AuthDept.DB()
	sqlDB, err := db.Master()
	if err != nil {
		return nil, err
	}
	prefix := db.GetConfig().Prefix
	if err = imbridge.EnsureDepartmentSchema(ctx, sqlDB, prefix); err != nil {
		return nil, err
	}
	return imbridge.NewSQLStore(sqlDB, prefix)
}

func (s *sAdminsystem) DeptLeaderOptions(ctx context.Context, req *system.DeptLeaderOptionsReq) *gf.R {
	if len(req.Keyword) > 128 || req.DeptID < 0 || req.Id < 0 || req.ParentID < 0 {
		return gf.Failed().SetMsg("查询参数无效")
	}
	store, err := departmentStore(ctx)
	if err != nil {
		return gf.Failed().SetMsg("部门通讯设置初始化失败")
	}
	scopeDept := req.DeptID
	if scopeDept == 0 {
		scopeDept = req.Id
	}
	if scopeDept == 0 {
		scopeDept = req.ParentID
	}
	value, err := store.DepartmentLeaderOptions(ctx, req.Keyword, scopeDept, req.Page, req.PageSize)
	if err != nil {
		return gf.Failed().SetMsg("通讯员工候选暂不可用，请确认通讯服务已开通")
	}
	return gf.Success().SetData(value)
}

func (s *sAdminsystem) DeptIMGroup(ctx context.Context, req *system.DeptIMGroupReq) *gf.R {
	unlock := imbridge.DepartmentMutationLock()
	defer unlock()
	if req.Id <= 0 || req.Enabled == nil || (*req.Enabled != 0 && *req.Enabled != 1) {
		return gf.Failed().SetMsg("部门群开关参数无效")
	}
	store, err := departmentStore(ctx)
	if err != nil {
		return gf.Failed().SetMsg("部门通讯设置初始化失败")
	}
	row, err := dao.AuthDept.Ctx(ctx).Where("id", req.Id).One()
	if err != nil || len(row) == 0 {
		return gf.Failed().SetMsg("部门不存在或已删除")
	}
	if *req.Enabled == 1 {
		if err = store.ValidateDepartmentGroup(ctx, req.Id, row["leader_uid"].Int64()); err != nil {
			return gf.Failed().SetMsg(err.Error())
		}
	}
	if _, err = dao.AuthDept.Ctx(ctx).Where("id", req.Id).Update(g.Map{"im_group_enabled": *req.Enabled}); err != nil {
		return gf.Failed().SetMsg("更新部门群开关失败")
	}
	imbridge.NotifyDepartmentsChanged()
	return gf.Success().SetData(g.Map{"id": req.Id, "im_group_enabled": *req.Enabled})
}

// 更新状态
func (s *sAdminsystem) DeptStatus(ctx context.Context, req *system.DeptStatusReq) (res *gf.R) {
	unlock := imbridge.DepartmentMutationLock()
	defer unlock()
	result, err := dao.AuthDept.Ctx(ctx).Where("id", req.Id).Update(gf.Map{"status": req.Status})
	if err != nil {
		res = gf.Failed().SetMsg("更新失败").SetData(err)
		return
	}
	msg := "更新成功！"
	if result == nil {
		msg = "暂无数据更新"
	}
	res = gf.Success().SetMsg(msg)
	imbridge.NotifyDepartmentsChanged()
	return
}

// 删除部门
func (s *sAdminsystem) DeptDel(ctx context.Context, req *system.DeptDelReq) (res *gf.R) {
	unlock := imbridge.DepartmentMutationLock()
	defer unlock()
	result, err := dao.AuthDept.Ctx(ctx).WhereIn("id", req.Ids).Delete()
	if err != nil {
		res = gf.Failed().SetMsg("删除部门失败").SetData(err)
		return
	}
	//删除子类数据
	dao.AuthDept.Ctx(ctx).WhereIn("pid", req.Ids).Delete()
	res = gf.Success().SetMsg("删除部门成功").SetData(result)
	imbridge.NotifyDepartmentsChanged()
	return
}

// 移动部门
func (s *sAdminsystem) DragDept(ctx context.Context, req *system.DragDeptReq) (res *gf.R) {
	if req.Id == req.Pid {
		res = gf.Failed().SetMsg("同一部门无法绑定")
		return
	}
	_, err := dao.AuthDept.Ctx(ctx).Where("id", req.Id).Update(gf.Map{"pid": req.Pid})
	if err != nil {
		res = gf.Failed().SetMsg("移动部门失败").SetData(err)
		return
	}
	res = gf.Success().SetMsg("移动部门成功")
	return
}
