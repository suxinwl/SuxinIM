package wecom

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func TestSavingExclusionsArchivesExistingEmployeesAtomically(t *testing.T) {
	ctx := setupDB(t)
	c := &Client{}
	if err := c.LoadSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveSettings(ctx, Settings{IncludeSubdepartments: true}); err != nil {
		t.Fatal(err)
	}
	id := seedEmployee(t, ctx, "employee")
	seedRawUser(t, ctx, "employee")
	if _, err := core.DB().Exec(ctx, `CREATE TRIGGER reject_archive BEFORE UPDATE ON biz_hrm_employee WHEN NEW.status='filtered' BEGIN SELECT RAISE(ABORT,'forced failure'); END`); err != nil {
		t.Fatal(err)
	}
	candidate := Settings{ExcludeUsers: []string{"employee"}, IncludeSubdepartments: true}
	if err := c.SaveSettings(ctx, candidate); err == nil {
		t.Fatal("configuration succeeded after failed archive")
	}
	if len(c.Settings().ExcludeUsers) != 0 {
		t.Fatal("memory configuration escaped rollback")
	}
	exclusions, err := core.LoadExplicitExclusions(ctx)
	if err != nil || len(exclusions.Users) != 0 {
		t.Fatal("saved configuration escaped rollback", err)
	}
	row, _ := core.Model("biz_hrm_employee").Where("id", id).One()
	if row["status"].String() != "active" {
		t.Fatal("employee escaped rollback")
	}
	_, _ = core.DB().Exec(ctx, "DROP TRIGGER reject_archive")
	if err = c.SaveSettings(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	row, _ = core.Model("biz_hrm_employee").Where("id", id).One()
	if row["status"].String() != "filtered" || countRows(t, ctx, "wecom_user") != 1 {
		t.Fatal("save did not archive existing employee or deleted raw member")
	}
	if err = c.SaveSettings(ctx, candidate); err != nil {
		t.Fatal("repeated save failed", err)
	}
}

func TestContactMoveToExcludedDepartmentArchivesExistingEmployee(t *testing.T) {
	ctx := setupDB(t)
	moved := false
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/department/list":
			respond(w, g.Map{"errcode": 0, "department": []Department{{ID: 1, Name: "公司"}, {ID: 2, ParentID: 1, Name: "万佳里"}, {ID: 3, ParentID: 2, Name: "车间"}}})
		case "/user/list":
			users := []User{}
			dep := int64(1)
			if moved {
				dep = 3
			}
			if (dep == 1 && r.URL.Query().Get("department_id") == "1") || (dep == 3 && r.URL.Query().Get("department_id") == "3") {
				users = append(users, User{UserID: "known", Name: "张三", Department: []int64{dep}, Status: 1})
			}
			respond(w, g.Map{"errcode": 0, "userlist": users})
		default:
			t.Error("unexpected API", r.URL.Path)
		}
	})
	if err := c.LoadSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveSettings(ctx, Settings{ExcludeDepartmentNames: []string{"万佳里"}, IncludeSubdepartments: true}); err != nil {
		t.Fatal(err)
	}
	if n, err := c.SyncContact(ctx); err != nil || n != 1 {
		t.Fatal("initial import", n, err)
	}
	moved = true
	if n, err := c.SyncContact(ctx); err != nil || n != 0 {
		t.Fatal("moved import", n, err)
	}
	row, _ := core.Model("biz_hrm_employee").Where("wecom_userid", "known").One()
	if row["status"].String() != "filtered" || row["name"].String() != "张三" || countRows(t, ctx, "wecom_user") != 1 {
		t.Fatal("moved employee not archived without deleting history mapping")
	}
	if _, err := core.DB().Exec(ctx, "CREATE TABLE gf_auth_dept(id INTEGER PRIMARY KEY,name TEXT,pid INTEGER,status INTEGER,deletetime DATETIME)"); err != nil {
		t.Fatal(err)
	}
	mappings, err := departmentMappings(ctx)
	if err != nil || len(mappings) != 1 || mappings[0]["name"] != "公司" || countRows(t, ctx, "wecom_department") != 3 {
		t.Fatal("business mapping leaked excluded department or removed raw tree", mappings, err)
	}
	exclusions, err := core.LoadExplicitExclusions(ctx)
	if err != nil || !exclusions.Users["known"] || !exclusions.Departments[3] {
		t.Fatal("current exclusion scope lost", err)
	}
	// Moving back to an allowed department updates the raw snapshot but must
	// leave the archived employee and account mapping under administrator control.
	moved = false
	for i := 0; i < 2; i++ {
		n, err := c.SyncContact(ctx)
		var partial *PartialError
		if n != 0 || !errors.As(err, &partial) || partial.Failed != 1 {
			t.Fatal("archive was not reported for manual recovery", n, err)
		}
		row, _ = core.Model("biz_hrm_employee").Where("wecom_userid", "known").One()
		raw, _ := core.Model("wecom_user").Where("wecom_userid", "known").One()
		if row["status"].String() != "filtered" || row["user_id"].Int64() != 0 || !strings.Contains(raw["processing_error"].String(), "不会自动恢复") {
			t.Fatal("allowed department resurrected an archived employee")
		}
	}
}

func TestContactAfterRemovingExclusionDoesNotRestoreArchivedEmployee(t *testing.T) {
	ctx := setupDB(t)
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/department/list":
			respond(w, g.Map{"errcode": 0, "department": []Department{{ID: 1, Name: "公司"}}})
		case "/user/list":
			respond(w, g.Map{"errcode": 0, "userlist": []User{{UserID: "archived", Name: "王五", Department: []int64{1}, Status: 1}}})
		default:
			t.Error("unexpected API", r.URL.Path)
		}
	})
	if err := c.LoadSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if n, err := c.SyncContact(ctx); err != nil || n != 1 {
		t.Fatal("initial import", n, err)
	}
	if err := c.SaveSettings(ctx, Settings{ExcludeUserNames: []string{"王五"}, IncludeSubdepartments: true}); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveSettings(ctx, Settings{IncludeSubdepartments: true}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		n, err := c.SyncContact(ctx)
		var partial *PartialError
		if n != 0 || !errors.As(err, &partial) || partial.Failed != 1 {
			t.Fatal("removed rule restored an archived employee", n, err)
		}
		row, _ := core.Model("biz_hrm_employee").Where("wecom_userid", "archived").One()
		raw, _ := core.Model("wecom_user").Where("wecom_userid", "archived").One()
		if row["status"].String() != "filtered" || row["user_id"].Int64() != 0 || !strings.Contains(raw["processing_error"].String(), "不会自动恢复") {
			t.Fatal("archived identity changed or exception missing")
		}
	}
}
