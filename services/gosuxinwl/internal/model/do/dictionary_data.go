// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// DictionaryData is the golang structure of table gf_dictionary_data for DAO operations like Where/Data.
type DictionaryData struct {
	g.Meta     `orm:"table:gf_dictionary_data, do:true"`
	Id         any         //
	DataFrom   any         // 数据来源:common=公共,business=商业端
	GroupId    any         // 数据分组id
	Keyname    any         // 字典名称
	Keyvalue   any         // 字典项值
	Tagcolor   any         // 标签颜色
	Des        any         // 字典描述
	Status     any         // 状态
	Weigh      any         // 排序
	Createtime *gtime.Time // 创建时间
	Updatetime *gtime.Time // 更新时间
}
