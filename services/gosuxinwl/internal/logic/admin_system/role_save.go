package adminsystem

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/suxinwl/GoSuxin/internal/accountsecurity"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/suxinwl/GoSuxin/api/admin/system"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/internal/dao"
)

// roleIDs accepts typed slices and the JSON arrays returned by existing roles.
// Empty selections are valid; malformed input must never panic or become '*'.
func roleIDs(value any) ([]int64, error) {
	if value == nil {
		return []int64{}, nil
	}
	var raw []byte
	var err error
	if text, ok := value.(string); ok {
		if strings.TrimSpace(text) == "" {
			return []int64{}, nil
		}
		raw = []byte(text)
	} else {
		raw, err = json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("角色权限应为编号数组")
		}
	}
	var values []json.RawMessage
	if err = json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("角色权限应为编号数组")
	}
	ids, seen := []int64{}, map[int64]bool{}
	for _, value := range values {
		text := string(value)
		if len(text) > 0 && text[0] == '"' {
			if err = json.Unmarshal(value, &text); err != nil {
				return nil, fmt.Errorf("角色权限编号无效")
			}
		}
		id, err := strconv.ParseInt(text, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("角色权限编号必须为正整数")
		}
		if !seen[id] {
			ids, seen[id] = append(ids, id), true
		}
	}
	return ids, nil
}

func roleCSV(value string) map[int64]bool {
	out := map[int64]bool{}
	value = strings.TrimSpace(value)
	// Runtime role templates store a JSON array; older host roles use CSV.
	// Reject an invalid collection as a whole, rather than retaining a subset
	// whose brackets or malformed IDs happened to be ignored.
	if strings.HasPrefix(value, "[") {
		var ids []int64
		if err := json.Unmarshal([]byte(value), &ids); err != nil {
			return out
		}
		for _, id := range ids {
			if id <= 0 {
				return map[int64]bool{}
			}
			out[id] = true
		}
		return out
	}
	if value == "" || value == "*" {
		return out
	}
	for _, text := range strings.Split(value, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		if err != nil || id <= 0 {
			return map[int64]bool{}
		}
		out[id] = true
	}
	return out
}

// Older installations also stored button IDs as comma-separated text.
func storedRoleButtons(value string) ([]int64, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "[") {
		return roleIDs(value)
	}
	parts := strings.Split(value, ",")
	return roleIDs(parts)
}

type roleAuthority struct {
	super                    bool
	assigned, menus, buttons map[int64]bool
	maxDataAccess            int
}

func readRoleAuthority(ctx context.Context, roles, access *gdb.Model, uid int64) (roleAuthority, error) {
	a := roleAuthority{assigned: map[int64]bool{}, menus: map[int64]bool{}, buttons: map[int64]bool{}}
	if uid <= 0 {
		return a, fmt.Errorf("请先登录")
	}
	ids, err := access.Clone().Ctx(ctx).Where("uid", uid).Array("role_id")
	if err != nil || len(ids) == 0 {
		return a, fmt.Errorf("没有可用角色权限")
	}
	rows, err := roles.Clone().Ctx(ctx).WhereIn("id", ids).Where("status", 0).All()
	if err != nil {
		return a, err
	}
	hasSelf, hasChildren := false, false
	for _, row := range rows {
		a.assigned[row["id"].Int64()] = true
		a.super = a.super || row["rules"].String() == "*"
		for id := range roleCSV(row["rules"].String()) {
			a.menus[id] = true
		}
		buttons, err := storedRoleButtons(row["btns"].String())
		if err != nil {
			return a, fmt.Errorf("当前角色按钮权限格式无效")
		}
		for _, id := range buttons {
			a.buttons[id] = true
		}
		hasSelf = hasSelf || row["data_access"].Int() == 0
		hasChildren = hasChildren || row["data_access"].Int() == 1
	}
	if len(a.assigned) == 0 {
		return a, fmt.Errorf("没有可用角色权限")
	}
	// Preserve GetDataAuthor's existing combination semantics (1, then 0,
	// otherwise 2) instead of granting the broadest numeric role automatically.
	if hasChildren {
		a.maxDataAccess = 1
	} else if !hasSelf {
		a.maxDataAccess = 2
	}
	return a, nil
}

func roleDescendant(id int64, parents map[int64]int64, roots map[int64]bool) bool {
	seen := map[int64]bool{}
	for id > 0 && !seen[id] {
		if roots[id] {
			return true
		}
		seen[id] = true
		id = parents[id]
	}
	return false
}

