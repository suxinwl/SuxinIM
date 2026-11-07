package admindatacenter

import (
	"context"
	"github.com/suxinwl/GoSuxin/api/admin/datacenter"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// 获取字典分类数据
func (s *sAdmindatacenter) TabledataList(ctx context.Context, req *datacenter.TabledataListReq) (res *gf.R) {
	list, err := dao.DictionaryGroup.Ctx(ctx).Fields("id,title,remark,tablename,status,weigh,db_way").Order("weigh asc").All()
	if err != nil {
		res = gf.Failed().SetMsg("获取字典分类数据失败").SetData(err)
		return
	}
	res = gf.Success().SetMsg("获取字典分类数据").SetData(list)
	return
}

// 保存、编辑字典分组
func (s *sAdmindatacenter) TabledataSave(ctx context.Context, req *datacenter.TabledataSaveReq) (res *gf.R) {
	if req.Id == gf.ZeroValue {
		if req.BbWay == "sys" {
			req.Tablename = "dictionary_data"
		}
		addId, err := dao.DictionaryGroup.Ctx(ctx).Data(req).FieldsEx("id").InsertAndGetId()
		if err != nil {
			res = gf.Failed().SetMsg("添加字典分组失败").SetData(err)
		} else {
			if addId != 0 {
				dao.DictionaryGroup.Ctx(ctx).Where("id", addId).Update(map[string]interface{}{"weigh": addId})
			}
			res = gf.Success().SetMsg("添加成功！")
		}
	} else {
		result, err := dao.DictionaryGroup.Ctx(ctx).Data(req).Where("id", req.Id).Update()
		if err != nil {
			res = gf.Failed().SetMsg("更新字典分组失败").SetData(err)
		} else {
			res = gf.Success().SetMsg("更新成功！").SetData(result)
		}
	}
	return
}

// 删除字典分组
func (s *sAdmindatacenter) TabledataDel(ctx context.Context, req *datacenter.TabledataDelReq) (res *gf.R) {
	tablename, _ := dao.DictionaryGroup.Ctx(ctx).WhereIn("id", req.Ids).Value("tablename")
	result, err := dao.DictionaryGroup.Ctx(ctx).WhereIn("id", req.Ids).Delete()
	if err != nil {
		res = gf.Failed().SetMsg("删除字典分组失败").SetData(err)
		return
	}
	//删除字典数据
	g.Model(tablename.String()).WhereIn("group_id", req.Ids).Delete()
	res = gf.Success().SetMsg("删除字典分组成功").SetData(result)
	return
}
