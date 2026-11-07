// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// Admin is the golang structure for table admin.
type Admin struct {
	AuthenticatedEpoch int64       `json:"-" orm:"-"`
	Id                 uint        `json:"id"            orm:"id"             description:"ID"`
	AccountId          int         `json:"accountId"     orm:"account_id"     description:"账号id/记录那个账号添加"`
	DeptId             int         `json:"deptId"        orm:"dept_id"        description:"部门id"`
	Username           string      `json:"username"      orm:"username"       description:"用户名"`
	Password           string      `json:"password"      orm:"password"       description:"密码"`
	Salt               string      `json:"salt"          orm:"salt"           description:"密码盐"`
	Name               string      `json:"name"          orm:"name"           description:"姓名"`
	Nickname           string      `json:"nickname"      orm:"nickname"       description:"昵称"`
	Avatar             string      `json:"avatar"        orm:"avatar"         description:"头像"`
	Email              string      `json:"email"         orm:"email"          description:"电子邮箱"`
	Mobile             string      `json:"mobile"        orm:"mobile"         description:"手机号码"`
	Tel                string      `json:"tel"           orm:"tel"            description:"备用电话用户自己填写"`
	Status             int         `json:"status"        orm:"status"         description:"状态:0=正常,1=禁用"`
	Remark             string      `json:"remark"        orm:"remark"         description:"备注"`
	Loginip            string      `json:"loginip"       orm:"loginip"        description:"登录IP"`
	Logintime          *gtime.Time `json:"logintime"     orm:"logintime"      description:"最后登录时间"`
	LoginAttempts      int         `json:"loginAttempts" orm:"login_attempts" description:"登录尝试次数"`
	LockTime           *gtime.Time `json:"lockTime"      orm:"lock_time"      description:"账号锁定时间"`
	Createtime         *gtime.Time `json:"createtime"    orm:"createtime"     description:"创建时间"`
	Updatetime         *gtime.Time `json:"updatetime"    orm:"updatetime"     description:"更新时间"`
	Deletetime         *gtime.Time `json:"deletetime"    orm:"deletetime"     description:"删除时间"`
	PwdResetTime       *gtime.Time `json:"pwdResetTime"  orm:"pwd_reset_time" description:"修改密码时间"`
}
