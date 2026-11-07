package install

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

type GetConfigReq struct {
	g.Meta `path:"/install/getConfig" noLogin:"1" noAuth:"1" noValApi:"1" tags:"GetConfig" method:"get" summary:"获取安装配置数据"`
}

type GetConfigRes struct {
	*gf.R
}

type SaveReq struct {
	g.Meta  `path:"/install/save" noLogin:"1" noAuth:"1" noValApi:"1" tags:"save" method:"post" summary:"执行安装操作"`
	FormDB  gf.MapStrStr `p:"formDB"  dc:"数据库配置"`
	FormApp gf.MapStrStr `p:"formApp"  dc:"应用配置"`
}

type SaveRes struct {
	*gf.R
}
