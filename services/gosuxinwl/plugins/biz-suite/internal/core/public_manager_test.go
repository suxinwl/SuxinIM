package core

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/internal/imbridge"
)

func TestPublicManagerActualRoutesRetainManagePermission(t *testing.T) {
	t.Setenv("GOSUXIN_IM_ENABLED", "false")
	ctx := context.Background()
	db, err := gdb.New(gdb.ConfigNode{Type: "sqlite", Name: filepath.Join(t.TempDir(), "public-manager.db"), Prefix: "gf_"})
	if err != nil {
		t.Fatal(err)
	}
	if err = Init(db); err != nil {
		t.Fatal(err)
	}
	business := DB()
	t.Cleanup(func() { _ = db.Close(ctx); _ = business.Close(ctx) })
	for _, statement := range []string{
		`CREATE TABLE gf_admin(id INTEGER PRIMARY KEY,status INTEGER)`,
		`INSERT INTO gf_admin VALUES(1,0),(2,0)`,
		`CREATE TABLE gf_auth_role_access(uid INTEGER,role_id INTEGER)`,
		`CREATE TABLE gf_auth_role(id INTEGER PRIMARY KEY,rules TEXT,btns TEXT,status INTEGER)`,
		`CREATE TABLE gf_auth_rule(id INTEGER PRIMARY KEY,permission TEXT,status INTEGER,type INTEGER)`,
		`INSERT INTO gf_auth_rule VALUES(10,'biz-suite.im.read',0,2),(11,'biz-suite.hrm.manage',0,2)`,
		`INSERT INTO gf_auth_role VALUES(1,'','10',0),(2,'','10,11',0)`,
		`INSERT INTO gf_auth_role_access VALUES(1,1),(2,2)`,
	} {
		if _, err = db.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) {
			r.SetCtxVar("uid", r.Header.Get("X-Test-UID"))
			r.Middleware.Next()
		}, Middleware)
		imbridge.BindManager(group)
	})
	if err = server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	for _, uid := range []string{"1", "2"} {
		for _, route := range []string{"access/networks", "access/employees", "certificate"} {
			request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/admin/im/manager/openim/%s", server.GetListenedPort(), route), nil)
			request.Header.Set("X-Test-UID", uid)
			request.Header.Set("X-GoSuxin-Client-IP", "203.0.113.9")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusForbidden {
				t.Fatalf("user without IM manage reached actual manager route %s: %d", route, response.StatusCode)
			}
		}
	}
}
