// ========================
// 用于验证后台管理接口的RBAC权限验证
// ========================
package middleware

import (
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"net/http"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
)

func Auth(r *ghttp.Request) {
	if r.GetServeHandler().Handler.GetMetaTag("noAuth") == "1" { //忽略不需要权限验证的接口
		r.Middleware.Next()
	} else {
		haseauth := checkAuth(r)
		if haseauth {
			r.Middleware.Next()
		} else {
			if r.Method == http.MethodGet && r.URL.Path == "/plugins/biz-suite/api/admin/im/session" {
				// The IM keepalive must stop when the shared runtime gate is revoked.
				r.Response.WriteHeader(http.StatusForbidden)
				r.Response.WriteJsonExit(gf.Failed().SetCode(http.StatusForbidden).SetMsg(gf.I18n(r, "sys_auth_permission")))
				return
			}
			r.Response.WriteJsonExit(gf.Failed().SetMsg(gf.I18n(r, "sys_auth_permission")))
			r.ExitAll()
		}
	}
}

// 检查接口权限
func checkAuth(r *ghttp.Request) bool {
	if plugin, ok := runtimePluginName(r.URL.Path); ok {
		return checkRuntimePluginAuth(r, plugin)
	}
	uid := r.Context().Value("uid")
	ctx := r.Context()
	role_id, acerr := dao.AuthRoleAccess.Ctx(ctx).Where("uid", uid).Array("role_id")
	if acerr != nil || role_id == nil {
		return false
	}
	//1.判断是否有超级角色
	super_role, rerr := dao.AuthRole.Ctx(ctx).WhereIn("id", role_id).Where("status", 0).Where("rules", "*").Count()
	if rerr != nil {
		return false
	}
	if super_role != 0 { //超级角色
		superRoleAuth, _ := g.Cfg("app").Get(ctx, "app.superRoleAuth")
		if superRoleAuth.Bool() {
			hasepath, ruerr := dao.AuthRule.Ctx(ctx).Where("status", 0).Where("type", 2).WhereIn("path", departmentPermissionPaths(r.URL.Path)).Count()
			if ruerr == nil && hasepath != 0 {
				return true
			}
		} else {
			return true
		}
	} else { //普通角色
		btns_ids, rerr := dao.AuthRole.Ctx(ctx).WhereIn("id", role_id).Where("status", 0).Array("btns") //数据权限
		if rerr != nil {
			return false
		}
		hasepath, ruerr := dao.AuthRule.Ctx(ctx).Where("status", 0).Where("type", 2).WhereIn("id", gf.ArrayMerge(btns_ids)).WhereIn("path", departmentPermissionPaths(r.URL.Path)).Count()
		if ruerr == nil && hasepath != 0 {
			return true
		}
	}
	return false
}

// These two department operations inherit existing department RBAC grants.
// No IM management permission or anonymous/noAuth bypass is introduced.
func departmentPermissionPaths(path string) []string {
	switch path {
	case "/admin/system/account/security":
		return []string{path, "/admin/system/account/getList", "/admin/system/account/resetPassword", "/admin/system/account/unlock", "/admin/system/account/accessPolicy"}
	case "/admin/system/dept/getLeaderOptions":
		return []string{path, "/admin/system/dept/getList", "/admin/system/dept/save", "/admin/system/dept/upIMGroup"}
	case "/admin/system/dept/upIMGroup":
		return []string{path, "/admin/system/dept/save"}
	default:
		return []string{path}
	}
}
