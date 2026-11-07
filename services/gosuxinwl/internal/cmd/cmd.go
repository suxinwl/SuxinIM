package cmd

import (
	"context"
	"fmt"
	"github.com/suxinwl/GoSuxin/internal/adminweb"
	"github.com/suxinwl/GoSuxin/internal/builtinai"
	"github.com/suxinwl/GoSuxin/internal/certvalidation"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/internal/extend/clogic"
	"github.com/suxinwl/GoSuxin/internal/extend/middleware"
	"github.com/suxinwl/GoSuxin/internal/imbridge"
	"github.com/suxinwl/GoSuxin/internal/plugins"
	"github.com/suxinwl/GoSuxin/internal/router"
	"github.com/suxinwl/GoSuxin/internal/runtimeplugin"
	"github.com/suxinwl/GoSuxin/internal/service"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/os/gcmd"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
)

var (
	Main = gcmd.Command{
		Name:  "main",
		Usage: "main",
		Brief: "start http server",
		Func: func(ctx context.Context, parser *gcmd.Parser) (err error) {
			s := g.Server()
			// Domain validation must bypass login and JSON response middleware.
			validation := certvalidation.Handler(".")
			s.BindHandler(certvalidation.Path, func(r *ghttp.Request) {
				validation.ServeHTTP(r.Response.BufferWriter, r.Request)
			})
			// 配置静态资源目录
			s.AddStaticPath("/resource/uploads", "./resource/uploads") //访问本地附件
			admin := adminweb.Handler("./resource/suxinweb")
			serveAdmin := func(r *ghttp.Request) {
				// The framework trims trailing slashes; the file server needs the original URL.
				request := r.Request.Clone(r.Context())
				if original, parseErr := url.ParseRequestURI(r.RequestURI); parseErr == nil {
					request.URL = original
				}
				admin.ServeHTTP(r.Response.BufferWriter, request)
			}
			s.BindHandler("/suxinweb", serveAdmin)
			s.BindHandler("/suxinweb/*path", serveAdmin)                         //访问部署管理后台前端vue打包代码
			s.AddStaticPath("/resource/static/brand", "./resource/static/brand") // GoSuxin public brand assets
			//安装页面
			runEnv, _ := g.Cfg("app").Get(ctx, "app.runEnv")
			if runEnv.String() == "debug" {
				s.AddStaticPath("/install", "./devsource/developer/install")
				path, _ := os.Getwd() //获取当前路径
				filePath := filepath.Join(path, "/devsource/developer/install/install.lock")
				if _, err := os.Stat(filePath); err != nil {
					go func() {
						time.Sleep(2 * time.Second)
						fmt.Printf("%c[1;40;32m%s%c[0m\n", 0x1B, "如果还没有安装-请在浏览器访问​进行​安装：​​http://127.0.0.1:"+gconv.String(s.GetListenedPort())+"/install", 0x1B)
					}()
				}
			}
			stopRuntime, runtimeErr := runtimeplugin.Default.Start(ctx, s, middleware.Token, middleware.Auth, service.Adminsystem().OperationLog)
			if runtimeErr != nil {
				return runtimeErr
			}
			defer stopRuntime()
			stopPlugins, startErr := plugins.Start(ctx, s)
			if startErr != nil {
				return startErr
			}
			defer stopPlugins()
			stopIM, imErr := imbridge.Start(ctx, s)
			if imErr != nil {
				return imErr
			}
			defer stopIM()
			if prepareErr := builtinai.Prepare(ctx); prepareErr != nil {
				return prepareErr
			}
			// Existing installations need the role editor's missing action rules;
			// fresh installations have no tables until the installer completes.
			if _, statErr := os.Stat("devsource/developer/install/install.lock"); statErr == nil {
				if seedErr := clogic.EnsureRoleManagementRules(ctx, dao.AuthRule.Ctx(ctx)); seedErr != nil {
					return seedErr
				}
				if seedErr := clogic.EnsureAccountSecurityRules(ctx, dao.AuthRule.Ctx(ctx)); seedErr != nil {
					return seedErr
				}
			}
			router.BindController(ctx, s)
			builtinai.BindPublic(s)
			s.Run()
			return nil
		},
	}
)
