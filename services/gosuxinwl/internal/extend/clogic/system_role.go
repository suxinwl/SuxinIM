package clogic

import (
	"context"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

/*
  - 1.批量获取子节点id
  - @tablename 数据表名称
    @ids 要获取的id
*/
func GetAllChilIds(tablename string, ids []*gvar.Var) []interface{} {
	var allsubids []interface{}
	for _, id := range ids {
		sub_ids := GetAllChilId(tablename, id)
		allsubids = append(allsubids, sub_ids...)
	}
	return allsubids
}

// 1.2获取所有子级ID
func GetAllChilId(tablename string, id interface{}) []interface{} {
	var subids []interface{}
	sub_ids, _ := g.Model(tablename).Where("pid", id).Array("id")
	if len(sub_ids) > 0 {
		for _, sid := range sub_ids {
			subids = append(subids, sid)
			subids = append(subids, GetAllChilId(tablename, sid)...)
		}
	}
	return subids
}

// 获取账号的数据权限
func GetDataAuthor(ctx context.Context) ([]interface{}, bool) {
	user_id := ctx.Value("uid") //当前用户ID
	var acount_id []interface{} = g.Slice{user_id}
	role_ids, _ := dao.AuthRoleAccess.Ctx(ctx).Where("uid", user_id).Array("role_id")
	data_access, _ := g.Model("auth_role").WhereIn("id", role_ids).Where("status", 0).Array("data_access")
	if IntInVarArray(1, data_access) { //数据权限0=自己1=自己及子权限，2=全部
		chri_role_ids := GetAllChilIds("auth_role", role_ids) //批量获取子节点id
		uid_ids, _ := dao.AuthRoleAccess.Ctx(ctx).WhereIn("role_id", chri_role_ids).Array("uid")
		for _, val := range uid_ids {
			acount_id = append(acount_id, val)
		}
		return acount_id, true //自己及子权限
	} else if IntInVarArray(0, data_access) {
		return acount_id, true //自己
	}
	return acount_id, false //全部
}

// Int类型是否存在Var数组中
func IntInVarArray(target int, arr []*gvar.Var) bool {
	for _, element := range arr {
		if target == gf.Int(element) {
			return true
		}
	}
	return false
}

// 获取子菜单包含的父级ID-返回全部ID
func GetRulesID(tablename string, field string, menus interface{}) interface{} {
	menus_rang := menus.([]interface{})
	var fnemuid []interface{}
	for _, v := range menus_rang {
		fid := getParentID(tablename, field, v)
		if fid != nil {
			fnemuid = MergeArr_interface(fnemuid, fid)
		}
	}
	r_nemu := MergeArr_interface(menus_rang, fnemuid)
	uni_fnemuid := UniqueArr(r_nemu) //去重
	return uni_fnemuid
}

// 获取所有父级ID
func getParentID(tablename string, field string, id interface{}) []interface{} {
	var pids []interface{}
	pid, _ := g.Model(tablename).Where("id", id).Value(field)
	if pid != nil {
		a_pid := pid.Int64()
		var zr_pid int64 = 0
		if a_pid != zr_pid {
			pids = append(pids, a_pid)
			getParentID(tablename, field, pid)
		}
	}
	return pids
}

// 获取菜单树形-打包代码菜单
func GetRuleTreeArrayByPack(list gf.OrmResult, pid int64) gf.OrmResult {
	childs := ToolFar(list, pid) //获取pid下的所有数据
	var chridnum gf.OrmResult
	if !g.IsEmpty(childs) {
		for _, v := range childs {
			newdata := GetRuleTreeArrayByPack(list, v["id"].Int64())
			if newdata != nil {
				v["children"] = gvar.New(GetRuleTreeArrayByPack(list, v["id"].Int64()))
			}
			chridnum = append(chridnum, v)
		}
	}
	return chridnum
}

// base_tool-获取pid下所有数组
func ToolFar(data gf.OrmResult, pid int64) gf.OrmResult {
	var mapString gf.OrmResult
	for _, v := range data {
		if v["pid"].Int64() == pid {
			mapString = append(mapString, v)
		}
	}
	return mapString
}

// 去重
func UniqueArr(datas []interface{}) []interface{} {
	d := make([]interface{}, 0)
	tempMap := make(map[int]bool, len(datas))
	for _, v := range datas { // 以值作为键名
		keyv := gf.Int(v)
		if tempMap[keyv] == false {
			tempMap[keyv] = true
			d = append(d, v)
		}
	}
	return d
}

// 合并数组-interface
func MergeArr_interface(a, b []interface{}) []interface{} {
	var arr []interface{}
	for _, i := range a {
		arr = append(arr, i)
	}
	for _, j := range b {
		arr = append(arr, j)
	}
	return arr
}
