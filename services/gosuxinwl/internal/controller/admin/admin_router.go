// =================================================================================
// admin模块路由入口
// =================================================================================

package admin

import (
	"context"
	"github.com/suxinwl/GoSuxin/internal/builtinai"
	"github.com/suxinwl/GoSuxin/internal/extend/middleware"
	"github.com/suxinwl/GoSuxin/internal/plugins"
	"github.com/suxinwl/GoSuxin/internal/service"

	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
)

var R = new(Router)

type Router struct{}

func (router *Router) BindController(ctx context.Context, group *ghttp.RouterGroup) {
	group.Group("/admin", func(group *ghttp.RouterGroup) {
		//登录验证拦截
		group.Middleware(middleware.Token)
		//验证后台管理接口的RBAC权限验证
		group.Middleware(middleware.Auth)
		//后台操作日志记录
		group.Middleware(service.Adminsystem().OperationLog)
		//注册路由-新增功能需要把admin_new.go新增的NewXX添加进来
		group.Bind(
			NewUser(),
			NewSystem(),
			NewSecurity(),
			NewDashboard(),
			NewInstall(),
			NewCommon(),
			NewDatacenter(),
			NewDeveloper(),
		) //append
		plugins.BindAdmin(group)
		builtinai.BindAdmin(ctx, group)
	})
}
