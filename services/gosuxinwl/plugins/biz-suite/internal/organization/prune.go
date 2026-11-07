package organization

import (
	"context"
	"sort"
	"strconv"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

type PruneReport struct {
	EmployeesFiltered  int        `json:"employees_filtered"`
	AccountsRemoved    int        `json:"accounts_removed"`
	DepartmentsRemoved int        `json:"departments_removed"`
	AlreadyFiltered    int        `json:"already_filtered"`
	Conflicts          []Conflict `json:"conflicts"`
	Preview            bool       `json:"preview"`
}

// WithTransaction serializes configuration changes with organization mapping.
// The callback must not make upstream requests or call another locked operation.
func WithTransaction(ctx context.Context, work func(context.Context, gdb.TX) error) error {
	return locked(ctx, work)
}

func Prune(ctx context.Context, exclusions *core.ExplicitExclusions, preview bool) (PruneReport, error) {
	var report PruneReport
	err := locked(ctx, func(ctx context.Context, tx gdb.TX) error {
		var err error
		report, err = PruneInTransaction(ctx, tx, exclusions, preview)
		return err
	})
	return report, err
}

// PruneInTransaction archives only explicitly excluded imported identities.
// Raw directory rows, mappings, roles and all attendance/message history remain.
// Deleted host identities are never automatically restored by this operation.
func PruneInTransaction(ctx context.Context, tx gdb.TX, exclusions *core.ExplicitExclusions, preview bool) (PruneReport, error) {
	report := PruneReport{Conflicts: []Conflict{}, Preview: preview}
	if exclusions == nil || (len(exclusions.Users) == 0 && len(exclusions.Departments) == 0) {
		return report, nil
	}
	conflict := func(kind, name, no, reason string) {
		report.Conflicts = append(report.Conflicts, Conflict{EntityType: kind, Name: name, EmployeeNo: no, Reason: reason})
	}
	bindings, err := model(ctx, tx, "biz_org_binding").All()
	if err != nil {
		return report, err
	}
	accountBindings, departmentBindings := map[string]int64{}, map[int64]int64{}
	for _, row := range bindings {
		switch row["entity_type"].String() {
		case "account":
			accountBindings[row["external_id"].String()] = row["host_id"].Int64()
		case "department":
			id, _ := strconv.ParseInt(row["external_id"].String(), 10, 64)
			departmentBindings[id] = row["host_id"].Int64()
		}
	}
	employees, err := model(ctx, tx, "biz_hrm_employee").OrderAsc("id").All()
	if err != nil {
		return report, err
	}
	accountCandidates := map[int64]gdb.Record{}
	for _, row := range employees {
		uid := row["wecom_userid"].String()
		if !exclusions.Users[uid] {
			continue
		}
		if row["status"].String() == "filtered" {
			report.AlreadyFiltered++
		} else {
			report.EmployeesFiltered++
			if !preview {
				if _, err = model(ctx, tx, "biz_hrm_employee").Where("id", row["id"]).Update(g.Map{"status": "filtered", "updated_at": core.Now()}); err != nil {
					return report, err
				}
			}
		}
		id := row["user_id"].Int64()
		if id == 0 {
			continue
		}
		if id == 1 || id == core.UID(ctx) || accountBindings[uid] != id {
			conflict("account", row["name"].String(), row["employee_no"].String(), "人工账号、管理员或当前登录账号受保护，仅移除同步员工档案")
			continue
		}
		accountCandidates[id] = row
	}
	protected := map[int64]bool{}
	if len(accountCandidates) > 0 {
		roles, e := host(ctx, tx, "auth_role").Unscoped().Where("rules", "*").Fields("id").All()
		if e != nil {
			return report, e
		}
		roleIDs := []int64{}
		for _, row := range roles {
			roleIDs = append(roleIDs, row["id"].Int64())
		}
		if len(roleIDs) > 0 {
			access, e := host(ctx, tx, "auth_role_access").WhereIn("role_id", roleIDs).Fields("uid").All()
			if e != nil {
				return report, e
			}
			for _, row := range access {
				protected[row["uid"].Int64()] = true
			}
		}
	}
	removedAccounts := map[int64]bool{}
	accountIDs := []int64{}
	for id := range accountCandidates {
		accountIDs = append(accountIDs, id)
	}
	sort.Slice(accountIDs, func(i, j int) bool { return accountIDs[i] < accountIDs[j] })
	for _, id := range accountIDs {
		employee := accountCandidates[id]
		if protected[id] {
			conflict("account", employee["name"].String(), employee["employee_no"].String(), "账号具有超级管理员角色，已保护，不自动删除")
			continue
		}
		row, e := host(ctx, tx, "admin").Unscoped().Where("id", id).One()
		if e != nil {
			return report, e
		}
		if len(row) == 0 || !row["deletetime"].IsEmpty() {
			removedAccounts[id] = true
			continue
		}
		if !preview {
			if _, err = host(ctx, tx, "admin").Unscoped().Where("id", id).Update(g.Map{"status": 1, "deletetime": core.Now(), "updatetime": core.Now()}); err != nil {
				return report, err
			}
		}
		report.AccountsRemoved++
		removedAccounts[id] = true
	}
	deps, err := model(ctx, tx, "wecom_department").Fields("wecom_id,host_dept_id,name").All()
	if err != nil {
		return report, err
	}
	departmentCandidates := map[int64]string{}
	for _, row := range deps {
		wid, id := row["wecom_id"].Int64(), row["host_dept_id"].Int64()
		if !exclusions.Departments[wid] || id == 0 {
			continue
		}
		if departmentBindings[wid] != id {
			conflict("department", row["name"].String(), "", "人工绑定部门受保护，不自动删除")
			continue
		}
		departmentCandidates[id] = row["name"].String()
	}
	if len(departmentCandidates) == 0 {
		return report, nil
	}
	hostDepartments, err := host(ctx, tx, "auth_dept").Unscoped().Fields("id,pid,name,deletetime").All()
	if err != nil {
		return report, err
	}
	parents, activeDepartments := map[int64]int64{}, map[int64]bool{}
	for _, row := range hostDepartments {
		id := row["id"].Int64()
		parents[id] = row["pid"].Int64()
		activeDepartments[id] = row["deletetime"].IsEmpty()
	}
	// An imported parent may contain manually added children or accounts. Keep
	// their complete ancestor path instead of making unrelated objects disappear.
	blocked := map[int64]bool{}
	blockAncestors := func(id int64) {
		for seen := map[int64]bool{}; id > 0 && !seen[id]; id = parents[id] {
			seen[id] = true
			if _, ok := departmentCandidates[id]; ok {
				blocked[id] = true
			}
		}
	}
	for id, active := range activeDepartments {
		if _, candidate := departmentCandidates[id]; active && !candidate {
			blockAncestors(parents[id])
		}
	}
	accounts, err := host(ctx, tx, "admin").Unscoped().WhereNull("deletetime").Fields("id,dept_id").All()
	if err != nil {
		return report, err
	}
	for _, row := range accounts {
		if !removedAccounts[row["id"].Int64()] {
			blockAncestors(row["dept_id"].Int64())
		}
	}
	for _, row := range employees {
		if !exclusions.Users[row["wecom_userid"].String()] {
			blockAncestors(row["dept_id"].Int64())
		}
	}
	ids := []int64{}
	for id := range departmentCandidates {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		if !activeDepartments[id] {
			continue
		}
		if blocked[id] {
			conflict("department", departmentCandidates[id], "", "部门仍包含未排除的员工、人工账号或子部门，已保护")
			continue
		}
		if !preview {
			if _, err = host(ctx, tx, "auth_dept").Unscoped().Where("id", id).Update(g.Map{"status": 1, "deletetime": core.Now(), "updatetime": core.Now()}); err != nil {
				return report, err
			}
		}
		report.DepartmentsRemoved++
	}
	return report, nil
}
