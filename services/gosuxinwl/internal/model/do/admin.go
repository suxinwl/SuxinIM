// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// Admin is the golang structure of table gf_admin for DAO operations like Where/Data.
type Admin struct {
	g.Meta        `orm:"table:gf_admin, do:true"`
	Id            any         // ID
	AccountId     any         // 账号id/记录那个账号添加
	DeptId        any         // 部门id
	Username      any         // 用户名
	Password      any         // 密码
	Salt          any         // 密码盐
	Name          any         // 姓名
	Nickname      any         // 昵称
	Avatar        any         // 头像
	Email         any         // 电子邮箱
	Mobile        any         // 手机号码
	Tel           any         // 备用电话用户自己填写
	Status        any         // 状态:0=正常,1=禁用
	Remark        any         // 备注
	Loginip       any         // 登录IP
	Logintime     *gtime.Time // 最后登录时间
	LoginAttempts any         // 登录尝试次数
	LockTime      *gtime.Time // 账号锁定时间
	Createtime    *gtime.Time // 创建时间
	Updatetime    *gtime.Time // 更新时间
	Deletetime    *gtime.Time // 删除时间
	PwdResetTime  *gtime.Time // 修改密码时间
}
