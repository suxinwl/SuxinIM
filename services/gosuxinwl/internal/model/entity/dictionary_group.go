// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// DictionaryGroup is the golang structure for table dictionary_group.
type DictionaryGroup struct {
	Id         uint        `json:"id"         orm:"id"          description:""`
	BusinessId int         `json:"businessId" orm:"business_id" description:"业务主账号id"`
	Title      string      `json:"title"      orm:"title"       description:"字典分组名称"`
	Remark     string      `json:"remark"     orm:"remark"      description:"备注"`
	DataFrom   string      `json:"dataFrom"   orm:"data_from"   description:"数据来源:common=公共,business=商业端"`
	DbWay      string      `json:"dbWay"      orm:"db_way"      description:"数据存储位置:sys=公共表,alone=单独建表"`
	Tablename  string      `json:"tablename"  orm:"tablename"   description:"数据表名称"`
	Status     int         `json:"status"     orm:"status"      description:"状态"`
	Weigh      int         `json:"weigh"      orm:"weigh"       description:"排序"`
	Createtime *gtime.Time `json:"createtime" orm:"createtime"  description:"创建时间"`
}
