package adminsystem

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/api/admin/system"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

func roleTestDB(t *testing.T) (context.Context, *gdb.Model, *gdb.Model, *gdb.Model) {
	t.Helper()
	ctx := context.Background()
	db, err := gdb.New(gdb.ConfigNode{Type: "sqlite", Name: filepath.Join(t.TempDir(), "role-crud.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(ctx) })
	for _, statement := range []string{
		`CREATE TABLE roles(id INTEGER PRIMARY KEY AUTOINCREMENT,business_id INTEGER NOT NULL DEFAULT 0,account_id INTEGER NOT NULL DEFAULT 0,pid INTEGER NOT NULL DEFAULT 0,name TEXT NOT NULL,rules TEXT NOT NULL,menu TEXT NOT NULL,btns TEXT NOT NULL,status INTEGER NOT NULL DEFAULT 0,data_access INTEGER NOT NULL DEFAULT 0,remark TEXT NOT NULL,weigh INTEGER NOT NULL)`,
		`CREATE TABLE rules(id INTEGER PRIMARY KEY,pid INTEGER,type INTEGER,status INTEGER,title TEXT DEFAULT '菜单',des TEXT DEFAULT '',locale TEXT DEFAULT '',weigh INTEGER DEFAULT 0)`,
		`CREATE TABLE access(uid INTEGER,role_id INTEGER)`,
		`INSERT INTO roles VALUES(1,0,0,0,'超级管理组','*','*','',0,2,'内置角色',1),(2,0,1,1,'管理者','10,20,30,31','[30]','31',0,0,'',2),(3,0,2,2,'下级角色','','[]','[]',0,0,'',3),(4,0,1,1,'无关角色','','[]','[]',0,0,'',4),(5,0,1,1,'禁用角色','*','*','',1,2,'',5)`,
		`INSERT INTO rules(id,pid,type,status) VALUES(10,0,0,0),(20,10,0,0),(30,20,1,0),(31,30,2,0),(40,0,1,0),(41,40,2,0),(42,40,2,1)`,
		`INSERT INTO access VALUES(1,1),(2,2),(3,5)`,
	} {
		if _, err = db.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	return ctx, db.Model("roles").Safe(), db.Model("rules").Safe(), db.Model("access").Safe()
}

func TestRoleSaveEmptySelectionAndTypedPermissions(t *testing.T) {
	ctx, roles, rules, access := roleTestDB(t)
	blank := &system.RoleSaveReq{Pid: 1, Name: " 职员 ", AccountId: 999, Weigh: 6}
	id, err := saveRoleRecord(ctx, roles, rules, access, 1, blank)
	if err != nil {
		t.Fatal(err)
	}
	row, err := roles.Clone().Where("id", id).One()
	if err != nil || row["name"].String() != "职员" || row["rules"].String() != "" || row["menu"].String() != "[]" || row["btns"].String() != "[]" || row["account_id"].Int64() != 1 {
		t.Fatalf("empty-role insert: %v %v", row, err)
	}
	if _, err = roles.Clone().Where("id", id).Data(map[string]any{"business_id": 77, "status": 1}).Update(); err != nil {
		t.Fatal(err)
	}
	edit := &system.RoleSaveReq{Id: id, Pid: 1, Name: "员工", Menu: []int64{30, 30}, Bbtns: []int{31, 31}, AccountId: 999, Rules: "*", Weigh: 4}
	if _, err = saveRoleRecord(ctx, roles, rules, access, 1, edit); err != nil {
		t.Fatal(err)
	}
	row, _ = roles.Clone().Where("id", id).One()
	if row["rules"].String() != "10,20,30,31" || row["menu"].String() != "[30]" || row["btns"].String() != "[31]" || row["account_id"].Int64() != 1 || row["business_id"].Int() != 77 || row["status"].Int() != 1 {
		t.Fatalf("role save lost ancestors, grant serialization or protected columns: %v", row)
	}
	// A delegated administrator can create a role only inside the same grant tree.
	if _, err = saveRoleRecord(ctx, roles, rules, access, 2, &system.RoleSaveReq{Pid: 2, Name: "受限职员", Menu: []any{float64(30)}, Bbtns: "[31]"}); err != nil {
		t.Fatal(err)
	}
}

func TestRoleSaveRejectsMalformedInputAndPrivilegeEscalation(t *testing.T) {
	ctx, roles, rules, access := roleTestDB(t)
	for _, tt := range []struct {
		name string
		uid  int64
		req  system.RoleSaveReq
	}{
		{"missing identity", 0, system.RoleSaveReq{Pid: 1, Name: "职员"}},
		{"disabled role", 3, system.RoleSaveReq{Pid: 1, Name: "职员"}},
		{"wildcard injection", 1, system.RoleSaveReq{Pid: 1, Name: "职员", Menu: "*"}},
		{"map instead of array", 1, system.RoleSaveReq{Pid: 1, Name: "职员", Menu: map[string]int{"id": 30}}},
		{"invalid button array", 1, system.RoleSaveReq{Pid: 1, Name: "职员", Menu: []int{30}, Bbtns: "31"}},
		{"fractional id", 1, system.RoleSaveReq{Pid: 1, Name: "职员", Menu: []float64{30.5}}},
		{"unknown id", 1, system.RoleSaveReq{Pid: 1, Name: "职员", Menu: []int{999}}},
		{"disabled grant", 1, system.RoleSaveReq{Pid: 1, Name: "职员", Bbtns: []int{42}}},
		{"builtin super", 1, system.RoleSaveReq{Id: 1, Name: "职员"}},
		{"change own role", 2, system.RoleSaveReq{Id: 2, Pid: 1, Name: "职员"}},
		{"change unrelated role", 2, system.RoleSaveReq{Id: 4, Pid: 2, Name: "职员"}},
		{"parent cycle", 1, system.RoleSaveReq{Id: 2, Pid: 3, Name: "职员"}},
		{"unknown parent", 1, system.RoleSaveReq{Pid: 99, Name: "职员"}},
		{"disabled parent", 1, system.RoleSaveReq{Pid: 5, Name: "职员"}},
		{"top level escape", 2, system.RoleSaveReq{Name: "职员"}},
		{"menu escalation", 2, system.RoleSaveReq{Pid: 2, Name: "职员", Menu: []int{40}}},
		{"button escalation", 2, system.RoleSaveReq{Pid: 2, Name: "职员", Bbtns: []int{41}}},
		{"parent grant escalation", 1, system.RoleSaveReq{Pid: 2, Name: "职员", Menu: []int{40}}},
		{"data access escalation", 2, system.RoleSaveReq{Pid: 2, Name: "职员", DataAccess: 2}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before, _ := roles.Clone().OrderAsc("id").All()
			if _, err := saveRoleRecord(ctx, roles, rules, access, tt.uid, &tt.req); err == nil {
				t.Fatal("invalid or unauthorized role save accepted")
			}
			after, _ := roles.Clone().OrderAsc("id").All()
			if !reflect.DeepEqual(before.List(), after.List()) {
				t.Fatal("failed save changed existing roles")
			}
		})
	}
}

func TestRoleMutationsProtectSuperAndDeleteFullSubtreeMemberships(t *testing.T) {
	ctx, roles, _, access := roleTestDB(t)
	for _, test := range []struct {
		uid, id int64
		remove  bool
	}{{1, 1, true}, {1, 1, false}, {2, 2, true}, {2, 4, false}, {3, 3, true}} {
		if err := mutateRoleRecords(ctx, roles, access, test.uid, []int64{test.id}, test.remove, 1); err == nil {
			t.Fatal("unauthorized role mutation accepted")
		}
	}
	if err := mutateRoleRecords(ctx, roles, access, 2, []int64{3}, false, 1); err != nil {
		t.Fatal(err)
	}
	if status, _ := roles.Clone().Where("id", 3).Value("status"); status.Int() != 1 {
		t.Fatal("authorized child status was not updated")
	}
	if _, err := roles.Clone().Data(map[string]any{"id": 6, "pid": 3, "name": "更下级角色", "rules": "", "menu": "[]", "btns": "[]", "remark": "", "weigh": 6}).Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err := access.Clone().Data([]map[string]any{{"uid": 9, "role_id": 3}, {"uid": 9, "role_id": 6}, {"uid": 9, "role_id": 4}}).Insert(); err != nil {
		t.Fatal(err)
	}
	if err := mutateRoleRecords(ctx, roles, access, 2, []int64{3}, true, 0); err != nil {
		t.Fatal(err)
	}
	if count, _ := roles.Clone().WhereIn("id", []int64{3, 6}).Count(); count != 0 {
		t.Fatal("deleted role left descendants behind")
	}
	if count, _ := access.Clone().WhereIn("role_id", []int64{3, 6}).Count(); count != 0 {
		t.Fatal("deleted role left membership grants behind")
	}
	if count, _ := access.Clone().Where("uid", 9).Where("role_id", 4).Count(); count != 1 {
		t.Fatal("unrelated role membership was deleted")
	}
}

func TestRoleMenuChoicesSeparateMenuAndButtonGrants(t *testing.T) {
	ctx, roles, rules, access := roleTestDB(t)
	// New templates deliberately store menu visibility and API buttons separately.
	if _, err := roles.Clone().Where("id", 2).Data(map[string]any{"rules": "[10,20,30]", "btns": "[31]"}).Update(); err != nil {
		t.Fatal(err)
	}
	for _, pid := range []int64{0, 2} {
		result := roleMenuChoices(ctx, roles, rules, access, 2, pid)
		if !result.Ok() {
			t.Fatal(result.Message)
		}
		data := result.Data.(gf.Map)
		if !reflect.DeepEqual(data["btn_rule_ids"], []int64{31}) {
			t.Fatalf("parent %d lost separate button permission: %v", pid, data["btn_rule_ids"])
		}
		encoded, _ := json.Marshal(data["list"])
		if !strings.Contains(string(encoded), `"btnids":[31]`) || strings.Contains(string(encoded), `"id":41`) || strings.Contains(string(encoded), `"id":42`) {
			t.Fatalf("parent checkbox contains out-of-scope buttons: %s", encoded)
		}
	}
	if result := roleMenuChoices(ctx, roles, rules, access, 2, 4); result.Ok() {
		t.Fatal("parent role outside delegation scope accepted")
	}
	if result := roleMenuChoices(ctx, roles, rules, access, 3, 1); result.Ok() {
		t.Fatal("disabled role can enumerate assignable permissions")
	}
}

func TestJSONTemplateParentSupportsChildCreateAndEdit(t *testing.T) {
	ctx, roles, rules, access := roleTestDB(t)
	if _, err := roles.Clone().Where("id", 2).Data(map[string]any{"rules": "[10,20,30]", "menu": "[30]", "btns": "[31]"}).Update(); err != nil {
		t.Fatal(err)
	}
	request := &system.RoleSaveReq{Pid: 2, Name: "模板下级员工", Menu: []int{30}, Bbtns: []int{31}}
	id, err := saveRoleRecord(ctx, roles, rules, access, 2, request)
	if err != nil {
		t.Fatalf("JSON template lost parent or operator permissions: %v", err)
	}
	request.Id, request.Name = id, "模板下级职员"
	if _, err = saveRoleRecord(ctx, roles, rules, access, 2, request); err != nil {
		t.Fatalf("cannot edit child of JSON template: %v", err)
	}
	row, err := roles.Clone().Where("id", id).One()
	if err != nil || row["name"].String() != request.Name || row["rules"].String() != "10,20,30,31" || row["btns"].String() != "[31]" {
		t.Fatalf("template child save lost intended grants: %v %v", row, err)
	}
	request.Bbtns = []int{41}
	if _, err = saveRoleRecord(ctx, roles, rules, access, 2, request); err == nil {
		t.Fatal("JSON template allowed an unrelated API permission")
	}
}

func TestStoredRoleMenusAcceptJSONAndCSVWithoutPartialMalformedGrants(t *testing.T) {
	expected := map[int64]bool{10: true, 20: true, 30: true}
	for _, value := range []string{"[10,20,30]", " [10,20,30,10] ", "10,20,30", "10, 20,30,10"} {
		if got := roleCSV(value); !reflect.DeepEqual(got, expected) {
			t.Fatalf("lost menu grants in %q: %v", value, got)
		}
	}
	for _, value := range []string{"", "*", "[10,20", "[10,20,0]", "[10,20,1.5]", "[10,20,\"30\"]", "[10,20,true]", "10,broken,30", "10,-20,30"} {
		if got := roleCSV(value); len(got) != 0 {
			t.Fatalf("malformed input yielded partial grants %q: %v", value, got)
		}
	}
}
