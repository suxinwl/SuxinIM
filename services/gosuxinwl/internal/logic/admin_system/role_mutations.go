package adminsystem

import (
	"context"
	"fmt"
	"github.com/suxinwl/GoSuxin/internal/accountsecurity"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
)

// Status changes and deletes use the same managed role tree as role saving.
// Deletion removes descendants and membership rows in one transaction.
func mutateRoleRecords(ctx context.Context, roles, access *gdb.Model, uid int64, ids []int64, remove bool, status uint, security ...*accountsecurity.Store) error {
	if len(ids) == 0 {
		return fmt.Errorf("请选择角色")
	}
	if !remove && status > 1 {
		return fmt.Errorf("角色状态无效")
	}
	return roles.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		roles, access := roles.Clone().TX(tx).Ctx(ctx), access.Clone().TX(tx).Ctx(ctx)
		a, err := readRoleAuthority(ctx, roles, access, uid)
		if err != nil {
			return err
		}
		rows, err := roles.Clone().All()
		if err != nil {
			return err
		}
		parents, byID := map[int64]int64{}, map[int64]gdb.Record{}
		for _, row := range rows {
			id := row["id"].Int64()
			parents[id], byID[id] = row["pid"].Int64(), row
		}
		selected := map[int64]bool{}
		for _, id := range ids {
			if id <= 0 || len(byID[id]) == 0 {
				return fmt.Errorf("所选角色不存在")
			}
			selected[id] = true
		}
		if remove {
			for id := range byID {
				if roleDescendant(id, parents, selected) {
					selected[id] = true
				}
			}
		}
		allIDs := []int64{}
		for id := range selected {
			if byID[id]["rules"].String() == "*" {
				return fmt.Errorf("不能停用或删除内置超级角色")
			}
			if !a.super && (a.assigned[id] || !roleDescendant(id, parents, a.assigned)) {
				return fmt.Errorf("只能管理所授权的下级角色")
			}
			allIDs = append(allIDs, id)
		}
		if len(security) > 0 {
			if err = security[0].RolesChangedTx(ctx, tx.GetSqlTX(), allIDs); err != nil {
				return err
			}
		}
		if remove {
			if _, err = access.Clone().WhereIn("role_id", allIDs).Delete(); err != nil {
				return err
			}
			_, err = roles.Clone().WhereIn("id", allIDs).Delete()
		} else {
			_, err = roles.Clone().WhereIn("id", allIDs).Data(map[string]any{"status": status}).Update()
		}
		return err
	})
}
