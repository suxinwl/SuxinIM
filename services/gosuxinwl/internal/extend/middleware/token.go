package middleware

import (
	"github.com/suxinwl/GoSuxin/utility/auth"
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
)

// 用于验证token有效性工具
func Token(r *ghttp.Request) {
	if TrustedPluginIdentity(r) {
		r.Middleware.Next()
		return
	}
	if r.GetServeHandler().Handler.GetMetaTag("noLogin") == "1" { //忽略不需要登录验证接口
		r.Middleware.Next()
	} else {
		user, err := auth.ParseToken(r)
		if err != nil {
			g.Log().Warningf(r.Context(), "admin authentication rejected path=%s reason=%v %s", r.URL.Path, err, auth.RequestTimingDiagnostic(r))
			r.Response.WriteJsonExit(gf.Failed().SetCode(auth.JwtTokenInvalid).SetMsg(err.Error()))
			r.ExitAll()
		} else {
			userMap, ok := user.Data.(map[string]interface{})
			if !ok {
				r.SetCtxVar("user", userMap)
			} else {
				r.SetCtxVar("user", userMap)
				if uid, uok := userMap["uid"]; uok {
					r.SetCtxVar("uid", uid)
				}
			}
			r.Middleware.Next()
		}
	}
}
