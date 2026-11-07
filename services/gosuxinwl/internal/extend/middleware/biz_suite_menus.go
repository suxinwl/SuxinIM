package middleware

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

const BizSuiteHRMRoute = "runtime_plugin_biz-suite"
const BizSuiteIMRoute = "biz_suite_im"

// EnsureBizSuiteEntrypoints is shared by host installation and worker startup.
// Stable rule IDs keep existing role grants valid; no business permission or
// shared runtime-access button is granted by this menu migration.
func EnsureBizSuiteEntrypoints(ctx context.Context, rules, roles *gdb.Model) error {
	return rules.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		rules, roles := rules.Clone().TX(tx).Ctx(ctx), roles.Clone().TX(tx).Ctx(ctx)
		findPrimary := rules.Clone().Where("routename", BizSuiteHRMRoute).Where("type", 1)
		if tx.GetDB().GetConfig().Type == "mysql" {
			findPrimary = findPrimary.LockUpdate()
		}
		primary, err := findPrimary.One()
		if err != nil {
			return err
		}
		primaryID, primaryStatus := int64(0), 0
		if len(primary) > 0 {
			primaryID, primaryStatus = primary["id"].Int64(), primary["status"].Int()
		}
		primaryData := bizSuiteMenuData("智慧HRM", BizSuiteHRMRoute, "hrm", "icon-user-group")
		if primaryID == 0 {
			primaryData["status"] = 0
			primaryID, err = rules.Clone().Data(primaryData).InsertAndGetId()
		} else {
			_, err = rules.Clone().Where("id", primaryID).Data(primaryData).Update()
		}
		if err != nil {
			return err
		}

		// This gate is intentionally unchanged: both independent SPAs exchange the
		// host login through the same authenticated biz-suite runtime endpoint.
		gate, err := rules.Clone().Where("type", 2).Where("path", runtimePluginRulePath("biz-suite")).One()
		if err != nil {
			return err
		}
		gateData := map[string]any{
			"uid": 0, "pid": primaryID, "type": 2, "title": "访问HRM与IM管理端", "des": "业务插件共享运行访问权限（仍需各模块业务权限）",
			"locale": "runtime.plugin.biz-suite.admin", "icon": "icon-settings", "routepath": "/runtime-plugins/biz-suite/hrm", "routename": BizSuiteHRMRoute + "_admin",
			"component": "systool/plugin/runtime", "redirect": "", "path": runtimePluginRulePath("biz-suite"), "permission": "runtime.plugin.biz-suite.admin",
			"requiresauth": 1, "hideinmenu": 1, "hidechildreninmenu": 0, "onlypage": 1, "noaffix": 0, "keepalive": 0,
		}
		if len(gate) == 0 {
			gateData["status"] = 0
			_, err = rules.Clone().Data(gateData).Insert()
		} else {
			_, err = rules.Clone().Where("id", gate["id"]).Data(gateData).Update()
		}
		if err != nil {
			return err
		}

		secondary, err := rules.Clone().Where("type", 1).Where("routename", BizSuiteIMRoute).One()
		if err != nil {
			return err
		}
		created := len(secondary) == 0
		secondaryID := int64(0)
		secondaryData := bizSuiteMenuData("企业即时通讯", BizSuiteIMRoute, "im", "icon-message")
		if created {
			// An administrator-disabled old entry must not reappear through a new one.
			secondaryData["status"] = primaryStatus
			secondaryID, err = rules.Clone().Data(secondaryData).InsertAndGetId()
		} else {
			secondaryID = secondary["id"].Int64()
			_, err = rules.Clone().Where("id", secondaryID).Data(secondaryData).Update()
		}
		if err != nil {
			return err
		}
		// Only presentation parents move. Existing button IDs and status survive.
		for _, module := range []string{"hrm", "wecom", "im"} {
			parentID := primaryID
			if module == "im" {
				parentID = secondaryID
			}
			_, err = rules.Clone().Where("type", 2).WhereIn("permission", []string{"biz-suite." + module + ".read", "biz-suite." + module + ".write", "biz-suite." + module + ".manage"}).Data(map[string]any{"pid": parentID}).Update()
			if err != nil {
				return err
			}
		}
		if created && len(gate) > 0 && gate["status"].Int() == 0 && primaryStatus == 0 {
			return migrateBizSuiteIMMenu(rules, roles, secondaryID)
		}
		return nil
	})
}

func bizSuiteMenuData(title, route, module, icon string) map[string]any {
	path := "/runtime-plugins/biz-suite/" + module
	component, onlyPage := "systool/plugin/runtime", 1
	if module == "im" {
		path, component, onlyPage = "/communication", "communication/index", 0
	}
	return map[string]any{
		"uid": 0, "pid": 0, "type": 1, "title": title, "des": "业务系统独立入口", "locale": "biz-suite.entry." + module,
		"icon": icon, "routepath": path, "routename": route, "component": component, "redirect": "", "path": path,
		"permission": "runtime.plugin.biz-suite", "requiresauth": 1, "hideinmenu": 0, "hidechildreninmenu": 0,
		"activemenu": 0, "noaffix": 0, "onlypage": onlyPage, "isext": 0, "keepalive": 0,
	}
}

