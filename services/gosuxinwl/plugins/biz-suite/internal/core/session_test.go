package core

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
)

func TestIMSessionRequiresCurrentUserAndPermissions(t *testing.T) {
	ctx := context.Background()
	db, err := gdb.New(gdb.ConfigNode{Type: "sqlite", Name: filepath.Join(t.TempDir(), "im-session.db"), Prefix: "gf_"})
	if err != nil {
		t.Fatal(err)
	}
	if err = Init(db); err != nil {
		t.Fatal(err)
	}
	business := DB()
	t.Cleanup(func() { business.Close(ctx); db.Close(ctx) })
	for _, sql := range []string{
		`CREATE TABLE gf_admin(id INTEGER PRIMARY KEY,status INTEGER)`,
		`INSERT INTO gf_admin VALUES(1,0),(2,0),(3,0),(4,0),(5,1)`,
		`CREATE TABLE gf_auth_role_access(uid INTEGER,role_id INTEGER)`,
		`CREATE TABLE gf_auth_role(id INTEGER PRIMARY KEY,rules TEXT,btns TEXT,status INTEGER)`,
		`CREATE TABLE gf_auth_rule(id INTEGER PRIMARY KEY,permission TEXT,status INTEGER,type INTEGER)`,
		`INSERT INTO gf_auth_role_access VALUES(1,1),(2,2),(3,3),(4,4),(5,1)`,
		`INSERT INTO gf_auth_role VALUES(1,'','10',0),(2,'','11',0),(3,'','12',0),(4,'','10',1)`,
		`INSERT INTO gf_auth_rule VALUES(10,'biz-suite.im.read',0,2),(11,'biz-suite.im.manage',0,2),(12,'biz-suite.im.write',0,2)`,
	} {
		if _, err = db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) {
			r.SetCtx(context.WithValue(r.Context(), "uid", gconv.Int64(r.Header.Get("X-Test-UID"))))
			r.Middleware.Next()
		}, Middleware)
		BindAccess(group)
	})
	if err = server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	request := func(uid int, expectedStatus int) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/admin/im/session", server.GetListenedPort()), nil)
		req.Header.Set("X-Test-UID", fmt.Sprint(uid))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != expectedStatus {
			t.Fatalf("uid %d status %d, want %d: %v", uid, resp.StatusCode, expectedStatus, err)
		}
		if expectedStatus != http.StatusOK {
			return
		}
		var payload struct {
			Code int `json:"code"`
			Data struct {
				UID         int64    `json:"uid"`
				Permissions []string `json:"permissions"`
			} `json:"data"`
		}
		if err = json.Unmarshal(body, &payload); err != nil || payload.Data.UID != int64(uid) || len(payload.Data.Permissions) != 1 {
			t.Fatalf("session did not report current identity and permissions: %v", err)
		}
		if resp.Header.Get("Cache-Control") != "no-store" || strings.Contains(strings.ToLower(string(body)), "token") || resp.Header.Get("Set-Cookie") != "" {
			t.Fatal("worker must not cache identity, issue credentials, or return host tokens")
		}
	}
	request(0, 401)
	request(1, 200) // read
	request(2, 200) // manage implies read
	request(3, 403) // write alone does not grant read
	request(4, 403) // disabled role
	request(5, 401) // disabled account
	if _, err = db.Exec(ctx, `UPDATE gf_auth_rule SET status=1 WHERE id=10`); err != nil {
		t.Fatal(err)
	}
	request(1, 403) // revoke while the same page remains open
	if _, err = db.Exec(ctx, `UPDATE gf_admin SET status=1 WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	request(2, 401)
}