func saveRoleRecord(ctx context.Context, roles, rules, access *gdb.Model, uid int64, req *system.RoleSaveReq, security ...*accountsecurity.Store) (int64, error) {
	if req == nil || req.Id < 0 || req.Pid < 0 {
		return 0, fmt.Errorf("角色编号无效")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || utf8.RuneCountInString(name) > 50 {
		return 0, fmt.Errorf("角色名称应为1至50个字符")
	}
	if req.DataAccess < 0 || req.DataAccess > 2 {
		return 0, fmt.Errorf("数据权限无效")
	}
	menuIDs, err := roleIDs(req.Menu)
	if err != nil {
		return 0, err
	}
	buttonIDs, err := roleIDs(req.Bbtns)
	if err != nil {
		return 0, err
	}
	var savedID int64
	err = roles.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		roles, rules, access := roles.Clone().TX(tx).Ctx(ctx), rules.Clone().TX(tx).Ctx(ctx), access.Clone().TX(tx).Ctx(ctx)
		a, err := readRoleAuthority(ctx, roles, access, uid)
		if err != nil {
			return err
		}
		allRoles, err := roles.Clone().All()
		if err != nil {
			return err
		}
		byRole, parents := map[int64]gdb.Record{}, map[int64]int64{}
		for _, row := range allRoles {
			id := row["id"].Int64()
			byRole[id], parents[id] = row, row["pid"].Int64()
		}
		if req.Id > 0 {
			existing := byRole[req.Id]
			if len(existing) == 0 {
				return fmt.Errorf("角色不存在")
			}
			if existing["rules"].String() == "*" {
				return fmt.Errorf("内置超级角色不能在普通角色表单中修改")
			}
			if !a.super && (a.assigned[req.Id] || !roleDescendant(req.Id, parents, a.assigned)) {
				return fmt.Errorf("只能编辑所管理的下级角色")
			}
		}
		if req.Pid > 0 {
			parent := byRole[req.Pid]
			if len(parent) == 0 || parent["status"].Int() != 0 {
				return fmt.Errorf("上级角色不存在或已停用")
			}
			if req.Id > 0 && roleDescendant(req.Pid, parents, map[int64]bool{req.Id: true}) {
				return fmt.Errorf("上级角色不能是自身或其下级角色")
			}
			if !a.super && !roleDescendant(req.Pid, parents, a.assigned) {
				return fmt.Errorf("上级角色不在可管理范围内")
			}
		} else if !a.super {
			return fmt.Errorf("不能创建或移动到顶级角色")
		}
		if !a.super && req.DataAccess > a.maxDataAccess {
			return fmt.Errorf("不能授予超出自身范围的数据权限")
		}
		allRules, err := rules.Clone().Where("status", 0).All()
		if err != nil {
			return err
		}
		byRule := map[int64]gdb.Record{}
		for _, row := range allRules {
			byRule[row["id"].Int64()] = row
		}
		parentMenus, parentButtons := map[int64]bool{}, map[int64]bool{}
		parentSuper := req.Pid == 0 || byRole[req.Pid]["rules"].String() == "*"
		if !parentSuper {
			parentMenus = roleCSV(byRole[req.Pid]["rules"].String())
			ids, err := storedRoleButtons(byRole[req.Pid]["btns"].String())
			if err != nil {
				return fmt.Errorf("上级角色按钮权限格式无效")
			}
			for _, id := range ids {
				parentButtons[id] = true
			}
		}
		granted := map[int64]bool{}
		for _, id := range menuIDs {
			seen := map[int64]bool{}
			for id > 0 {
				row, exists := byRule[id]
				if !exists || row["type"].Int() == 2 {
					return fmt.Errorf("所选菜单不存在或已停用")
				}
				if seen[id] {
					return fmt.Errorf("菜单上级关系存在循环")
				}
				seen[id] = true
				if (!a.super && !a.menus[id]) || (!parentSuper && !parentMenus[id]) {
					return fmt.Errorf("不能授予超出自身或上级角色范围的菜单")
				}
				granted[id] = true
				id = row["pid"].Int64()
			}
		}
		for _, id := range buttonIDs {
			row, exists := byRule[id]
			if !exists || row["type"].Int() != 2 {
				return fmt.Errorf("所选按钮不存在或已停用")
			}
			if (!a.super && !a.buttons[id]) || (!parentSuper && !parentButtons[id]) {
				return fmt.Errorf("不能授予超出自身或上级角色范围的按钮")
			}
			granted[id] = true
		}
		ordered := []int64{}
		for id := range granted {
			ordered = append(ordered, id)
		}
		sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
		stringsIDs := []string{}
		for _, id := range ordered {
			stringsIDs = append(stringsIDs, strconv.FormatInt(id, 10))
		}
		menusJSON, _ := json.Marshal(menuIDs)
		buttonsJSON, _ := json.Marshal(buttonIDs)
		data := map[string]any{"pid": req.Pid, "name": name, "remark": req.Remark, "weigh": req.Weigh, "data_access": req.DataAccess, "menu": string(menusJSON), "btns": string(buttonsJSON), "rules": strings.Join(stringsIDs, ",")}
		if req.Id == 0 {
			data["account_id"], data["status"] = uid, 0
			savedID, err = roles.Clone().Data(data).InsertAndGetId()
		} else {
			savedID = req.Id
			_, err = roles.Clone().Where("id", req.Id).Data(data).Update()
		}
		if err == nil && len(security) > 0 {
			err = security[0].RolesChangedTx(ctx, tx.GetSqlTX(), []int64{savedID})
		}
		return err
	})
	return savedID, err
}

func hostRoleModels(ctx context.Context) (*gdb.Model, *gdb.Model, *gdb.Model) {
	return dao.AuthRole.Ctx(ctx), dao.AuthRule.Ctx(ctx), dao.AuthRoleAccess.Ctx(ctx)
}
