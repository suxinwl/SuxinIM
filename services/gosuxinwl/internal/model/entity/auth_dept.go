// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// AuthDept is the golang structure for table auth_dept.
type AuthDept struct {
	Id             uint        `json:"id"         orm:"id"          description:""`
	BusinessId     int         `json:"businessId" orm:"business_id" description:"业务主账号id"`
	AccountId      int         `json:"accountId"  orm:"account_id"  description:"添加账号"`
	Name           string      `json:"name"       orm:"name"        description:"部门名称"`
	Pid            int         `json:"pid"        orm:"pid"         description:"上级部门"`
	Weigh          int         `json:"weigh"      orm:"weigh"       description:"排序"`
	Status         int         `json:"status"     orm:"status"      description:"状态:0=正常;1=禁用"`
	IMGroupEnabled int         `json:"imGroupEnabled" orm:"im_group_enabled" description:"部门群自动管理开关"`
	LeaderUID      int64       `json:"leaderUid" orm:"leader_uid" description:"部门负责人宿主UID"`
	Remark         string      `json:"remark"     orm:"remark"      description:"备注"`
	Createtime     *gtime.Time `json:"createtime" orm:"createtime"  description:"创建时间"`
	Updatetime     *gtime.Time `json:"updatetime" orm:"updatetime"  description:"更新时间"`
	Deletetime     *gtime.Time `json:"deletetime" orm:"deletetime"  description:"删除时间"`
}
