package user

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

type GetUserinfoReq struct {
	g.Meta `path:"/user/getUserinfo" noAuth:"1" tags:"getUserinfo" method:"get" summary:"获取用户信息"`
}

type GetUserinfoRes struct {
	*gf.R
}

type GetMenuReq struct {
	g.Meta `path:"/user/getMenu" noAuth:"1" tags:"getMenu" method:"get" summary:"获取用户管理后台菜单"`
}

type GetMenuRes struct {
	*gf.R
}

// 修改用户信息（密码、邮箱等）
type SaveInfoReq struct {
	g.Meta      `path:"/user/saveInfo" noAuth:"1" tags:"getMenu" method:"post" summary:"修改本人的用户信息"`
	Type        string `p:"type" v:"required#修改内容类型不能为空" dc:"修改内容类型"`
	Avatar      string `p:"avatar" d:"" dc:"头像"`
	Nickname    string `p:"nickname" d:"" dc:"昵称"`
	Captcha     string `p:"captcha" dc:"验证码"`
	Mobile      string `p:"mobile" dc:"手机号"`
	Email       string `p:"email" dc:"邮箱"`
	Newpassword string `p:"newpassword" dc:"新密码"`
	Oldpassword string `p:"oldpassword" dc:"旧密码"`
}

type SaveInfoRes struct {
	*gf.R
}
