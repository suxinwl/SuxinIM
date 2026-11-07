// ==========================
// 删除子数据
// ==========================
package clogic

import "github.com/suxinwl/GoSuxin/framework/frame/g"

// 删除全部子数据
func DelTreeData(tablename string, pid interface{}) (err error) {
	//1.获取子类id
	sub_ids, err := g.Model(tablename).WhereIn("pid", pid).Array("id")
	if err != nil {
		return
	}
	//执行删除bin递归子类数据
	if len(sub_ids) > 0 {
		g.Model(tablename).WhereIn("pid", pid).Delete()
		DelTreeData(tablename, sub_ids)
	}
	return
}
