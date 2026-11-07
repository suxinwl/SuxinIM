package clogic

import (
	"context"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
)

// Add capabilities without granting them or changing existing role selections.
// Self-service password changes are authenticated own-account operations.
func EnsureAccountSecurityRules(ctx context.Context, rules *gdb.Model) error {
	return rules.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		m := rules.Clone().TX(tx).Ctx(ctx)
		for _, item := range []struct{ parent, title, path, permission string }{
			{"account", "查看账号安全状态", "/admin/system/account/security", "security"},
			{"account", "重置密码", "/admin/system/account/resetPassword", "resetPassword"},
			{"account", "解锁账号", "/admin/system/account/unlock", "unlock"},
			{"account", "登录锁定策略", "/admin/system/account/lockPolicy", "lockPolicy"},
			{"account", "个人登录与保存权限", "/admin/system/account/accessPolicy", "accessPolicy"},
			{"role", "角色登录与保存权限", "/admin/system/role/accessPolicy", "accessPolicy"},
		} {
			exists, e := m.Clone().Where("type", 2).Where("path", item.path).Exist()
			if e != nil {
				return e
			}
			if exists {
				continue
			}
			parent, e := m.Clone().Where("type", 1).Where("routename", item.parent).One()
			if e != nil {
				return e
			}
			if len(parent) == 0 {
				continue
			}
			_, e = m.Clone().Data(map[string]any{"uid": 0, "pid": parent["id"].Int64(), "type": 2, "title": item.title, "des": item.title, "locale": "", "icon": "", "routepath": "", "component": "", "redirect": "", "routename": "", "path": item.path, "permission": item.permission, "weigh": 0, "status": parent["status"].Int(), "requiresauth": 1, "hideinmenu": 1, "hidechildreninmenu": 0, "onlypage": 0, "noaffix": 0, "keepalive": 0}).Insert()
			if e != nil {
				return e
			}
		}
		_, e := m.Clone().Where("path", "/admin/user/setting/saveInfo").Data(map[string]any{"path": "/admin/user/saveInfo"}).Update()
		return e
	})
}
