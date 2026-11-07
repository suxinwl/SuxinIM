// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// LoginLog is the golang structure for table login_log.
type LoginLog struct {
	Id         uint        `json:"id"         orm:"id"         description:""`
	Uid        int         `json:"uid"        orm:"uid"        description:"用户id"`
	Username   string      `json:"username"   orm:"username"   description:"登录账号"`
	Ip         string      `json:"ip"         orm:"ip"         description:"登录IP"`
	Address    string      `json:"address"    orm:"address"    description:"地点"`
	Des        string      `json:"des"        orm:"des"        description:"登录行为"`
	Os         string      `json:"os"         orm:"os"         description:"操作系统"`
	Browser    string      `json:"browser"    orm:"browser"    description:"浏览器类型"`
	ErrorMsg   string      `json:"errorMsg"   orm:"error_msg"  description:"登录失败原因"`
	Status     int         `json:"status"     orm:"status"     description:"状态:0=成功,1=失败"`
	Createtime *gtime.Time `json:"createtime" orm:"createtime" description:"创建时间"`
}
