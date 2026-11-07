// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// Member is the golang structure of table gf_member for DAO operations like Where/Data.
type Member struct {
	g.Meta         `orm:"table:gf_member, do:true"`
	Id             any         // ID
	BusinessId     any         // 业务主账号id
	Username       any         // 用户名
	Name           any         // 姓名
	Nickname       any         // 昵称
	Remark         any         // 备注
	Password       any         // 密码
	Salt           any         // 密码盐
	Email          any         // 电子邮箱
	Mobile         any         // 手机号
	Avatar         any         // 头像
	Level          any         // 等级
	Sex            any         // 性别:1=男性,2=女性,0=未知
	Birthday       *gtime.Time // 出生日期
	Money          any         // 余额
	Score          any         // 积分
	Successions    any         // 连续登录天数
	Maxsuccessions any         // 最大连续登录天数
	Prevtime       any         // 上次登录时间
	Logintime      any         // 登录时间
	Loginip        any         // 登录IP
	Loginfailure   any         // 失败次数
	Status         any         // 状态
	Createtime     *gtime.Time // 创建时间
	Updatetime     *gtime.Time // 更新时间
	Deletetime     *gtime.Time // 删除时间
}
