package organization

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/migration"
)

func setup(t *testing.T) context.Context {
	t.Helper()
	ctx := context.Background()
	db, err := gdb.New(gdb.ConfigNode{Type: "sqlite", Name: filepath.Join(t.TempDir(), "organization.db"), Prefix: "gf_"})
	if err != nil {
		t.Fatal(err)
	}
	if err = core.Init(db); err != nil {
		t.Fatal(err)
	}
	business := core.DB()
	t.Cleanup(func() { business.Close(ctx); db.Close(ctx) })
	for _, sql := range []string{
		`CREATE TABLE gf_admin(id INTEGER PRIMARY KEY,account_id INTEGER,dept_id INTEGER,username VARCHAR(20),name TEXT,nickname TEXT,password TEXT,salt TEXT,status INTEGER,avatar TEXT,email TEXT,mobile TEXT,tel TEXT NOT NULL,remark TEXT NOT NULL,createtime DATETIME,updatetime DATETIME,deletetime DATETIME)`,
		`CREATE TABLE gf_auth_dept(id INTEGER PRIMARY KEY,business_id INTEGER,account_id INTEGER,name TEXT,pid INTEGER,weigh INTEGER,status INTEGER,remark TEXT NOT NULL,createtime DATETIME,updatetime DATETIME,deletetime DATETIME)`,
		`CREATE TABLE gf_auth_role(id INTEGER PRIMARY KEY,business_id INTEGER,account_id INTEGER,pid INTEGER,name TEXT,rules TEXT,menu TEXT,btns TEXT,status INTEGER,data_access INTEGER,remark TEXT,weigh INTEGER,createtime DATETIME)`,
		`CREATE TABLE gf_auth_role_access(uid INTEGER,role_id INTEGER)`,
		`CREATE TABLE gf_auth_rule(id INTEGER PRIMARY KEY,uid INTEGER,pid INTEGER,type INTEGER,title TEXT,des TEXT,locale TEXT,icon TEXT,routepath TEXT,component TEXT,redirect TEXT,routename TEXT,path TEXT,permission TEXT,status INTEGER,requiresauth INTEGER,hideinmenu INTEGER)`,
		`INSERT INTO gf_admin(id,username,name,password,salt,status,dept_id,tel,remark) VALUES(1,'admin','管理员','original-hash','original-salt',0,1,'','')`,
		`INSERT INTO gf_auth_dept(id,name,pid,status,remark,deletetime) VALUES(1,'旧部门',0,0,'','2026-01-01')`,
		`INSERT INTO gf_auth_role(id,name,rules,menu,btns,status) VALUES(1,'超级管理组','*','*','',0)`,
		`INSERT INTO gf_auth_role_access(uid,role_id) VALUES(1,1)`,
	} {
		if _, err = core.DB().Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if err = migration.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if err = core.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	if err = SeedRoles(ctx); err != nil {
		t.Fatal(err)
	}
	return ctx
}
func dep(t *testing.T, ctx context.Context, id, parent int64, name string) {
	t.Helper()
	_, err := core.Model("wecom_department").Data(g.Map{"id": fmt.Sprint(id), "wecom_id": id, "parent_id": parent, "name": name, "raw_json": "{}", "synced_at": core.Now()}).Insert()
	if err != nil {
		t.Fatal(err)
	}
}
func person(t *testing.T, ctx context.Context, uid, name, no string, depts []int64, primary int64) {
	t.Helper()
	raw := g.Map{"userid": uid, "name": name, "department": depts, "main_department": primary, "extattr": g.Map{"attrs": []g.Map{{"name": "工号", "type": 0, "text": g.Map{"value": no}}}}}
	_, err := core.Model("wecom_user").Data(g.Map{"id": uid, "wecom_userid": uid, "name": name, "department_ids": jsonText(depts), "status": 1, "raw_json": jsonText(raw), "synced_at": core.Now()}).Insert()
	if err != nil {
		t.Fatal(err)
	}
	_, err = core.Model("biz_hrm_employee").Data(g.Map{"id": "emp-" + uid, "wecom_userid": uid, "name": name, "created_at": core.Now(), "updated_at": core.Now()}).Insert()
	if err != nil {
		t.Fatal(err)
	}
}
func input(depts []int64, users ...string) Input {
	in := Input{map[int64]bool{}, map[string]bool{}}
	for _, id := range depts {
		in.AllowedDepartments[id] = true
	}
	for _, uid := range users {
		in.AllowedUsers[uid] = true
	}
	return in
}
func count(t *testing.T, table string) int {
	t.Helper()
	n, e := core.Model(table).Count()
	if e != nil {
		t.Fatal(e)
	}
	return n
}

func TestOrganizationCreatesDisabledAccountsAndPreservesAdmin(t *testing.T) {
	ctx := setup(t)
	dep(t, ctx, 10, 0, "集团")
	dep(t, ctx, 20, 10, "研发部")
	person(t, ctx, "hu", "测试员工甲", "07319", []int64{20}, 20)
	in := input([]int64{20}, "hu")
	r, err := Sync(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if r.DepartmentsCreated != 2 || r.AccountsCreated != 1 || r.EmployeesLinked != 1 || r.ConflictCount != 0 {
		t.Fatalf("unexpected report %+v", r)
	}
	a, _ := core.HostModel("admin").Where("username", "07319").One()
	if a["name"].String() != "测试员工甲" || a["status"].Int() != 1 || a["dept_id"].Int64() == 0 || len(a["password"].String()) != 32 {
		t.Fatal("disabled account mapping failed")
	}
	admin, _ := core.HostModel("admin").Where("id", 1).One()
	if admin["username"].String() != "admin" || admin["password"].String() != "original-hash" || admin["dept_id"].Int64() != 1 {
		t.Fatal("existing admin changed")
	}
	old, _ := core.HostModel("auth_dept").Where("id", 1).One()
	if old["deletetime"].String() == "" {
		t.Fatal("soft-deleted department resurrected")
	}
	r, err = Sync(ctx, in)
	if err != nil || r.DepartmentsCreated != 0 || r.AccountsCreated != 0 || count(t, "gf_auth_role_access") != 2 {
		t.Fatalf("not idempotent %+v %v", r, err)
	}
	// Administrator activates the account and assigns a chosen role. A later
	// directory update may change its display name, never its password/roles.
	_, _ = core.HostModel("admin").Where("id", a["id"]).Update(g.Map{"status": 0, "password": "admin-chosen-hash"})
	_, _ = core.Model("wecom_department").Where("wecom_id", 20).Update(g.Map{"name": "技术部"})
	_, _ = core.Model("wecom_user").Where("wecom_userid", "hu").Update(g.Map{"name": "测试员工甲新"})
	r, err = Sync(ctx, in)
	if err != nil || r.AccountsUpdated != 1 || r.DepartmentsUpdated != 1 {
		t.Fatalf("profile update failed %+v %v", r, err)
	}
	a, _ = core.HostModel("admin").Where("id", a["id"]).One()
	if a["status"].Int() != 0 || a["password"].String() != "admin-chosen-hash" {
		t.Fatal("activation/password reset by sync")
	}
	_, _ = core.Model("wecom_user").Where("wecom_userid", "hu").Update(g.Map{"status": 5})
	if _, err = Sync(ctx, in); err != nil {
		t.Fatal(err)
	}
	a, _ = core.HostModel("admin").Where("id", a["id"]).One()
	if a["status"].Int() != 1 {
		t.Fatal("confirmed resigned owned account remained active")
	}
}

func TestOrganizationConflictsAndFiltersNeverGuessAccounts(t *testing.T) {
	ctx := setup(t)
	dep(t, ctx, 10, 0, "集团")
	dep(t, ctx, 20, 10, "研发部")
	dep(t, ctx, 30, 10, "分子公司")
	person(t, ctx, "duplicate1", "甲", "7319", []int64{20}, 20)
	person(t, ctx, "duplicate2", "乙", "7319", []int64{30}, 30)
	person(t, ctx, "missing", "丙", "", []int64{20}, 20)
	person(t, ctx, "ambiguous", "丁", "1600", []int64{20, 30}, 0)
	person(t, ctx, "collision", "戊", "admin", []int64{20}, 20)
	person(t, ctx, "excluded", "己", "1601", []int64{30}, 30)
	r, err := Sync(ctx, input([]int64{20}, "duplicate1", "missing", "ambiguous", "collision"))
	if err != nil {
		t.Fatal(err)
	}
	if r.AccountsCreated != 0 || r.ConflictCount != 4 || r.Skipped != 2 || count(t, "gf_admin") != 1 {
		t.Fatalf("unsafe matching %+v", r)
	}
	if count(t, "gf_auth_dept") != 3 {
		t.Fatal("excluded subtree imported")
	}
	if !strings.Contains(r.Conflicts[0].Reason, "主部门") && !strings.Contains(jsonText(r), "重复工号") {
		t.Fatal("missing useful errors")
	}
}

func TestOrganizationRollbackAndConcurrentIdempotency(t *testing.T) {
	ctx := setup(t)
	dep(t, ctx, 10, 0, "集团")
	person(t, ctx, "one", "甲", "0001", []int64{10}, 10)
	in := input([]int64{10}, "one")
	_, err := core.DB().Exec(ctx, `CREATE TRIGGER reject_binding BEFORE INSERT ON biz_org_binding WHEN NEW.entity_type='account' BEGIN SELECT RAISE(ABORT,'forced failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Sync(ctx, in); err == nil {
		t.Fatal("expected transaction failure")
	}
	if count(t, "gf_admin") != 1 || count(t, "gf_auth_dept") != 1 || count(t, "biz_org_binding") != 0 {
		t.Fatal("failed transaction left partial identities")
	}
	_, _ = core.DB().Exec(ctx, `DROP TRIGGER reject_binding`)
	var wg sync.WaitGroup
	errors := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := Sync(ctx, in); errors <- e }()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if count(t, "gf_admin") != 2 || count(t, "biz_org_binding") != 2 || count(t, "gf_auth_role_access") != 2 {
		t.Fatal("concurrent sync duplicated rows")
	}
}

func TestRoleSeedsPreserveEditsAndStopElevatedDefaults(t *testing.T) {
	ctx := setup(t)
	if count(t, "gf_auth_role") != 5 {
		t.Fatal("four staff roles not seeded")
	}
	employee, _ := core.Model("biz_org_role").Where("role_key", "employee").Value("role_id")
	_, _ = core.HostModel("auth_role").Where("id", employee).Update(g.Map{"name": "自定义员工", "status": 1})
	if err := SeedRoles(ctx); err != nil {
		t.Fatal(err)
	}
	r, _ := core.HostModel("auth_role").Where("id", employee).One()
	if r["name"].String() != "自定义员工" || r["status"].Int() != 1 || count(t, "gf_auth_role") != 5 {
		t.Fatal("seed overwrote administrator choices")
	}
	dep(t, ctx, 10, 0, "集团")
	person(t, ctx, "one", "甲", "0001", []int64{10}, 10)
	report, err := Sync(ctx, input([]int64{10}, "one"))
	if err != nil || report.AccountsCreated != 0 || report.ConflictCount != 1 {
		t.Fatal("assigned disabled default role", err)
	}
	_, _ = core.HostModel("auth_role").Where("id", employee).Update(g.Map{"rules": "*", "status": 0})
	report, err = Sync(ctx, input([]int64{10}, "one"))
	if err != nil || report.AccountsCreated != 0 {
		t.Fatal("assigned super default role", err)
	}
}

func TestRoleSeedsDoNotAdoptSameNameExistingRole(t *testing.T) {
	ctx := setup(t)
	previous, _ := core.Model("biz_org_role").Where("role_key", "employee").Value("role_id")
	_, _ = core.Model("biz_org_role").Where("role_key", "employee").Delete()
	_, _ = core.HostModel("auth_role").Where("id", previous).Update(g.Map{"rules": "*"})
	if err := SeedRoles(ctx); err != nil {
		t.Fatal(err)
	}
	current, _ := core.Model("biz_org_role").Where("role_key", "employee").Value("role_id")
	if current.Int64() == previous.Int64() {
		t.Fatal("same-name privileged role adopted")
	}
	role, _ := core.HostModel("auth_role").Where("id", current).One()
	if role["rules"].String() == "*" || role["name"].String() != "员工（企业微信1）" {
		t.Fatal("safe distinct role not created")
	}
}

func TestDepartmentPersistenceFailureRollsBack(t *testing.T) {
	ctx := setup(t)
	dep(t, ctx, 10, 0, "集团")
	_, err := core.DB().Exec(ctx, `CREATE TRIGGER reject_department_binding BEFORE INSERT ON biz_org_binding BEGIN SELECT RAISE(ABORT,'forced failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Sync(ctx, input([]int64{10})); err == nil {
		t.Fatal("department database error treated as a business conflict")
	}
	if count(t, "gf_auth_dept") != 1 || count(t, "biz_org_binding") != 0 {
		t.Fatal("failed department insert left an orphan")
	}
}

func TestManualDepartmentRemappingCannotCreateHostCycle(t *testing.T) {
	ctx := setup(t)
	dep(t, ctx, 10, 0, "集团")
	dep(t, ctx, 20, 10, "研发部")
	in := input([]int64{10, 20})
	if _, err := Sync(ctx, in); err != nil {
		t.Fatal(err)
	}
	child, _ := core.Model("wecom_department").Where("wecom_id", 20).Value("host_dept_id")
	root, _ := core.Model("wecom_department").Where("wecom_id", 10).Value("host_dept_id")
	_, _ = core.Model("wecom_department").Where("wecom_id", 10).Update(g.Map{"host_dept_id": child.Int64()})
	report, err := Sync(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if report.ConflictCount == 0 || !strings.Contains(jsonText(report), "循环") {
		t.Fatal("unsafe host hierarchy accepted", report)
	}
	p, _ := core.HostModel("auth_dept").Where("id", child).Value("pid")
	if p.Int64() != root.Int64() {
		t.Fatal("host department was reparented into a cycle")
	}
}
