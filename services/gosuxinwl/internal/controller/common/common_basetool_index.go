package common

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/common/basetool"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// 获取使用字典数据接口
func (c *ControllerBasetool) GetDicData(ctx context.Context, req *basetool.GetDicDataReq) (res *basetool.GetDicDataRes, err error) {
	var resData = gf.Failed()
	tablename, _ := dao.DictionaryGroup.Ctx(ctx).Where("id", req.GroupId).Value("tablename")
	getfield := "id,keyname as label,keyvalue as value"
	if gf.DbHaseField(tablename.String(), "tagcolor") {
		getfield = "id,keyname as label,keyvalue as value,tagcolor as color"
	}
	list, err := g.Model(tablename.String()).Where("group_id", req.GroupId).Where("status", 0).Fields(getfield).All()
	if err != nil {
		resData = gf.Failed().SetMsg("获取字典数据失败！").SetData(err)
	} else {
		resData = gf.Success().SetMsg("获取字典数据列表").SetData(list)
	}
	res = &basetool.GetDicDataRes{
		R: resData,
	}
	return
}
