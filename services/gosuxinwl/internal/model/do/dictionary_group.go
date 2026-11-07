// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// DictionaryGroup is the golang structure of table gf_dictionary_group for DAO operations like Where/Data.
type DictionaryGroup struct {
	g.Meta     `orm:"table:gf_dictionary_group, do:true"`
	Id         any         //
	BusinessId any         // 业务主账号id
	Title      any         // 字典分组名称
	Remark     any         // 备注
	DataFrom   any         // 数据来源:common=公共,business=商业端
	DbWay      any         // 数据存储位置:sys=公共表,alone=单独建表
	Tablename  any         // 数据表名称
	Status     any         // 状态
	Weigh      any         // 排序
	Createtime *gtime.Time // 创建时间
}
