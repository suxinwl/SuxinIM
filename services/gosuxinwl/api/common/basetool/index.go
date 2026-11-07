package basetool

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

type GetDicDataReq struct {
	g.Meta  `path:"/basetool/getDicData" tags:"getDicData" method:"get" summary:"获取字典数据"`
	GroupId string `p:"group_id" v:"required#分组id不能为空" dc:"分组id"`
}

type GetDicDataRes struct {
	*gf.R
}
