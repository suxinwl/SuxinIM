package router

import (
	"context"
	"github.com/suxinwl/GoSuxin/internal/controller/admin"
	"github.com/suxinwl/GoSuxin/internal/controller/common"
	"github.com/suxinwl/GoSuxin/internal/extend/middleware"
	"github.com/suxinwl/GoSuxin/internal/imbridge"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"strings"

	"github.com/suxinwl/GoSuxin/internal/controller/wxapp"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/os/gcfg"
	"golang.org/x/time/rate"
) //append

func BindController(ctx context.Context, serve *ghttp.Server) {
	//配置传输文件最大值，如上传文件最大上限
	MaxBodySize, _ := g.Cfg("upload").Get(ctx, "MaxBodySize")
	serve.SetClientMaxBodySize(MaxBodySize.Int64() * 1024 * 1024) // 单位为MB
	//服务模块接口路由
	serve.Group("/", func(group *ghttp.RouterGroup) {
		//1.跨域处理，安全起见正式环境请注释该行
		group.Middleware(MiddlewareCORS)
		//2.group.Middleware(ghttp.MiddlewareHandlerResponse)
		group.Middleware(middleware.HandlerResponse) //自定返回数据格式
		//3.请求频率限制：使用GF框架的限流中间件防止恶意请求
		group.Middleware(Limiter)
		//5.接口对称解密验证-防止接口盗用
		group.Middleware(middleware.ValidityApi)
		/*******************开始注册路由************************/
		// 绑定后台路由
		admin.R.BindController(ctx, group)
		// 绑定公共路由
		common.R.BindController(ctx, group)
		// 微信小程序
		wxapp.R.BindController(ctx, group)
	}) //append
	//静态文件访问控制
	serve.Group("/resource/uploads", func(group *ghttp.RouterGroup) {
		group.Hook("/*", ghttp.HookBeforeServe, func(r *ghttp.Request) {
			r.Response.Header().Set("Cache-Control", "private, no-store")
			// 1. 检查文件访问权限
			if !CheckPermission(r) {
				r.Response.WriteStatus(403, "您没有访问附件权限")
				r.ExitAll()
			}
			// 2. 记录访问日志
			// 3. 继续处理请求
			r.Middleware.Next()
		})
	})
}

// 允许跨域
func MiddlewareCORS(r *ghttp.Request) {
	corsOptions := r.Response.DefaultCORSOptions()
	allowurl, _ := gcfg.Instance("app").Get(r.Context(), "app.allowurl") // 允许访问域名授权
	corsOptions.AllowDomain = strings.Split(allowurl.String(), `,`)
	r.Response.CORS(corsOptions)
	r.Middleware.Next()
}

// 它允许每秒100个请求，突发大小为1。
// 注意：在生产环境中，考虑使用分布式速率限制器。
// Limiter是一个中间件，为所有HTTP请求实现速率限制。
// 当超过速率限制时，它返回HTTP 429 （Too Many Requests）
var limiter = rate.NewLimiter(rate.Limit(100), 50)

func Limiter(r *ghttp.Request) {
	if limiter.Allow() {
		r.Middleware.Next()
	} else {
		r.Response.WriteJsonExit(gf.Failed().SetMsg("你访问频率过高"))
		r.ExitAll()
	}
}

// 检查用户是否有权限访问文件[你可以更新业务需求扩展验证功能]
func CheckPermission(r *ghttp.Request) bool {
	return imbridge.HRMAssetRequestAllowed(r)
}
