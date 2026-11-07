package core

import (
	"context"
	"fmt"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"net/http"
	"path/filepath"
	"testing"
)

func TestPermissionIsolation(t *testing.T) {
	ctx := context.Background()
	db, err := gdb.New(gdb.ConfigNode{Type: "sqlite", Name: filepath.Join(t.TempDir(), "access.db"), Prefix: "gf_"})
	if err != nil {
		t.Fatal(err)
	}
	if err = Init(db); err != nil {
		t.Fatal(err)
	}
	business := DB()
	defer db.Close(ctx)
	defer business.Close(ctx)
	for _, q := range []string{
		`CREATE TABLE gf_admin(id INTEGER PRIMARY KEY,status INTEGER)`,
		`INSERT INTO gf_admin VALUES(1,0),(2,0),(3,0),(4,0),(5,1),(6,0)`,
		`CREATE TABLE gf_auth_role_access(uid INTEGER,role_id INTEGER)`,
		`CREATE TABLE gf_auth_role(id INTEGER PRIMARY KEY,rules TEXT,btns TEXT,status INTEGER)`,
		`CREATE TABLE gf_auth_rule(id INTEGER PRIMARY KEY,permission TEXT,status INTEGER,type INTEGER)`,
		`INSERT INTO gf_auth_role_access VALUES(1,1),(2,2),(3,3),(4,4),(5,1),(6,5)`,
		`INSERT INTO gf_auth_role VALUES(1,'','10',0),(2,'','11',0),(3,'','11',1),(4,'*','',0),(5,'','12',0)`,
		`INSERT INTO gf_auth_rule VALUES(10,'biz-suite.hrm.read',0,2),(11,'biz-suite.im.manage',0,2),(12,'biz-suite.wecom.write',0,2)`,
	} {
		if _, err = db.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	user := func(id int64) context.Context { return context.WithValue(ctx, "uid", id) }
	if !Has(user(1), "biz-suite.hrm.read") || Has(user(1), "biz-suite.hrm.write") || Has(user(1), "biz-suite.im.read") {
		t.Fatal("read grant escaped scope")
	}
	if !Has(user(2), "biz-suite.im.write") || !Has(user(2), "biz-suite.im.read") {
		t.Fatal("manage must imply read and write")
	}
	if Has(user(3), "biz-suite.im.read") || Has(user(0), "biz-suite.hrm.read") {
		t.Fatal("disabled or anonymous role allowed")
	}
	s := g.Server(guid.S())
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) {
			if r.Header.Get("X-Test-UID") == "1" {
				r.SetCtx(user(1))
			}
			if r.Header.Get("X-Test-UID") == "4" {
				r.SetCtx(user(4))
			}
			if r.Header.Get("X-Test-UID") == "6" {
				r.SetCtx(user(6))
			}
			if r.Header.Get("X-Test-UID") == "5" {
				r.SetCtx(user(5))
			}
			r.Middleware.Next()
		}, Middleware)
		for _, p := range []string{"hrm/employees", "im/conversations", "wecom/settings", "wecom/sync/contact", "wecom/diagnostics/contact", "wecom/diagnostics/app", "wecom/diagnostics/checkin", "wecom/diagnostics/approval", "wecom/credentials/contact", "wecom/organization/sync", "wecom/organization/prune", "wecom/departments/mapping"} {
			group.ALL("/api/admin/"+p, func(r *ghttp.Request) { r.Response.Write("ok") })
		}
	})
	if err = s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	for _, tc := range []struct {
		method, path, uid string
		status            int
	}{{"GET", "hrm/employees", "", 401}, {"GET", "hrm/employees", "1", 200}, {"POST", "hrm/employees", "1", 403}, {"GET", "im/conversations", "1", 403}, {"GET", "hrm/employees", "5", 401}, {"POST", "wecom/settings", "6", 403}, {"POST", "wecom/sync/contact", "6", 403}, {"POST", "wecom/sync/contact", "4", 200}, {"POST", "wecom/settings", "4", 200},
		{"POST", "wecom/diagnostics/contact", "6", 403}, {"POST", "wecom/diagnostics/app", "6", 403}, {"POST", "wecom/diagnostics/checkin", "6", 403}, {"POST", "wecom/diagnostics/approval", "6", 403}, {"POST", "wecom/credentials/contact", "6", 403}, {"POST", "wecom/diagnostics/app", "", 401}, {"POST", "wecom/diagnostics/contact", "4", 200},
		{"GET", "wecom/organization/sync", "6", 403}, {"POST", "wecom/organization/sync", "6", 403}, {"POST", "wecom/departments/mapping", "6", 403}, {"GET", "wecom/organization/sync", "4", 200}, {"POST", "wecom/organization/sync", "4", 200}, {"POST", "wecom/departments/mapping", "4", 200},
		{"GET", "wecom/organization/prune", "6", 403}, {"POST", "wecom/organization/prune", "6", 403}, {"GET", "wecom/organization/prune", "4", 200}, {"POST", "wecom/organization/prune", "4", 200}, {"POST", "wecom/organization/prune", "", 401}} {
		req, _ := http.NewRequest(tc.method, fmt.Sprintf("http://127.0.0.1:%d/api/admin/%s", s.GetListenedPort(), tc.path), nil)
		req.Header.Set("X-Test-UID", tc.uid)
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		res.Body.Close()
		if res.StatusCode != tc.status {
			t.Fatalf("%s %s=%d want%d", tc.method, tc.path, res.StatusCode, tc.status)
		}
	}
}

