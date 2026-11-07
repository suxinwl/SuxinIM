package gf

import (
	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// 获取数据表下的字段值
func GetTalbeFieldVal(tablename, field, id interface{}) *gvar.Var {
	data, _ := g.Model(tablename).Where("id", id).Value(field)
	return gvar.New(data)
}

// 获取树结构数据
func GetTreeData(pdata OrmResult, parent_id int64, pid_file string) OrmResult {
	var returnList OrmResult
	for _, v := range pdata {
		if v[pid_file].Int64() == parent_id {
			children := GetTreeData(pdata, v["id"].Int64(), pid_file)
			if children != nil {
				v["children"] = gvar.New(children)
			}
			returnList = append(returnList, v)
		}
	}
	if returnList == nil {
		returnList = make(OrmResult, 0)
	}
	return returnList
}

// 获取字典数据下的字段值
func GetDicFieldVal(group_id, val interface{}) *gvar.Var {
	if String(val) == "" {
		return gvar.New(nil)
	}
	tablename, _ := g.Model("dictionary_group").Where("id", group_id).Value("tablename")
	data, _ := g.Model(tablename.String()).Where("group_id", group_id).Where("keyvalue", val).Fields("keyname,tagcolor").One()
	return gvar.New(data)
}

// 只获取字典数据下的字段值
func GetDicVal(group_id, val interface{}) *gvar.Var {
	if String(val) == "" {
		return gvar.New(nil)
	}
	tablename, _ := g.Model("dictionary_group").Where("id", group_id).Value("tablename")
	data, _ := g.Model(tablename.String()).Where("group_id", group_id).Where("keyvalue", val).Value("keyname")
	return gvar.New(data)
}

// 1.1 获取分类下全部子id
func CateAllChilId(tablename string, cid interface{}) []interface{} {
	cids := GetAllChilId(tablename, cid)
	return append(cids, cid)
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
