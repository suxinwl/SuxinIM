package admininstall

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/mysql"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/utility/dbcompat"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

func TestDatabaseInstallAndCRUD(t *testing.T) {
	cases := []map[string]string{{"type": "sqlite", "path": filepath.Join(t.TempDir(), "中文目录", "system.db")}}
	if host := os.Getenv("SUXIN_TEST_MYSQL_HOST"); host != "" {
		cases = append(cases, map[string]string{"type": "mysql", "host": host, "port": os.Getenv("SUXIN_TEST_MYSQL_PORT"), "user": os.Getenv("SUXIN_TEST_MYSQL_USER"), "pass": os.Getenv("SUXIN_TEST_MYSQL_PASSWORD"), "version": os.Getenv("SUXIN_TEST_MYSQL_VERSION"), "name": fmt.Sprintf("suxin_test_%d", time.Now().UnixNano())})
	}
	for _, input := range cases {
		t.Run(input["type"]+input["version"], func(t *testing.T) {
			ctx := context.Background()
			v, err := normalizeDatabase(input)
			if err != nil {
				t.Fatal(err)
			}
			db, err := openInstallDatabase(ctx, v)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if v["type"] == "mysql" {
				defer func() {
					server, err := sql.Open("mysql", mysqlDSN(v, ""))
					if err != nil {
						t.Error(err)
						return
					}
					defer server.Close()
					if _, err = server.Exec("DROP DATABASE `" + v["name"] + "`"); err != nil {
						t.Error(err)
					}
				}()
			}
			if err = seedDatabase(ctx, db, v["type"], "../../../devsource/developer/install", "factory_admin", "test-password"); err != nil {
				t.Fatal(err)
			}
			var username, hash, salt string
			if err = db.QueryRow("SELECT username,password,salt FROM gf_admin WHERE id=1").Scan(&username, &hash, &salt); err != nil {
				t.Fatal(err)
			}
			if username != "factory_admin" || hash != gf.Md5(gf.Md5("test-password")+salt) {
				t.Fatal("installer did not set usable login credentials")
			}
			var name, nickname, avatar, email, mobile, tel, department string
			if err = db.QueryRow("SELECT name,nickname,avatar,email,mobile,tel FROM gf_admin WHERE id=1").Scan(&name, &nickname, &avatar, &email, &mobile, &tel); err != nil {
				t.Fatal(err)
			}
			if name != "Suxin管理员" || nickname != "Suxin管理员" || avatar != "/resource/static/brand/logo.png?v=suxin1" || email != "" || mobile != "" || tel != "" {
				t.Fatal("fresh administrator must have Suxin branding and no prefilled contact data")
			}
			if err = db.QueryRow("SELECT name FROM gf_auth_dept WHERE id=1").Scan(&department); err != nil || department != "Suxin技术团队" {
				t.Fatalf("default department: %v", err)
			}
			var quick, mailAccounts int
			if err = db.QueryRow("SELECT COUNT(*) FROM gf_home_quickop WHERE path_url='developer_ai'").Scan(&quick); err != nil || quick != 1 {
				t.Fatalf("AI / MCP shortcut: %d %v", quick, err)
			}
			if err = db.QueryRow("SELECT COUNT(*) FROM gf_email").Scan(&mailAccounts); err != nil || mailAccounts != 0 {
				t.Fatal("fresh installation must not contain an email sending account")
			}
			if existing, err := openInstallDatabase(ctx, v); err == nil {
				existing.Close()
				t.Fatal("installer accepted an existing database")
			}
			cfg := gdb.ConfigNode{Type: v["type"], Name: v["name"], Host: v["host"], Port: v["port"], User: v["user"], Pass: v["pass"], Prefix: "gf_", Extra: v["extra"], CreatedAt: "createtime", UpdatedAt: "updatetime", DeletedAt: "deletetime"}
			orm, err := gdb.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer orm.Close(ctx)
			// These are the same ORM operations used by department, role and menu services.
			id, err := orm.Model("auth_dept").Ctx(ctx).Data(gf.Map{"name": "生产一部", "remark": "", "pid": 1, "status": 0}).InsertAndGetId()
			if err != nil || id <= 1 {
				t.Fatalf("insert: id=%d err=%v", id, err)
			}
			if _, err = orm.Model("auth_dept").Ctx(ctx).Where("id", id).Data(gf.Map{"name": "生产二部"}).Update(); err != nil {
				t.Fatal(err)
			}
			row, err := orm.Model("auth_dept").Ctx(ctx).Where("id", id).One()
			if err != nil || row["name"].String() != "生产二部" || row["createtime"].IsEmpty() {
				t.Fatalf("read/timestamps: %v %v", row, err)
			}
			// Explicit upsert is used by sort/save endpoints.
			if _, err = orm.Model("auth_dept").Ctx(ctx).Data(gf.Map{"id": id, "name": "生产二部", "weigh": 20, "remark": ""}).OnConflict("id").Save(); err != nil {
				t.Fatal(err)
			}
			roles, err := orm.Model("auth_role_access").Ctx(ctx).Where("uid", 1).Array("role_id")
			if err != nil || len(roles) != 1 {
				t.Fatalf("role membership: %v", err)
			}
			count, err := orm.Model("auth_role").Ctx(ctx).WhereIn("id", roles).Where("rules", "*").Count()
			if err != nil || count != 1 {
				t.Fatalf("super-admin permissions: %d %v", count, err)
			}
			tables, err := dbcompat.Tables(ctx, orm)
			if err != nil || len(tables) != 14 {
				t.Fatalf("metadata tables: %d %v", len(tables), err)
			}
			fields, err := dbcompat.Columns(ctx, orm, "gf_admin")
			if err != nil || len(fields) == 0 || fields[0].Name != "id" {
				t.Fatalf("metadata fields: %v", err)
			}
			if _, err = orm.Model("auth_dept").Ctx(ctx).Where("id", id).Delete(); err != nil {
				t.Fatal(err)
			}
			count, err = orm.Model("auth_dept").Ctx(ctx).Where("id", id).Count()
			if err != nil || count != 0 {
				t.Fatalf("soft delete: %d %v", count, err)
			}
			// Re-running the additive migration must preserve the installed data.
			migration := "../../../manifest/sql/v1.0.0.sql"
			if v["type"] == "sqlite" {
				migration = "../../../manifest/sql/v1.0.0.sqlite.sql"
			}
			content, err := os.ReadFile(migration)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if _, err = db.Exec(string(content)); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestNormalizeDatabase(t *testing.T) {
	for _, kind := range []string{"mysql8", "mysql57", "sqlite"} {
		v, err := normalizeDatabase(map[string]string{"type": kind, "host": "localhost", "user": "root", "name": "gosuxin"})
		if err != nil {
			t.Fatal(err)
		}
		if kind == "mysql57" && v["version"] != "5.7" {
			t.Fatal(v)
		}
		if kind == "sqlite" && (!filepath.IsAbs(v["name"]) || !strings.Contains(v["extra"], "WAL")) {
			t.Fatal(v)
		}
	}
	for _, v := range []map[string]string{{"type": "postgres"}, {"type": "sqlite", "path": ":memory:"}, {"type": "sqlite", "path": "a.db?mode=memory"}, {"type": "sqlite", "prefix": "custom_"}, {"type": "mysql8", "host": "localhost", "user": "root", "name": "db`;DROP DATABASE x"}} {
		if _, err := normalizeDatabase(v); err == nil {
			t.Fatalf("accepted invalid input: %v", v)
		}
	}
}
