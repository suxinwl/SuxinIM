package organization

import (
	"context"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func TestPruneExcludedSubtreeRetainsHistoryAfterManualDepartmentDelete(t *testing.T) {
	ctx := setup(t)
	dep(t, ctx, 10, 0, "集团")
	dep(t, ctx, 20, 10, "万佳里")
	dep(t, ctx, 30, 20, "一车间")
	person(t, ctx, "parent-user", "甲", "1001", []int64{20}, 20)
	person(t, ctx, "child-user", "乙", "1002", []int64{30}, 30)
	in := input([]int64{20, 30}, "parent-user", "child-user")
	if _, err := Sync(ctx, in); err != nil {
		t.Fatal(err)
	}
	departmentID, _ := core.Model("wecom_department").Where("wecom_id", 20).Value("host_dept_id")
	if _, err := core.HostModel("auth_dept").Where("id", departmentID).Update(g.Map{"status": 1, "deletetime": core.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Model("biz_hrm_attendance_clock").Data(g.Map{"id": "history", "employee_id": "emp-parent-user", "clock_time": "2026-10-29 08:00:00", "clock_type": "in", "status": "normal", "source": "wecom", "raw_json": "{}", "created_at": core.Now(), "updated_at": core.Now()}).Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Model("wecom_settings").Data(g.Map{"id": 1, "settings_json": `{"exclude_department_names":["万佳里"],"include_subdepartments":true}`, "updated_at": core.Now()}).Insert(); err != nil {
		t.Fatal(err)
	}
	excluded, err := core.LoadExplicitExclusions(ctx)
	if err != nil || len(excluded.Users) != 2 || len(excluded.Departments) != 2 {
		t.Fatalf("subtree not resolved: %+v %v", excluded, err)
	}
	preview, err := Prune(ctx, excluded, true)
	if err != nil || preview.EmployeesFiltered != 2 || preview.AccountsRemoved != 2 || preview.DepartmentsRemoved != 1 {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	active, _ := core.Model("biz_hrm_employee").Where("status", "active").Count()
	if active != 2 {
		t.Fatal("preview changed employees")
	}
	roles := count(t, "gf_auth_role_access")
	report, err := Prune(ctx, excluded, false)
	if err != nil || report.EmployeesFiltered != 2 || report.AccountsRemoved != 2 || report.DepartmentsRemoved != 1 || len(report.Conflicts) != 0 {
		t.Fatalf("apply: %+v %v", report, err)
	}
	for _, table := range []string{"wecom_user", "biz_hrm_employee"} {
		if count(t, table) != 2 {
			t.Fatal("raw/mapping rows deleted", table)
		}
	}
	if count(t, "biz_hrm_attendance_clock") != 1 || count(t, "gf_auth_role_access") != roles {
		t.Fatal("history or roles deleted")
	}
	accounts, _ := core.HostModel("admin").Unscoped().Where("id > ?", 1).All()
	for _, row := range accounts {
		if row["deletetime"].IsEmpty() || row["status"].Int() != 1 {
			t.Fatal("imported account still available")
		}
	}
	again, err := Prune(ctx, excluded, false)
	if err != nil || again.EmployeesFiltered != 0 || again.AlreadyFiltered != 2 || again.AccountsRemoved != 0 || again.DepartmentsRemoved != 0 {
		t.Fatalf("repeat not idempotent: %+v %v", again, err)
	}
	if _, err = Sync(ctx, in); err != nil {
		t.Fatal(err)
	}
	if n, _ := core.HostModel("admin").Unscoped().WhereNull("deletetime").Count(); n != 1 {
		t.Fatal("organization sync rebuilt excluded accounts")
	}
	// Removing a rule is not permission to reactivate an archived account.
	_, _ = core.Model("wecom_settings").Where("id", 1).Update(g.Map{"settings_json": "{}"})
	if _, err = Sync(ctx, in); err != nil {
		t.Fatal(err)
	}
	if n, _ := core.HostModel("admin").Unscoped().WhereNull("deletetime").Count(); n != 1 {
		t.Fatal("rule removal resurrected accounts")
	}
}

func TestPruneProtectsManualElevatedAndCurrentAccountsAndManualChildren(t *testing.T) {
	ctx := setup(t)
	dep(t, ctx, 10, 0, "集团")
	dep(t, ctx, 20, 10, "万佳里")
	for _, uid := range []string{"manual", "elevated", "current"} {
		person(t, ctx, uid, uid, uid, []int64{20}, 20)
	}
	if _, err := Sync(ctx, input([]int64{20}, "manual", "elevated", "current")); err != nil {
		t.Fatal(err)
	}
	_, _ = core.Model("biz_org_binding").Where("entity_type", "account").Where("external_id", "manual").Delete()
	elevated, _ := core.Model("biz_hrm_employee").Where("wecom_userid", "elevated").Value("user_id")
	_, _ = core.HostModel("auth_role_access").Data(g.Map{"uid": elevated, "role_id": 1}).Insert()
	current, _ := core.Model("biz_hrm_employee").Where("wecom_userid", "current").Value("user_id")
	ctx = context.WithValue(ctx, "uid", current.Int64())
	parent, _ := core.Model("wecom_department").Where("wecom_id", 20).Value("host_dept_id")
	_, _ = core.HostModel("auth_dept").Data(g.Map{"id": 99, "name": "人工部门", "pid": parent, "status": 0, "remark": ""}).Insert()
	excluded, err := core.ExplicitExclusionsForRules(ctx, core.ExclusionRules{ExcludeDepartmentNames: []string{"万佳里"}, IncludeSubdepartments: true})
	if err != nil {
		t.Fatal(err)
	}
	report, err := Prune(ctx, excluded, false)
	if err != nil || report.AccountsRemoved != 0 || report.DepartmentsRemoved != 0 || len(report.Conflicts) < 4 {
		t.Fatalf("protected identity changed: %+v %v", report, err)
	}
	if n, _ := core.HostModel("admin").Unscoped().WhereNotNull("deletetime").Count(); n != 0 {
		t.Fatal("protected account deleted")
	}
	parentRow, _ := core.HostModel("auth_dept").Unscoped().Where("id", parent).One()
	if !parentRow["deletetime"].IsEmpty() {
		t.Fatal("parent of manual children hidden")
	}
}

func TestPruneFailureRollsBackEmployeeAndHostTogether(t *testing.T) {
	ctx := setup(t)
	dep(t, ctx, 10, 0, "集团")
	person(t, ctx, "user", "员工", "1001", []int64{10}, 10)
	if _, err := Sync(ctx, input([]int64{10}, "user")); err != nil {
		t.Fatal(err)
	}
	if _, err := core.DB().Exec(ctx, `CREATE TRIGGER reject_account_archive BEFORE UPDATE ON gf_admin WHEN NEW.deletetime IS NOT NULL BEGIN SELECT RAISE(ABORT,'forced failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := Prune(ctx, &core.ExplicitExclusions{Departments: map[int64]bool{}, Users: map[string]bool{"user": true}}, false)
	if err == nil {
		t.Fatal("forced database failure accepted")
	}
	status, _ := core.Model("biz_hrm_employee").Where("wecom_userid", "user").Value("status")
	if status.String() != "active" {
		t.Fatal("employee change escaped failed transaction")
	}
}

func TestExplicitExclusionsDoNotInferRemovalFromMissingOrRestrictedDirectory(t *testing.T) {
	ctx := setup(t)
	dep(t, ctx, 10, 0, "集团")
	person(t, ctx, "unknown", "员工", "1001", []int64{999}, 999)
	person(t, ctx, "missing", "待核对", "1002", nil, 0)
	_, _ = core.Model("wecom_user").Where("wecom_userid", "unknown").Update(g.Map{"processing_error": "最近一次通讯录同步未观察到该成员"})
	excluded, err := core.ExplicitExclusionsForRules(ctx, core.ExclusionRules{ExcludeDepartmentNames: []string{"万佳里"}, ExcludeUserNames: []string{"员工甲"}, IncludeSubdepartments: true})
	if err != nil || len(excluded.Users) != 0 || len(excluded.Departments) != 0 {
		t.Fatalf("unknown data treated as excluded: %+v %v", excluded, err)
	}
	report, err := Prune(ctx, excluded, false)
	if err != nil || report.EmployeesFiltered != 0 {
		t.Fatal("unknown member removed", report, err)
	}
}
