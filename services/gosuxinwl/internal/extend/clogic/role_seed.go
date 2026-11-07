package clogic

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
)

// EnsureRoleManagementRules repairs the missing role-list/save capabilities.
// It never grants a role, changes existing rule IDs or re-enables disabled rules.
func EnsureRoleManagementRules(ctx context.Context, rules *gdb.Model) error {
	return rules.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		rules := rules.Clone().TX(tx).Ctx(ctx)
		parentQuery := rules.Clone().Where("type", 1).Where("routename", "role")
		if tx.GetDB().GetConfig().Type == "mysql" {
			parentQuery = parentQuery.LockUpdate()
		}
		parent, err := parentQuery.One()
		if err != nil || len(parent) == 0 {
			return err
		}
		for _, item := range []struct{ title, path, permission string }{
			{"查看角色", "/admin/system/role/getList", "list"},
			{"新增和编辑角色", "/admin/system/role/save", "add"},
		} {
			exists, err := rules.Clone().Where("type", 2).Where("path", item.path).Exist()
			if err != nil {
				return err
			}
			if exists {
				continue
			}
			_, err = rules.Clone().Data(map[string]any{
				"uid": 0, "pid": parent["id"].Int64(), "type": 2,
				"title": item.title, "des": item.title, "locale": "", "icon": "",
				"routepath": "", "component": "", "redirect": "", "routename": "",
				"path": item.path, "permission": item.permission, "weigh": 0,
				"status": parent["status"].Int(), "requiresauth": 1, "hideinmenu": 1,
				"hidechildreninmenu": 0, "onlypage": 0, "noaffix": 0, "keepalive": 0,
			}).Insert()
			if err != nil {
				return err
			}
		}
		return nil
	})
}
