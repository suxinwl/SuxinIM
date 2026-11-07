package organization

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

var syncMu sync.Mutex

func model(ctx context.Context, tx gdb.TX, table string) *gdb.Model {
	return core.Model(table).TX(tx).Ctx(ctx)
}
func host(ctx context.Context, tx gdb.TX, table string) *gdb.Model {
	return core.HostModel(table).TX(tx).Ctx(ctx)
}
func jsonText(v any) string { b, _ := json.Marshal(v); return string(b) }

func locked(ctx context.Context, work func(context.Context, gdb.TX) error) error {
	syncMu.Lock()
	defer syncMu.Unlock()
	return core.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		q := model(ctx, tx, "biz_org_sync_state").Where("id", 1)
		if core.DB().GetConfig().Type == "mysql" {
			q = q.LockUpdate()
		}
		row, err := q.One()
		if err != nil {
			return err
		}
		if len(row) == 0 {
			return fmt.Errorf("组织映射数据库尚未初始化")
		}
		return work(ctx, tx)
	})
}

// SeedRoles only creates a template once. Subsequent restarts preserve every
// administrator change (including disabled/deleted roles and removed grants).
func SeedRoles(ctx context.Context) error {
	return locked(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, spec := range []struct{ key, name, scope, remark string }{
			{"employee", "员工", "self", "查看本人考勤，使用即时通讯；新账号停用，管理员设置密码后启用"},
			{"staff", "职员", "self", "查看本人考勤，使用即时通讯"},
			{"department_manager", "部门负责人", "department", "查看本人及主部门、下级部门考勤；由管理员分配，不按职位名称自动提权"},
			{"hr_manager", "人事管理员", "manage", "管理全公司HRM，使用即时通讯；不含企业微信配置、宿主系统管理"},
		} {
			exists, err := model(ctx, tx, "biz_org_role").Where("role_key", spec.key).Exist()
			if err != nil {
				return err
			}
			if exists {
				continue
			}
			roleName := spec.name
			for suffix := 1; ; suffix++ {
				duplicate, err := host(ctx, tx, "auth_role").Where("name", roleName).Exist()
				if err != nil {
					return err
				}
				if !duplicate {
					break
				}
				// Never adopt a same-name role whose permissions may be unrelated.
				roleName = fmt.Sprintf("%s（企业微信%d）", spec.name, suffix)
			}
			menus := []int64{}
			for _, route := range []string{"runtime_plugin_biz-suite", "biz_suite_im"} {
				row, err := host(ctx, tx, "auth_rule").Where("routename", route).Where("type", 1).Where("status", 0).One()
				if err != nil {
					return err
				}
				if len(row) == 0 {
					return fmt.Errorf("角色菜单尚未初始化")
				}
				menus = append(menus, row["id"].Int64())
			}
			buttons := []int64{}
			for _, permission := range []string{"runtime.plugin.biz-suite.admin", "biz-suite.hrm." + spec.scope, "biz-suite.im.read", "biz-suite.im.write"} {
				row, err := host(ctx, tx, "auth_rule").Where("permission", permission).Where("type", 2).Where("status", 0).One()
				if err != nil {
					return err
				}
				if len(row) == 0 {
					return fmt.Errorf("基础角色权限尚未初始化: %s", permission)
				}
				buttons = append(buttons, row["id"].Int64())
			}
			// Host 'rules' includes menu ancestry and button IDs for UI metadata;
			// 'btns' is the separately enforced API permission list.
			rules := append([]int64{}, menus...)
			seen := map[int64]bool{}
			for _, id := range rules {
				seen[id] = true
			}
			for i := 0; i < len(rules); i++ {
				row, err := host(ctx, tx, "auth_rule").Where("id", rules[i]).One()
				if err != nil {
					return err
				}
				pid := row["pid"].Int64()
				if pid > 0 && !seen[pid] {
					seen[pid] = true
					rules = append(rules, pid)
				}
			}
			rules = append(rules, buttons...)
			ruleIDs := make([]string, 0, len(rules))
			for _, id := range rules {
				ruleIDs = append(ruleIDs, strconv.FormatInt(id, 10))
			}
			super, err := host(ctx, tx, "auth_role").Where("rules", "*").OrderAsc("id").Value("id")
			if err != nil {
				return err
			}
			id, err := host(ctx, tx, "auth_role").Data(g.Map{"business_id": 0, "account_id": 0, "pid": super.Int64(), "name": roleName, "rules": strings.Join(ruleIDs, ","), "menu": jsonText(menus), "btns": jsonText(buttons), "status": 0, "data_access": 0, "remark": spec.remark, "weigh": 0, "createtime": core.Now()}).InsertAndGetId()
			if err != nil {
				return err
			}
			if _, err = model(ctx, tx, "biz_org_role").Data(g.Map{"role_key": spec.key, "role_id": id, "created_at": core.Now()}).Insert(); err != nil {
				return err
			}
		}
		return nil
	})
}
