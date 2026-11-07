// Package access shares the developer tools' administrator policy across host and workers.
package access

import (
	"context"
	"errors"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/internal/extend/middleware"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

func Authorize(ctx context.Context, uid int64) error {
	if uid < 1 {
		return errors.New("管理员身份无效")
	}
	a := dao.Admin.Columns()
	count, err := dao.Admin.Ctx(ctx).Where(a.Id, uid).Where(a.Status, 0).Count()
	if err != nil || count != 1 {
		return errors.New("管理员账号已禁用或不存在")
	}
	ra := dao.AuthRoleAccess.Columns()
	roles, err := dao.AuthRoleAccess.Ctx(ctx).Where(ra.Uid, uid).Array(ra.RoleId)
	if err != nil || len(roles) == 0 {
		return errors.New("开发工具仅允许超级管理员访问")
	}
	role := dao.AuthRole.Columns()
	count, err = dao.AuthRole.Ctx(ctx).WhereIn(role.Id, roles).Where(role.Status, 0).Where(role.Rules, "*").Count()
	if err != nil || count < 1 {
		return errors.New("开发工具仅允许启用的超级管理员访问")
	}
	return nil
}
func RequireAdmin(r *ghttp.Request) {
	if err := Authorize(r.Context(), gconv.Int64(r.Context().Value("uid"))); err != nil {
		r.Response.WriteHeader(403)
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		r.ExitAll()
		return
	}
	r.Response.Header().Set("Cache-Control", "no-store")
	r.Middleware.Next()
}
func RequireWorkerAdmin(r *ghttp.Request) {
	if !middleware.TrustedPluginIdentity(r) {
		r.Response.WriteStatus(401)
		r.ExitAll()
		return
	}
	RequireAdmin(r)
}
