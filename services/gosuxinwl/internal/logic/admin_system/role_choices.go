package adminsystem

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

// The host stores visible menus in rules and executable capabilities in btns.
// Keep their assignment lists separate, including when selecting a parent role.
func roleMenuChoices(ctx context.Context, roles, rules, access *gdb.Model, uid, parentID int64) *gf.R {
	a, err := readRoleAuthority(ctx, roles, access, uid)
	if err != nil {
		return gf.Failed().SetMsg(err.Error())
	}
	parentSuper := true
	parentMenus, parentButtons := map[int64]bool{}, map[int64]bool{}
	if parentID > 0 {
		rows, err := roles.Clone().Ctx(ctx).All()
		if err != nil {
			return gf.Failed().SetMsg("角色读取失败")
		}
		parents := map[int64]int64{}
		var selected gdb.Record
		for _, row := range rows {
			id := row["id"].Int64()
			parents[id] = row["pid"].Int64()
			if id == parentID {
				selected = row
			}
		}
		if len(selected) == 0 || selected["status"].Int() != 0 {
			return gf.Failed().SetMsg("上级角色不存在或已停用")
		}
		if !a.super && !roleDescendant(parentID, parents, a.assigned) {
			return gf.Failed().SetMsg("上级角色不在可管理范围内")
		}
		parentSuper = selected["rules"].String() == "*"
		parentMenus = roleCSV(selected["rules"].String())
		ids, err := storedRoleButtons(selected["btns"].String())
		if err != nil {
			return gf.Failed().SetMsg("上级角色按钮权限格式无效")
		}
		for _, id := range ids {
			parentButtons[id] = true
		}
	}
	rows, err := rules.Clone().Ctx(ctx).Where("status", 0).OrderAsc("weigh").OrderAsc("id").All()
	if err != nil {
		return gf.Failed().SetMsg("菜单权限读取失败")
	}
	menus := gdb.Result{}
	menuParents := map[int64]int64{}
	for _, row := range rows {
		id := row["id"].Int64()
		if row["type"].Int() == 2 || (!a.super && !a.menus[id]) || (!parentSuper && !parentMenus[id]) {
			continue
		}
		title := row["title"]
		if title.String() == "" {
			title = row["locale"]
		}
		menus = append(menus, gdb.Record{"id": row["id"], "pid": row["pid"], "title": title, "checkable": gvar.New(true)})
		menuParents[id] = row["pid"].Int64()
	}
	buttons := gdb.Result{}
	buttonIDs := []int64{}
	for _, row := range rows {
		id, pid := row["id"].Int64(), row["pid"].Int64()
		if row["type"].Int() != 2 || (!a.super && !a.buttons[id]) || (!parentSuper && !parentButtons[id]) {
			continue
		}
		if _, ok := menuParents[pid]; !ok {
			continue
		}
		buttons = append(buttons, gdb.Record{"id": row["id"], "pid": row["pid"], "title": row["title"], "des": row["des"], "locale": row["locale"]})
		buttonIDs = append(buttonIDs, id)
	}
	tree := gf.GetMenuChildrenArray(menus, 0, "pid")
	for _, menu := range menus {
		id := menu["id"].Int64()
		direct := gdb.Result{}
		descendants := []int64{}
		for _, button := range buttons {
			pid := button["pid"].Int64()
			if pid == id {
				direct = append(direct, button)
			}
			if roleDescendant(pid, menuParents, map[int64]bool{id: true}) {
				descendants = append(descendants, button["id"].Int64())
			}
		}
		menu["btnids"] = gvar.New(descendants)
		if len(direct) > 0 {
			children := []any{}
			if existing := menu["children"]; existing != nil {
				children = existing.Interfaces()
			}
			children = append(children, gf.Map{"title": "按钮权限", "id": direct[0]["id"], "pid": menu["id"], "checkable": false, "btn_rules": direct})
			menu["children"] = gvar.New(children)
		}
	}
	if tree == nil {
		tree = gdb.Result{}
	}
	return gf.Success().SetData(gf.Map{"list": tree, "btn_rule_ids": buttonIDs})
}