// Run only when the IM entry is first created. Later starts must respect an
// administrator removing that menu from a role. btns/status are never updated.
func migrateBizSuiteIMMenu(rules, roles *gdb.Model, newMenu int64) error {
	imRules, err := rules.Clone().Where("type", 2).Where("status", 0).WhereIn("permission", []string{"biz-suite.im.read", "biz-suite.im.manage"}).Array("id")
	if err != nil {
		return err
	}
	if len(imRules) == 0 {
		return nil
	}
	rows, err := roles.Clone().Where("status", 0).Fields("id,rules,btns").All()
	if err != nil {
		return err
	}
	for _, row := range rows {
		original := row["rules"].String()
		if original == "*" {
			continue
		}
		menuIDs, buttonIDs := bizSuiteRuleIDs(original), bizSuiteRuleIDs(row["btns"].String())
		if menuIDs[newMenu] {
			continue
		}
		canReadIM := false
		for _, rule := range imRules {
			if buttonIDs[rule.Int64()] {
				canReadIM = true
				break
			}
		}
		if !canReadIM {
			continue
		}
		// JSON-stored menus cannot be extended by appending a CSV suffix.
		// Normalize valid history; leave malformed rows unchanged and fail closed.
		storedMenus, err := gf.PermissionIDs(original)
		if err != nil {
			continue
		}
		updated := strings.Join(append(storedMenus, strconv.FormatInt(newMenu, 10)), ",")
		result, err := roles.Clone().Where("id", row["id"]).Where("rules", original).Data(map[string]any{"rules": updated}).Update()
		if err != nil {
			return err
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			return fmt.Errorf("role menu changed during biz-suite migration; retry startup")
		}
	}
	return nil
}

// FilterBizSuiteIMMenu only removes a candidate entry. The caller's normal
// role-menu query still controls whether it exists in the first place, so an
// administrator's later menu removal is never undone. Access is checked across
// all active roles, matching the existing host gate and worker RBAC semantics.
func FilterBizSuiteIMMenu(ctx context.Context, rows gdb.Result) gdb.Result {
	present := false
	for _, row := range rows {
		if row["routename"].String() == BizSuiteIMRoute {
			present = true
			break
		}
	}
	if !present {
		return rows
	}
	allowed := false
	if uid := ctx.Value("uid"); uid != nil {
		roleIDs, err := dao.AuthRoleAccess.Ctx(ctx).Where("uid", uid).Array("role_id")
		if err == nil && len(roleIDs) > 0 {
			roles, err := dao.AuthRole.Ctx(ctx).WhereIn("id", roleIDs).Where("status", 0).Fields("rules,btns").All()
			if err == nil {
				requireSuperRules, _ := g.Cfg("app").Get(ctx, "app.superRoleAuth")
				allowed, err = canSeeBizSuiteIMMenu(dao.AuthRule.Ctx(ctx), roles, requireSuperRules.Bool())
				if err != nil {
					allowed = false
				}
			}
		}
	}
	return filterBizSuiteIMRows(rows, allowed)
}

func filterBizSuiteIMRows(rows gdb.Result, allowed bool) gdb.Result {
	if allowed {
		return rows
	}
	filtered := make(gdb.Result, 0, len(rows))
	for _, row := range rows {
		if row["routename"].String() != BizSuiteIMRoute {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func canSeeBizSuiteIMMenu(rules *gdb.Model, roles gdb.Result, requireSuperRules bool) (bool, error) {
	isSuper, ids := false, map[int64]bool{}
	for _, role := range roles {
		if role["rules"].String() == "*" {
			isSuper = true
		}
		for id := range bizSuiteRuleIDs(role["btns"].String()) {
			ids[id] = true
		}
	}
	if isSuper && !requireSuperRules {
		return true, nil
	}
	query := rules.Clone().Where("status", 0).Where("type", 2)
	if !isSuper {
		if len(ids) == 0 {
			return false, nil
		}
		granted := make([]int64, 0, len(ids))
		for id := range ids {
			granted = append(granted, id)
		}
		query = query.WhereIn("id", granted)
	}
	rows, err := query.Fields("path,permission").All()
	if err != nil {
		return false, err
	}
	gate, module := false, false
	for _, rule := range rows {
		if rule["path"].String() == runtimePluginRulePath("biz-suite") {
			gate = true
		}
		if p := rule["permission"].String(); p == "biz-suite.im.read" || p == "biz-suite.im.manage" {
			module = true
		}
	}
	return gate && module, nil
}

func bizSuiteRuleIDs(value string) map[int64]bool {
	ids := map[int64]bool{}
	fields, err := gf.PermissionIDs(value)
	if err != nil {
		return ids
	}
	for _, field := range fields {
		if id, err := strconv.ParseInt(field, 10, 64); err == nil && id > 0 {
			ids[id] = true
		}
	}
	return ids
}
