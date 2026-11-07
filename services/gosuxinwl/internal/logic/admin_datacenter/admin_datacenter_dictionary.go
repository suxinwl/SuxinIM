package admindatacenter

import (
	"context"
	"encoding/json"
	"github.com/suxinwl/GoSuxin/api/admin/datacenter"
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/container/gmap"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// 获取字典数据
func (s *sAdmindatacenter) DictionaryList(ctx context.Context, req *datacenter.DictionaryListReq) (res *gf.R) {
	//组合搜索条件
	whereMap := gmap.New()
	whereMap.Set("group_id", req.GroupId)
	if !g.IsEmpty(req.Title) {
		whereMap.Set("keyname like ?", "%"+req.Title+"%")
	}
	if !g.IsEmpty(req.Status) {
		whereMap.Set("status", req.Status)
	}
	if times := gf.FindTimeCondition(req.Createtime); times != nil {
		whereMap.Set("createtime between ? and ?", times)
	}
	MDB := g.Model(req.Tablename).Where(whereMap)
	totalCount, _ := MDB.Clone().Count()
	list, err := MDB.Page(req.Page, req.PageSize).Order("id asc").All()
	if err != nil {
		res = gf.Failed().SetMsg("获取字典数据失败").SetData(err)
		return
	}
	res = gf.Success().SetMsg("获取字典列表").SetData(gf.Map{
		"page":     req.Page,
		"pageSize": req.PageSize,
		"total":    totalCount,
		"items":    list})
	return
}

// 保存、编辑字典数据
func (s *sAdmindatacenter) DictionarySave(ctx context.Context, req *datacenter.DictionarySaveReq) (res *gf.R) {
	if req.Id == gf.ZeroValue {
		addId, err := g.Model(req.Tablename).Data(req).FieldsEx("id").InsertAndGetId()
		if err != nil {
			res = gf.Failed().SetMsg("添加字典数据失败").SetData(err)
		} else {
			if addId != 0 {
				g.Model(req.Tablename).Where("id", addId).Update(map[string]interface{}{"weigh": addId})
			}
			res = gf.Success().SetMsg("添加成功！")
		}
	} else {
		result, err := g.Model(req.Tablename).Data(req).Where("id", req.Id).Update()
		if err != nil {
			res = gf.Failed().SetMsg("更新字典数据失败").SetData(err)
		} else {
			res = gf.Success().SetMsg("更新成功！").SetData(result)
		}
	}
	return
}

// 更新状态
func (s *sAdmindatacenter) DictionaryStatus(ctx context.Context, req *datacenter.DictionaryStatusReq) (res *gf.R) {
	result, err := g.Model(req.Tablename).Where("id", req.Id).Update(gf.Map{"status": req.Status})
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

// 删除字典数据
func (s *sAdmindatacenter) DictionaryDel(ctx context.Context, req *datacenter.DictionaryDelReq) (res *gf.R) {
	result, err := g.Model(req.Tablename).WhereIn("id", req.Ids).Delete()
	if err != nil {
		res = gf.Failed().SetMsg("删除字典数据失败").SetData(err)
		return
	}
	res = gf.Success().SetMsg("删除字典数据成功").SetData(result)
	return
}

// 使用数据表数据(表单生成使用)
func (s *sAdmindatacenter) GetTableDataForm(ctx context.Context, req *datacenter.GetTableDataFormReq) (res *gf.R) {
	var custom interface{}
	if !g.IsEmpty(req.Custom) {
		custom = gf.StringToJSON(gf.String(req.Custom))
	}
	if gf.DbHaseField(req.Tablename, "pid") {
		list, err := g.Model(req.Tablename).Where(custom).Fields("id,id as value,pid," + req.Showfield + " as label").All()
		if err != nil {
			res = gf.Failed().SetMsg("使用数据表数据失败！").SetData(err)
		} else {
			list = gf.GetTreeArray(list, 0, "")
			dataTolist := list.Json()
			var parameter []gf.Map
			_ = json.Unmarshal([]byte(dataTolist), &parameter)
			listarray := gf.GetTreeToList(parameter, "label")
			res = gf.Success().SetMsg("使用数据表数据列表").SetData(listarray)
		}
	} else {
		list, err := g.Model(req.Tablename).Where(custom).Fields("id as value," + req.Showfield + " as label").All()
		if err != nil {
			res = gf.Failed().SetMsg("使用数据表数据失败！").SetData(err)
		} else {
			res = gf.Success().SetMsg("使用数据表数据列表").SetData(list)
		}
	}
	return
}
