package imbridge

import "testing"

func TestDepartmentDirectoryDoesNotAutomaticallyCreateGroups(t *testing.T) {
	ctx, store, remote := fixture(t)
	t.Setenv("GOSUXIN_IM_AUTO_DEPARTMENT_GROUPS", "false")
	provision(t, ctx, store, remote)
	if len(remote.departments) != 0 {
		t.Fatal("department groups were dispatched without opt-in")
	}
	departments, err := store.directoryDepartments(ctx)
	if err != nil || len(departments) != 2 {
		t.Fatal("HRM department directory was lost", err)
	}
	var count int
	if err = store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM biz_openim_outbox WHERE kind='department' AND status IN ('pending','failed')`).Scan(&count); err != nil || count != 0 {
		t.Fatal("automatic group task survived disabling", err)
	}
}
