package datacenter

import (
	"github.com/suxinwl/GoSuxin/api/admin/baseapi"
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

type DictionaryListReq struct {
	g.Meta `path:"datacenter/dictionary/getList" tags:"getList" method:"get" summary:"获取字典列表"`
	baseapi.PageReq
	Title      string `p:"title" d:"" dc:"搜索名称"`
	GroupId    int64  `p:"group_id" d:"0" dc:"分组id"`
	Status     any    `p:"status" d:"" dc:"状态"`
	Tablename  string `p:"tablename" v:"required#数据表名不能为空" dc:"数据表名"`
	Createtime string `p:"createtime" d:"" dc:"创建时间"`
}
type DictionaryListRes struct {
	*gf.R
}

// 保存数据
type DictionarySaveReq struct {
	g.Meta    `path:"datacenter/dictionary/save" tags:"Save" method:"post" summary:"保存、编辑字典数据"`
	Id        int64  `p:"id" d:"0" dc:"数据id，新增时为0"`
	Keyname   string `p:"keyname" v:"required#字典名称不能为空" dc:"字典名称"`
	Keyvalue  string `p:"keyvalue" v:"required#字典项值不能为空" dc:"字典项值"`
	Tablename string `p:"tablename" v:"required#数据表名不能为空" dc:"数据表名"`
	GroupId   int64  `p:"group_id" v:"required#字典分组不能为空" dc:"典分组"`
	Tagcolor  string `p:"tagcolor" dc:"标签颜色"`
	Des       string `p:"des" dc:"字典描述"`
	DataFrom  string `p:"data_from" d:"business" dc:"数据来源"`
	Status    int    `p:"status" d:"0" dc:"状态"`
	Weigh     int    `p:"weigh" d:"0" dc:"排序"`
}
type DictionarySaveRes struct {
	*gf.R
}

type DictionaryStatusReq struct {
	g.Meta    `path:"datacenter/dictionary/upStatus" tags:"upStatus" method:"post" summary:"更新状态"`
	Id        int64  `p:"id"  v:"required#id不能为空" dc:"更新id"`
	Tablename string `p:"tablename" v:"required#数据表名不能为空" dc:"数据表名"`
	Status    uint   `p:"status" v:"required#status不能为空" dc:"状态"`
}
type DictionaryStatusRes struct {
	*gf.R
}

type DictionaryDelReq struct {
	g.Meta    `path:"datacenter/dictionary/del" tags:"Del" method:"delete" summary:"删除"`
	Ids       g.Array `p:"ids"`
	Tablename string  `p:"tablename" v:"required#数据表名不能为空" dc:"数据表名"`
}
type DictionaryDelRes struct {
	*gf.R
}

type GetTableDataFormReq struct {
	g.Meta    `path:"datacenter/dictionary/getTableDataForm" tags:"GetTableDataForm" method:"get" summary:"使用数据表数据(表单生成使用)"`
	Tablename string `p:"tablename" v:"required#数据表名不能为空" dc:"数据表名"`
	Custom    any    `p:"custom"   dc:"自定搜索条件"`
	Showfield string `p:"showfield"   dc:"获取字段"`
}
type GetTableDataFormRes struct {
	*gf.R
}
