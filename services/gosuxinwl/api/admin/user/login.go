package user

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

type LoginReq struct {
	g.Meta `path:"/user/login" noLogin:"1" noAuth:"1" tags:"Login" method:"post" summary:"登录"`
	// Type     string `p:"type" d:"username" dc:"登录方式"`
	Username string `p:"username" d:""` // 用户名
	Password string `p:"password" d:""` // 密码
	Codeid   string `p:"codeid" `       // 数字验证码ID
	Captcha  string `p:"captcha" `      // 图片验证码
	Mobile   string `p:"mobile" d:"" dc:"手机号"`
	Email    string `p:"email" d:"" dc:"邮箱"`
}

type LoginRes struct {
	*gf.R
}

type LoginOutReq struct {
	g.Meta `path:"/user/logout" noAuth:"1" tags:"Logout" method:"post" summary:"退出登录"`
}

type LoginOutRes struct {
	*gf.R
}
