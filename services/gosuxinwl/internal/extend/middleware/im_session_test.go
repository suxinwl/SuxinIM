package middleware

import (
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
)

func TestIMSessionGateDenialHasForbiddenStatus(t *testing.T) {
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/plugins", func(group *ghttp.RouterGroup) {
		// No trusted identity means the shared gate must deny before any DB query.
		group.Middleware(Auth)
		group.ALL("/*path", func(r *ghttp.Request) { r.Response.Write("allowed") })
	})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"biz-suite/api/admin/im/session", http.StatusForbidden},
		{"inventory/api/admin/im/session", http.StatusOK}, // Existing plugin response contract is unchanged.
	} {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/plugins/%s", server.GetListenedPort(), tc.path))
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != tc.status || string(body) == "allowed" {
			t.Fatalf("gate denial %s: status %d want %d", tc.path, resp.StatusCode, tc.status)
		}
	}
}
