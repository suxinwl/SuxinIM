package dashboard

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

type GetQuickReq struct {
	g.Meta `path:"/dashboard/workplace/getQuick" noAuth:"1" tags:"getQuick" method:"get" summary:"获取快捷操作数据"`
}

type GetQuickRes struct {
	*gf.R
}

type SaveQuickReq struct {
	g.Meta     `path:"/dashboard/workplace/saveQuick" tags:"saveQuick" method:"post" summary:"保存快捷操作数据"`
	Id         int64  `p:"id" d:"0" dc:"数据id"`
	BusinessId int64  `p:"businessId" d:"1" dc:"预留多租户数据分离"`
	Icon       string `p:"icon" dc:"是否已经安装"`
	Type       int    `p:"type" d:"0" dc:"类型"`
	Name       string `p:"name" v:"required#名称不能为空" dc:"名称"`
	PathUrl    string `p:"path_url" dc:"路由地址"`
	Weigh      int64  `p:"weigh" d:"0" dc:"排序"`
}

type SaveQuickRes struct {
	*gf.R
}
