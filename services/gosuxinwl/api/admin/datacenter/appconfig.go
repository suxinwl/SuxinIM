package datacenter

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

type GetConfigReq struct {
	g.Meta `path:"datacenter/appconfig/getConfig" tags:"getConfig" method:"get" summary:"获取系统配置"`
}
type GetConfigRes struct {
	*gf.R
}

// 保存应用配置数据
type SaveConfigReq struct {
	g.Meta       `path:"datacenter/appconfig/saveConfig" tags:"saveConfig" method:"post" summary:"保存应用配置数据"`
	Vueobjroot   string `p:"vueobjroot"  d:"" dc:"附件访问域名、前缀路径"`
	MultiLogin   bool   `p:"MultiLogin"  dc:"是否允许多端登录"`
	LoginCaptcha bool   `p:"loginCaptcha" dc:"登录是否启用人机验证"`
	ValidityApi  bool   `p:"validityApi" dc:"是否启接口请求对称加密"`
}
type SaveConfigRes struct {
	*gf.R
}
