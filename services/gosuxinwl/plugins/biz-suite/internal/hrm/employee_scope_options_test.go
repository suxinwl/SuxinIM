package hrm

import (
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func TestEmployeeScopeOptionsUseLocalPrimaryDepartment(t *testing.T) {
	ctx := setup(t)
	if _, err := core.HostModel("auth_dept").Ctx(ctx).Data(g.Map{"id": 11, "pid": 10, "name": "子部门"}).Insert(); err != nil {
		t.Fatal(err)
	}
	id, err := UpsertEmployee(ctx, Employee{ID: "scope-person", Name: "同名员工", EmployeeNo: "7319", DeptID: 11, WeComUserID: "external-user"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = core.Model("wecom_department").Data(g.Map{"id": "upstream-88", "wecom_id": 88, "parent_id": 0, "name": "企微兼职部门", "raw_json": "{}", "synced_at": core.Now()}).Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err = core.Model("wecom_user").Data(g.Map{"id": "upstream-user", "wecom_userid": "external-user", "employee_id": id, "name": "同名员工", "department_ids": "[88]", "raw_json": "{}", "synced_at": core.Now()}).Insert(); err != nil {
		t.Fatal(err)
	}
	_, err = UpsertEmployee(ctx, Employee{ID: "unassigned-person", Name: "无部门员工", EmployeeNo: "1600"})
	if err != nil {
		t.Fatal(err)
	}
	c := newAttWorkflowClient(t, ctx)
	c.base = strings.TrimSuffix(c.base, "attendance/v2/")
	data := c.ok("GET", "options", nil)
	found := false
	for _, item := range data["employees"].([]any) {
		row := item.(map[string]any)
		if row["id"] == id {
			found = true
			if row["dept_id"] != float64(11) || row["department"] != "Dev / 子部门" || row["employee_no"] != "7319" {
				t.Fatalf("scope options must use local primary department: %v", row)
			}
		}
		if row["id"] == "unassigned-person" && (row["dept_id"] != float64(0) || row["department"] != "") {
			t.Fatalf("unassigned department was guessed: %v", row)
		}
		if _, leaked := row["raw_json"]; leaked {
			t.Fatal("raw directory data must not be returned")
		}
	}
	if !found {
		t.Fatal("scope employee is missing")
	}
}
