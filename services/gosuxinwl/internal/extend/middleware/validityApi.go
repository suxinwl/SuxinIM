// =================
// api接口对称加密验证
// =================
package middleware

import (
	"encoding/base64"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"strconv"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/framework/crypto/gmd5"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
)

func ValidityApi(r *ghttp.Request) {
	validityApi, _ := g.Cfg("app").Get(r.Context(), "app.validityApi")
	if pathArr := strings.Split(r.Request.URL.Path, "/"); (len(pathArr) > 0 && strings.Contains(validityApi.String(), pathArr[1])) || r.GetServeHandler().Handler.GetMetaTag("noValApi") == "1" { //忽略单个接口加密验证
		r.Middleware.Next()
	} else {
		apiverify := r.Header.Get("apiverify")
		apisecret, _ := g.Cfg("app").Get(r.Context(), "app.apisecret")
		decodedBytes, err := base64.StdEncoding.DecodeString(apiverify)
		if err != nil || apiverify == "" { //先判断数据是否传值
			r.Response.WriteJsonExit(gf.Failed().SetMsg(gf.I18n(r, "common_vlidity_api")))
			r.ExitAll()
			return
		}
		decodedBytes_arr := strings.Split(string(decodedBytes), "#")
		encrypt := decodedBytes_arr[0]
		verifytime := decodedBytes_arr[1]
		mdsecret, _ := gmd5.Encrypt(apisecret.String() + verifytime)
		verifytimeint, _ := strconv.ParseInt(verifytime, 10, 64)
		if mdsecret == encrypt && (time.Now().Unix()-verifytimeint < 60*5) { //验证码5分钟内有效，所以两端时间戳相差不能大于5分钟
			r.Middleware.Next()
		} else {
			r.Response.WriteJsonExit(gf.Failed().SetMsg(gf.I18n(r, "common_vlidity_api")))
			r.ExitAll()
		}
	}
}
