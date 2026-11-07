// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// AuthRole is the golang structure for table auth_role.
type AuthRole struct {
	Id         uint        `json:"id"         orm:"id"          description:""`
	BusinessId int         `json:"businessId" orm:"business_id" description:"业务主账号id"`
	AccountId  int         `json:"accountId"  orm:"account_id"  description:"添加用户id"`
	Pid        int         `json:"pid"        orm:"pid"         description:"父级"`
	Name       string      `json:"name"       orm:"name"        description:"名称"`
	Rules      string      `json:"rules"      orm:"rules"       description:"规则ID 所拥有的权限包括父级"`
	Menu       string      `json:"menu"       orm:"menu"        description:"选择的id，用于编辑赋值"`
	Btns       string      `json:"btns"       orm:"btns"        description:"按钮id，用于编辑赋值"`
	Status     int         `json:"status"     orm:"status"      description:"状态1=禁用"`
	DataAccess int         `json:"dataAccess" orm:"data_access" description:"数据权限0=自己1=自己及子权限，2=全部"`
	Remark     string      `json:"remark"     orm:"remark"      description:"描述"`
	Weigh      int         `json:"weigh"      orm:"weigh"       description:"排序"`
	Createtime *gtime.Time `json:"createtime" orm:"createtime"  description:"添加时间"`
}
