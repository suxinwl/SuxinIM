package basetool

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

type WeighReq struct {
	g.Meta    `path:"/basetool/table/weigh" tags:"Weigh" method:"post" summary:"table排序"`
	Id        int64  `p:"id" d:"0"`                        // 数据的id
	Pid       string `p:"pid" d:"0"`                       // 父级id
	Tableanme string `p:"tableanme" v:"required#数据表名不能为空"` // 数据表名
	WeighList any    `p:"weighList" v:"required#排序数据不能为空"` // 排序数据
}

type WeighRes struct {
	*gf.R
}

// 获取数据库表
type GetTablesReq struct {
	g.Meta `path:"/basetool/table/getTables" tags:"Weigh" method:"get" summary:"获取数据库表"`
}

type GetTablesRes struct {
	*gf.R
}
