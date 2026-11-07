package datacenter

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

type TabledataListReq struct {
	g.Meta `path:"datacenter/tabledata/getList" tags:"getList" method:"get" summary:"获取字典分组列表"`
}
type TabledataListRes struct {
	*gf.R
}

// 保存数据
type TabledataSaveReq struct {
	g.Meta    `path:"datacenter/tabledata/save" tags:"Save" method:"post" summary:"保存、编辑字典"`
	Id        int64  `p:"id" d:"0" dc:"数据id，新增时为0"`
	Title     string `p:"title" v:"required#字典名称不能为空" dc:"字典名称"`
	Data_from string `p:"data_from" d:"business" dc:"数据来自"`
	BbWay     string `p:"db_way" d:"sys" dc:"数据存储表"`
	Tablename string `p:"tablename" dc:"数据表名"`
	Remark    string `p:"remark" dc:"备注"`
	Status    int    `p:"status" d:"0" dc:"状态"`
	Weigh     int    `p:"weigh" d:"0" dc:"排序"`
}
type TabledataSaveRes struct {
	*gf.R
}

type TabledataDelReq struct {
	g.Meta `path:"datacenter/tabledata/del" tags:"Del" method:"delete" summary:"删除"`
	Ids    g.Array `p:"ids"`
}
type TabledataDelRes struct {
	*gf.R
}
