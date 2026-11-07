package clogic

import (
	"context"
	"path/filepath"
	"testing"

	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
)

func TestRoleManagementSeedRepairsMissingAddWithoutResettingGrants(t *testing.T) {
	ctx := context.Background()
	db, err := gdb.New(gdb.ConfigNode{Type: "sqlite", Name: filepath.Join(t.TempDir(), "role-seed.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(ctx) })
	_, err = db.Exec(ctx, `CREATE TABLE rules(id INTEGER PRIMARY KEY AUTOINCREMENT,uid INTEGER,pid INTEGER,type INTEGER,status INTEGER,title TEXT NOT NULL,des TEXT NOT NULL,locale TEXT NOT NULL,icon TEXT NOT NULL,routepath TEXT NOT NULL,component TEXT NOT NULL,redirect TEXT NOT NULL,routename TEXT NOT NULL,path TEXT NOT NULL,permission TEXT NOT NULL,weigh INTEGER,requiresauth INTEGER,hideinmenu INTEGER,hidechildreninmenu INTEGER,onlypage INTEGER,noaffix INTEGER,keepalive INTEGER)`)
	if err != nil {
		t.Fatal(err)
	}
	rules := db.Model("rules").Safe()
	_, err = rules.Clone().Data(map[string]any{"id": 7, "pid": 3, "type": 1, "status": 0, "title": "角色管理", "des": "", "locale": "", "icon": "", "routepath": "role", "component": "system/role/index", "redirect": "", "routename": "role", "path": "", "permission": ""}).Insert()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = EnsureRoleManagementRules(ctx, rules); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := rules.Clone().Where("type", 2).OrderAsc("id").All()
	if err != nil || len(rows) != 2 {
		t.Fatalf("seed duplicated or omitted rules: %v %v", rows, err)
	}
	var saveID int64
	for _, row := range rows {
		if row["pid"].Int64() != 7 || row["requiresauth"].Int() != 1 {
			t.Fatal("role permissions escaped role menu/auth")
		}
		if row["path"].String() == "/admin/system/role/save" {
			saveID = row["id"].Int64()
			if row["permission"].String() != "add" {
				t.Fatal("toolbar add permission not supplied")
			}
		}
	}
	if saveID == 0 {
		t.Fatal("missing save capability")
	}
	if _, err = rules.Clone().Where("id", saveID).Data(map[string]any{"status": 1, "permission": "custom-add"}).Update(); err != nil {
		t.Fatal(err)
	}
	if err = EnsureRoleManagementRules(ctx, rules); err != nil {
		t.Fatal(err)
	}
	row, err := rules.Clone().Where("id", saveID).One()
	if err != nil || row["status"].Int() != 1 || row["permission"].String() != "custom-add" {
		t.Fatal("startup overwrote an administrator's disabled or customized rule")
	}
}
