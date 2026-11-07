// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// Member is the golang structure for table member.
type Member struct {
	Id             uint        `json:"id"             orm:"id"             description:"ID"`
	BusinessId     int         `json:"businessId"     orm:"business_id"    description:"业务主账号id"`
	Username       string      `json:"username"       orm:"username"       description:"用户名"`
	Name           string      `json:"name"           orm:"name"           description:"姓名"`
	Nickname       string      `json:"nickname"       orm:"nickname"       description:"昵称"`
	Remark         string      `json:"remark"         orm:"remark"         description:"备注"`
	Password       string      `json:"password"       orm:"password"       description:"密码"`
	Salt           string      `json:"salt"           orm:"salt"           description:"密码盐"`
	Email          string      `json:"email"          orm:"email"          description:"电子邮箱"`
	Mobile         string      `json:"mobile"         orm:"mobile"         description:"手机号"`
	Avatar         string      `json:"avatar"         orm:"avatar"         description:"头像"`
	Level          uint        `json:"level"          orm:"level"          description:"等级"`
	Sex            uint        `json:"sex"            orm:"sex"            description:"性别:1=男性,2=女性,0=未知"`
	Birthday       *gtime.Time `json:"birthday"       orm:"birthday"       description:"出生日期"`
	Money          float64     `json:"money"          orm:"money"          description:"余额"`
	Score          int         `json:"score"          orm:"score"          description:"积分"`
	Successions    uint        `json:"successions"    orm:"successions"    description:"连续登录天数"`
	Maxsuccessions uint        `json:"maxsuccessions" orm:"maxsuccessions" description:"最大连续登录天数"`
	Prevtime       int64       `json:"prevtime"       orm:"prevtime"       description:"上次登录时间"`
	Logintime      int64       `json:"logintime"      orm:"logintime"      description:"登录时间"`
	Loginip        string      `json:"loginip"        orm:"loginip"        description:"登录IP"`
	Loginfailure   uint        `json:"loginfailure"   orm:"loginfailure"   description:"失败次数"`
	Status         int         `json:"status"         orm:"status"         description:"状态"`
	Createtime     *gtime.Time `json:"createtime"     orm:"createtime"     description:"创建时间"`
	Updatetime     *gtime.Time `json:"updatetime"     orm:"updatetime"     description:"更新时间"`
	Deletetime     *gtime.Time `json:"deletetime"     orm:"deletetime"     description:"删除时间"`
}
