package imbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
)

type capacityRemoteProbe struct {
	*fakeRemote
	calls atomic.Int32
}

func (p *capacityRemoteProbe) Manage(_ context.Context, operation string, body any) (json.RawMessage, error) {
	if operation != "storage/capacity" || len(body.(map[string]any)) != 0 {
		return nil, ErrInvalid
	}
	p.calls.Add(1)
	return json.RawMessage(`{"dataCapacity":false,"status":"unknown","filesystems":[]}`), nil
}

func TestCapacityManagementRouteIsRegisteredBoundedAndAdminOnly(t *testing.T) {
	t.Setenv("GOSUXIN_HRM_PUBLIC_ADMINS_ONLY", "true")
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	for _, statement := range []string{
		`INSERT INTO gf_admin(id,username,status) VALUES(50,'synthetic-capacity-manager',0)`,
		`INSERT INTO gf_auth_role(id,rules,btns,status) VALUES(50,'*','',0)`,
		`INSERT INTO gf_auth_role_access(uid,role_id) VALUES(50,50)`,
	} {
		_, err := store.DB.ExecContext(ctx, statement)
		must(t, err)
	}
	probe := &capacityRemoteProbe{fakeRemote: remote}
	svc := &Service{Store: store, Remote: probe, Cache: newFakeCache()}
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.SetLogStdout(false)
	server.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) {
			r.SetCtxVar("uid", r.Header.Get("X-Test-UID"))
			r.Middleware.Next()
		})
		bindManager(group, func(context.Context) (*Service, func(), error) { return svc, func() {}, nil })
	})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	base := "/api/admin/im/manager/openim/"
	for _, test := range []struct {
		name, uid, method, path, body string
		status                        int
	}{
		{"advertised", "50", "GET", base + "capabilities", "", 200},
		{"empty operation", "50", "POST", base + "operations/storage/capacity", `{}`, 200},
		{"injected upstream", "50", "POST", base + "operations/storage/capacity", `{"url":"http://invalid"}`, 400},
		{"injected admin token", "50", "POST", base + "operations/storage/capacity", `{"adminToken":"synthetic"}`, 400},
		{"null body", "50", "POST", base + "operations/storage/capacity", `null`, 400},
		{"employee denied", "1", "POST", base + "operations/storage/capacity", `{}`, 403},
		{"anonymous denied", "0", "POST", base + "operations/storage/capacity", `{}`, 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, _ := http.NewRequest(test.method, fmt.Sprintf("http://127.0.0.1:%d%s", server.GetListenedPort(), test.path), strings.NewReader(test.body))
			request.Host = "192.168.10.11:9880"
			request.Header.Set("X-Test-UID", test.uid)
			request.Header.Set("Content-Type", "application/json")
			response, err := http.DefaultClient.Do(request)
			must(t, err)
			defer response.Body.Close()
			var result struct {
				Data struct {
					Operations []string `json:"operations"`
				} `json:"data"`
			}
			must(t, json.NewDecoder(response.Body).Decode(&result))
			if response.StatusCode != test.status {
				t.Fatalf("status=%d want=%d", response.StatusCode, test.status)
			}
			if test.name == "advertised" {
				found := false
				for _, name := range result.Data.Operations {
					found = found || name == "storage/capacity"
				}
				if !found {
					t.Fatal("capacity route missing from advertised capabilities")
				}
			}
		})
	}
	if probe.calls.Load() != 1 {
		t.Fatal("only the authorized empty capacity operation may reach the bridge")
	}
}
