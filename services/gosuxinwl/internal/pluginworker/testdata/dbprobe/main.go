// A test worker used by scripts/test-database-install.cjs.
package main

import (
	"context"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/internal/pluginworker"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

func main() {
	pluginworker.Run("db-probe", func(ctx context.Context, group *ghttp.RouterGroup) {
		group.POST("/api/admin/probe", func(r *ghttp.Request) {
			if r.Context().Value("uid") == nil {
				r.Response.WriteStatus(401)
				return
			}
			id, err := g.Model("auth_dept").Ctx(r.Context()).Data(g.Map{"name": "插件测试车间", "pid": 1, "remark": ""}).InsertAndGetId()
			if err != nil {
				r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
				return
			}
			r.Response.WriteJson(gf.Success().SetData(id))
		})
	})
}
