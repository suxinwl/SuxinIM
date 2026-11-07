package middleware

import (
	"context"
	"fmt"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/internal/plugins"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"regexp"
	"strings"
)

var runtimePluginPathPattern = regexp.MustCompile(`^/plugins/([a-z][a-z0-9-]{0,63})/(?:admin|api/admin)(?:/|$)`)

func runtimePluginName(path string) (string, bool) {
	match := runtimePluginPathPattern.FindStringSubmatch(strings.TrimRight(path, "/"))
	if len(match) != 2 {
		return "", false
	}
	return match[1], true
}

func runtimePluginRulePath(plugin string) string {
	return "/plugins/" + plugin + "/admin"
}

// EnsureRuntimePluginRule creates a visible menu node and a button rule for a
// runtime plugin. Creating the rule does not grant it to any role; role
// administrators must explicitly select the button for an authorized role.
func EnsureRuntimePluginRule(ctx context.Context, plugin string, titles ...string) error {
	if !regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`).MatchString(plugin) {
		return fmt.Errorf("invalid runtime plugin name")
	}
	if plugin == "biz-suite" {
		return EnsureBizSuiteEntrypoints(ctx, dao.AuthRule.Ctx(ctx), dao.AuthRole.Ctx(ctx))
	}
	routeName := "runtime_plugin_" + plugin
	title := plugin
	if len(titles) > 0 && strings.TrimSpace(titles[0]) != "" {
		title = strings.TrimSpace(titles[0])
	}
	parent, err := dao.AuthRule.Ctx(ctx).Where("type", 1).Where("routename", routeName).Value("id")
	if err != nil {
		return err
	}
	parentID := int64(0)
	if parent != nil {
		parentID = parent.Int64()
	}
	if parentID == 0 {
		var err error
		parentID, err = dao.AuthRule.Ctx(ctx).Data(map[string]interface{}{
			"uid":                0,
			"title":              title,
			"des":                "运行插件管理入口",
			"locale":             "runtime.plugin." + plugin,
			"weigh":              0,
			"type":               1,
			"pid":                0,
			"icon":               "icon-apps",
			"routepath":          "/runtime-plugins/" + plugin,
			"routename":          routeName,
			"component":          "systool/plugin/runtime",
			"redirect":           "/runtime-plugins/" + plugin,
			"path":               "/runtime-plugins/" + plugin,
			"permission":         "runtime.plugin." + plugin,
			"requiresauth":       1,
			"hideinmenu":         0,
			"hidechildreninmenu": 0,
			"activemenu":         0,
			"noaffix":            0,
			"onlypage":           0,
			"isext":              0,
			"keepalive":          0,
			"status":             0,
		}).InsertAndGetId()
		if err != nil {
			return err
		}
	} else {
		// Repair entries provisioned by earlier runtime hosts, without changing
		// role grants or re-enabling an administrator-disabled menu.
		_, err = dao.AuthRule.Ctx(ctx).Where("id", parentID).Data(map[string]interface{}{
			"title": title, "locale": "runtime.plugin." + plugin,
			"des": "运行插件管理入口", "icon": "icon-apps",
			"routepath": "/runtime-plugins/" + plugin, "routename": routeName,
			"component": "systool/plugin/runtime", "redirect": "/runtime-plugins/" + plugin,
			"path": "/runtime-plugins/" + plugin, "permission": "runtime.plugin." + plugin,
			"hideinmenu": 0, "onlypage": 0, "requiresauth": 1,
		}).Update()
		if err != nil {
			return err
		}
	}
	rulePath := runtimePluginRulePath(plugin)
	existing, err := dao.AuthRule.Ctx(ctx).Where("type", 2).Where("path", rulePath).Value("id")
	if err != nil {
		return err
	}
	if existing != nil && existing.Int64() != 0 {
		_, err = dao.AuthRule.Ctx(ctx).Where("id", existing.Int64()).Data(map[string]interface{}{
			"pid":                parentID,
			"title":              "访问管理端",
			"des":                "运行插件管理权限",
			"locale":             "runtime.plugin." + plugin + ".admin",
			"icon":               "icon-settings",
			"routepath":          "/runtime-plugins/" + plugin,
			"routename":          routeName + "_admin",
			"component":          "systool/plugin/runtime",
			"redirect":           "/runtime-plugins/" + plugin,
			"permission":         "runtime.plugin." + plugin + ".admin",
			"requiresauth":       1,
			"hideinmenu":         0,
			"hidechildreninmenu": 0,
			"onlypage":           0,
			"noaffix":            0,
			"keepalive":          0,
		}).Update()
		return err
	}
	_, err = dao.AuthRule.Ctx(ctx).Data(map[string]interface{}{
		"uid":          0,
		"pid":          parentID,
		"title":        "访问管理端",
		"des":          "运行插件管理权限",
		"locale":       "runtime.plugin." + plugin + ".admin",
		"type":         2,
		"icon":         "icon-settings",
		"routepath":    "/runtime-plugins/" + plugin,
		"routename":    routeName + "_admin",
		"component":    "systool/plugin/runtime",
		"redirect":     "/runtime-plugins/" + plugin,
		"path":         rulePath,
		"permission":   "runtime.plugin." + plugin + ".admin",
		"status":       0,
		"requiresauth": 1,
	}).InsertAndGetId()
	return err
}

func checkRuntimePluginAuth(r *ghttp.Request, plugin string) bool {
	uid := r.Context().Value("uid")
	if uid == nil {
		return false
	}
	ctx := r.Context()
	roleIDs, err := dao.AuthRoleAccess.Ctx(ctx).Where("uid", uid).Array("role_id")
	if err != nil || len(roleIDs) == 0 {
		return false
	}
	roles := dao.AuthRole.Ctx(ctx).WhereIn("id", roleIDs).Where("status", 0)
	isSuper, err := dao.AuthRole.Ctx(ctx).WhereIn("id", roleIDs).Where("status", 0).Where("rules", "*").Exist()
	if err != nil {
		return false
	}
	superRoleAuth, _ := g.Cfg("app").Get(ctx, "app.superRoleAuth")
	if isSuper && !superRoleAuth.Bool() {
		return true
	}
	// Native plugins keep their existing menu and button grants. API aliases
	// still enforce the exact original action rule before reaching the worker.
	if plugins.Active(plugin) {
		prefix := "/admin/" + plugin + "/"
		if plugin == "ebook" {
			prefix = "/admin/album/"
		}
		query := dao.AuthRule.Ctx(ctx).Where("status", 0).Where("type", 2).WhereLike("path", prefix+"%")
		if !isSuper {
			buttons, e := roles.Array("btns")
			if e != nil {
				return false
			}
			query = query.WhereIn("id", gf.ArrayMerge(buttons))
		}
		allowed, e := query.Exist()
		return e == nil && allowed
	}

	rulePath := runtimePluginRulePath(plugin)
	ruleIDs, err := dao.AuthRule.Ctx(ctx).Where("status", 0).Where("type", 2).Where("path", rulePath).Array("id")
	if err != nil || len(ruleIDs) == 0 {
		return false
	}
	if isSuper {
		return true
	}
	btnIDs, err := roles.Array("btns")
	if err != nil {
		return false
	}
	for _, wanted := range ruleIDs {
		for _, granted := range gf.ArrayMerge(btnIDs) {
			if gconv.String(wanted) == gconv.String(granted) {
				return true
			}
		}
	}
	return false
}
