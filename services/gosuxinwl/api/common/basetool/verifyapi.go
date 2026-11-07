package basetool

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

type GetCaptchaReq struct {
	g.Meta `path:"/basetool/getCaptcha" noLogin:"1" noAuth:"1" tags:"GetCaptcha" method:"get" summary:"获取验证码、图片验证码、手机验证码、邮箱验证码"`
	Type   string `p:"type" v:"required#验证类型不能为空"` // 验证类型
	Mobile string `p:"mobile"`                     // 手机验证码
	Email  string `p:"email"`                      // 写邮箱
	Keyval string `p:"keyval"`                     // 扫码类型
}

type GetCaptchaRes struct {
	*gf.R
}

type CheckHaseRuleReq struct {
	g.Meta       `path:"/basetool/checkHaseRule" noLogin:"1" noAuth:"1" tags:"CheckHaseRule" method:"post" summary:"检查菜单路由是否存在"`
	Id           string `p:"id" dc:"编辑的id"`          // 创建时不传
	Codelocation string `p:"codelocation" dc:"后台类型"` //预留参数
	Routename    string `p:"routename" v:"required#路由名不能为空" dc:"路由名"`
}

type CheckHaseRuleRes struct {
	*gf.R
}
