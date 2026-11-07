// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// DictionaryData is the golang structure for table dictionary_data.
type DictionaryData struct {
	Id         uint        `json:"id"         orm:"id"         description:""`
	DataFrom   string      `json:"dataFrom"   orm:"data_from"  description:"数据来源:common=公共,business=商业端"`
	GroupId    int         `json:"groupId"    orm:"group_id"   description:"数据分组id"`
	Keyname    string      `json:"keyname"    orm:"keyname"    description:"字典名称"`
	Keyvalue   string      `json:"keyvalue"   orm:"keyvalue"   description:"字典项值"`
	Tagcolor   string      `json:"tagcolor"   orm:"tagcolor"   description:"标签颜色"`
	Des        string      `json:"des"        orm:"des"        description:"字典描述"`
	Status     int         `json:"status"     orm:"status"     description:"状态"`
	Weigh      int         `json:"weigh"      orm:"weigh"      description:"排序"`
	Createtime *gtime.Time `json:"createtime" orm:"createtime" description:"创建时间"`
	Updatetime *gtime.Time `json:"updatetime" orm:"updatetime" description:"更新时间"`
}
