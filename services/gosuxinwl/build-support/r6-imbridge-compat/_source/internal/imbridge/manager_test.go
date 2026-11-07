package imbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
)

func TestPublicHRMManagerRoutesRequireHRMAdministratorAndKeepEmployeeIMDenied(t *testing.T) {
	t.Setenv("GOSUXIN_HRM_PUBLIC_ADMINS_ONLY", "true")
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	for _, statement := range []string{
		`INSERT INTO gf_admin(id,username,status) VALUES(50,'public-manager',0)`,
		`INSERT INTO gf_auth_role(id,rules,btns,status) VALUES(50,'*','',0)`,
		`INSERT INTO gf_auth_role_access(uid,role_id) VALUES(50,50)`,
	} {
		_, err := store.DB.ExecContext(ctx, statement)
		must(t, err)
	}
	svc := &Service{Store: store, Remote: remote, Cache: newFakeCache()}
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/", func(group *ghttp.RouterGroup) {
		// Identity/RBAC/CSRF are supplied by the host and core middleware in
		// production; their rejection behavior has separate handler tests.
		group.Middleware(func(r *ghttp.Request) {
			r.SetCtxVar("uid", r.Header.Get("X-Test-UID"))
			r.Middleware.Next()
		})
		bindManager(group, func(context.Context) (*Service, func(), error) {
			return svc, func() {}, nil
		})
	})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	for _, test := range []struct {
		name, uid, path string
		status          int
	}{
		{"HRM manager reads network configuration", "50", "/api/admin/im/manager/openim/access/networks", 200},
		{"HRM manager reads certificate metadata", "50", "/api/admin/im/manager/openim/certificate", 200},
		{"communication employee cannot manage networks", "1", "/api/admin/im/manager/openim/access/networks", 403},
		{"communication employee cannot read certificate metadata", "1", "/api/admin/im/manager/openim/certificate", 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s", server.GetListenedPort(), test.path), nil)
			request.Header.Set("X-Test-UID", test.uid)
			request.Header.Set("X-GoSuxin-Client-IP", "203.0.113.9")
			request.Header.Set("X-Forwarded-For", "192.168.10.9")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var envelope struct {
				Data NetworkPolicy `json:"data"`
			}
			if err = json.NewDecoder(response.Body).Decode(&envelope); err != nil || response.StatusCode != test.status {
				t.Fatalf("manager status=%d want=%d decode=%v", response.StatusCode, test.status, err)
			}
			if test.status == 200 && test.path == "/api/admin/im/manager/openim/access/networks" && (envelope.Data.CurrentIP != "203.0.113.9" || envelope.Data.CurrentZone != "internet") {
				t.Fatal("public manager source was incorrectly classified as LAN")
			}
		})
	}
	if _, err := svc.Login(WithClientIP(ctx, "203.0.113.9"), 1, "web"); err == nil {
		t.Fatal("public HRM management changed employee Internet IM defaults")
	}
}
