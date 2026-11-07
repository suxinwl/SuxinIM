package core

import (
	"context"
	"fmt"
	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/internal/extend/middleware"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"net/http"
	"strings"
)

var Modules = []string{"hrm", "im", "wecom"}
var Actions = []string{"read", "write", "manage"}

func Permissions(ctx context.Context) ([]string, error) {
	roles, err := HostModel("auth_role_access").Ctx(ctx).Where("uid", UID(ctx)).Array("role_id")
	if err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		return []string{}, nil
	}
	rows, err := HostModel("auth_role").Ctx(ctx).WhereIn("id", roles).Where("status", 0).All()
	if err != nil {
		return nil, err
	}
	buttons := []any{}
	isSuper := false
	for _, row := range rows {
		if row["rules"].String() == "*" {
			isSuper = true
			checkRules, _ := g.Cfg("app").Get(ctx, "app.superRoleAuth")
			if checkRules.Bool() {
				continue
			}
			out := []string{}
			for _, module := range Modules {
				for _, action := range Actions {
					out = append(out, "biz-suite."+module+"."+action)
				}
			}
			return out, nil
		}
		buttons = append(buttons, gf.ArrayMerge([]*gvar.Var{row["btns"]})...)
	}
	if len(buttons) == 0 && !isSuper {
		return []string{}, nil
	}
	query := HostModel("auth_rule").Ctx(ctx).Where("status", 0).Where("type", 2).WhereLike("permission", "biz-suite.%")
	if !isSuper {
		query = query.WhereIn("id", buttons)
	}
	rules, err := query.Array("permission")
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, p := range rules {
		out = append(out, p.String())
	}
	return out, nil
}
func Has(ctx context.Context, permission string) bool {
	perms, err := Permissions(ctx)
	if err != nil {
		return false
	}
	for _, p := range perms {
		if permission == "biz-suite.hrm.read" && (p == "biz-suite.hrm.self" || p == "biz-suite.hrm.department" || p == "biz-suite.hrm.write") {
			return true
		}
		prefix := permission[:strings.LastIndex(permission, ".")+1]
		if p == permission || p == prefix+"manage" {
			return true
		}
	}
	return false
}
func Middleware(r *ghttp.Request) {
	active, err := HostUserExists(r.Context(), UID(r.Context()))
	if err != nil || !active {
		r.Response.WriteStatus(401)
		r.ExitAll()
		return
	}
	path := strings.TrimRight(strings.TrimPrefix(r.URL.Path, "/api/admin/"), "/")
	if path == "access" {
		r.Middleware.Next()
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return
	}
	module := parts[0]
	action := "read"
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		action = "write"
	}
	if strings.Contains(path, "/manager/") || strings.Contains(path, "sensitive-words") || strings.Contains(path, "/etl") || strings.HasPrefix(path, "wecom/diagnostics/") || path == "wecom/credentials" || strings.HasPrefix(path, "wecom/credentials/") || (action == "write" && (strings.Contains(path, "face-packs") || path == "im/channels" || path == "wecom/settings")) {
		action = "manage"
	}
	if path == "wecom/organization/sync" || path == "wecom/organization/prune" || (action == "write" && (path == "wecom/departments/mapping" || path == "wecom/sync/contact")) {
		action = "manage"
	}
	if !Has(r.Context(), "biz-suite."+module+"."+action) {
		r.Response.WriteStatus(403)
		r.ExitAll()
		return
	}
	r.Middleware.Next()
}
func BindAccess(group *ghttp.RouterGroup) {
	group.GET("/api/admin/access", func(r *ghttp.Request) {
		p, err := Permissions(r.Context())
		if err != nil {
			r.Response.WriteJson(gf.Failed().SetMsg("权限读取失败"))
			return
		}
		r.Response.WriteJson(gf.Success().SetData(g.Map{"uid": UID(r.Context()), "permissions": p, "menus": entryMenus(p)}))
	})
	// Reuse the normal host session validation/refresh window and this module's
	// read permission. No token is issued and a revoked session cannot log in.
	group.GET("/api/admin/im/session", func(r *ghttp.Request) {
		r.Response.Header().Set("Cache-Control", "no-store")
		p, err := Permissions(r.Context())
		if err != nil {
			r.Response.WriteHeader(http.StatusServiceUnavailable)
			r.Response.WriteJson(gf.Failed().SetMsg("权限读取失败"))
			return
		}
		r.Response.WriteJson(gf.Success().SetData(g.Map{"uid": UID(r.Context()), "permissions": p}))
	})
}

// Seed adds only missing permission rows under the runtime menu; role grants
// remain under the host administrator's control.
func Seed(ctx context.Context) error {
	if err := middleware.EnsureBizSuiteEntrypoints(ctx, HostModel("auth_rule"), HostModel("auth_role")); err != nil {
		return err
	}
	parent, err := HostModel("auth_rule").Ctx(ctx).Where("routename", "runtime_plugin_biz-suite").Value("id")
	if err != nil {
		return err
	}
	if parent.Int64() <= 0 {
		return fmt.Errorf("runtime plugin menu must be installed before permissions")
	}
	for _, module := range Modules {
		parentID := parent.Int64()
		if module == "im" {
			imParent, err := HostModel("auth_rule").Ctx(ctx).Where("routename", middleware.BizSuiteIMRoute).Value("id")
			if err != nil || imParent.Int64() <= 0 {
				return fmt.Errorf("IM menu must be installed before permissions")
			}
			parentID = imParent.Int64()
		}
		actions := append([]string{}, Actions...)
		if module == "hrm" {
			actions = append(actions, "self", "department")
		}
		for _, action := range actions {
			permission := "biz-suite." + module + "." + action
			exists, err := HostModel("auth_rule").Ctx(ctx).Where("permission", permission).Exist()
			if err != nil {
				return err
			}
			if exists {
				continue
			}
			_, err = HostModel("auth_rule").Ctx(ctx).Data(g.Map{"uid": 0, "pid": parentID, "type": 2, "title": module + " " + action, "des": "业务插件权限", "locale": permission, "icon": "", "routepath": "", "component": "", "redirect": "", "routename": permission, "path": "/plugins/biz-suite/api/admin/" + module + "/" + action, "permission": permission, "status": 0, "requiresauth": 1, "hideinmenu": 1}).Insert()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func entryMenus(permissions []string) []g.Map {
	allowed := map[string]bool{}
	for _, permission := range permissions {
		allowed[permission] = true
	}
	read := func(module string) bool {
		if module == "hrm" && (allowed["biz-suite.hrm.self"] || allowed["biz-suite.hrm.department"] || allowed["biz-suite.hrm.write"]) {
			return true
		}
		return allowed["biz-suite."+module+".read"] || allowed["biz-suite."+module+".manage"]
	}
	menus := []g.Map{}
	if read("hrm") || read("wecom") {
		path := "/hrm"
		if !read("hrm") {
			path = "/wecom"
		}
		menus = append(menus, g.Map{"title": "智慧HRM", "path": path})
	}
	if read("im") {
		menus = append(menus, g.Map{"title": "企业即时通讯", "path": "/im"})
	}
	return menus
}