func TestSeedSuppliesStrictHostFields(t *testing.T) {
	ctx := context.Background()
	db, err := gdb.New(gdb.ConfigNode{Type: "sqlite", Name: filepath.Join(t.TempDir(), "strict.db"), Prefix: "gf_"})
	if err != nil {
		t.Fatal(err)
	}
	if err = Init(db); err != nil {
		t.Fatal(err)
	}
	business := DB()
	defer db.Close(ctx)
	defer business.Close(ctx)
	// These strings intentionally have no defaults, matching MySQL's host DDL.
	_, err = db.Exec(ctx, `CREATE TABLE gf_auth_rule(id INTEGER PRIMARY KEY,uid INTEGER,pid INTEGER,type INTEGER,title TEXT NOT NULL,des TEXT NOT NULL,locale TEXT NOT NULL,icon TEXT NOT NULL,routepath TEXT NOT NULL,component TEXT NOT NULL,redirect TEXT NOT NULL,routename TEXT NOT NULL,path TEXT NOT NULL,permission TEXT,status INTEGER,requiresauth INTEGER,hideinmenu INTEGER)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = HostModel("auth_rule").Data(g.Map{"id": 1, "type": 1, "title": "plugin", "des": "", "locale": "", "icon": "", "routepath": "", "component": "", "redirect": "", "routename": "runtime_plugin_biz-suite", "path": "", "status": 0}).Insert()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = Seed(ctx); err != nil {
			t.Fatal(err)
		}
	}
	n, err := HostModel("auth_rule").Count()
	if err != nil || n != 14 {
		t.Fatalf("seed not idempotent: %d %v", n, err)
	}
	primary, _ := HostModel("auth_rule").Where("id", 1).One()
	imParent, _ := HostModel("auth_rule").Where("routename", "biz_suite_im").Value("id")
	if primary["title"].String() != "智慧HRM" || imParent.Int64() <= 1 {
		t.Fatal("old menu ID was replaced or independent IM menu missing")
	}
	for _, module := range Modules {
		want := int64(1)
		if module == "im" {
			want = imParent.Int64()
		}
		parents, err := HostModel("auth_rule").WhereLike("permission", "biz-suite."+module+".%").Array("pid")
		count := 3
		if module == "hrm" {
			count = 5
		}
		if err != nil || len(parents) != count {
			t.Fatalf("missing module permission parents: %s", module)
		}
		for _, parent := range parents {
			if parent.Int64() != want {
				t.Fatalf("wrong parent for %s", module)
			}
		}
	}
}

func TestEntryMenusReflectExistingPermissions(t *testing.T) {
	for _, tt := range []struct {
		permissions   []string
		titles, paths []string
	}{
		{nil, nil, nil},
		{[]string{"biz-suite.hrm.read"}, []string{"智慧HRM"}, []string{"/hrm"}},
		{[]string{"biz-suite.wecom.manage"}, []string{"智慧HRM"}, []string{"/wecom"}},
		{[]string{"biz-suite.im.manage"}, []string{"企业即时通讯"}, []string{"/im"}},
		{[]string{"biz-suite.hrm.write", "biz-suite.im.write"}, []string{"智慧HRM"}, []string{"/hrm"}},
		{[]string{"biz-suite.hrm.self"}, []string{"智慧HRM"}, []string{"/hrm"}},
		{[]string{"biz-suite.hrm.department"}, []string{"智慧HRM"}, []string{"/hrm"}},
		{[]string{"biz-suite.hrm.read", "biz-suite.im.read", "biz-suite.wecom.read"}, []string{"智慧HRM", "企业即时通讯"}, []string{"/hrm", "/im"}},
	} {
		menus := entryMenus(tt.permissions)
		if len(menus) != len(tt.titles) {
			t.Fatalf("unexpected entry count for %v: %v", tt.permissions, menus)
		}
		for i, menu := range menus {
			if menu["title"] != tt.titles[i] || menu["path"] != tt.paths[i] {
				t.Fatalf("unexpected entry for %v: %v", tt.permissions, menus)
			}
		}
	}
}
