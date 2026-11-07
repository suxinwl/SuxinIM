package runtimeplugin

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
)

func TestRuntimePluginUnauthenticatedNavigationAndAPI(t *testing.T) {
	s := g.Server(guid.S())
	s.Group("/plugins/:plugin", func(group *ghttp.RouterGroup) {
		group.Group("/admin", func(admin *ghttp.RouterGroup) {
			admin.Middleware(runtimeAdminPageRedirect)
			admin.ALL("/*path", func(r *ghttp.Request) { r.Response.Write("shell") })
		})
		group.Group("/api/admin", func(api *ghttp.RouterGroup) {
			api.Middleware(runtimeAdminPageRedirect)
			api.ALL("/*path", func(r *ghttp.Request) { r.Response.Write("api") })
		})
	})
	s.SetPort(0)
	s.SetDumpRouterMap(false)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	time.Sleep(50 * time.Millisecond)

	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	base := "http://127.0.0.1:" + gconvString(s.GetListenedPort())

	page, err := client.Get(base + "/plugins/inventory/admin/")
	if err != nil {
		t.Fatal(err)
	}
	defer page.Body.Close()
	if page.StatusCode != http.StatusFound {
		t.Fatalf("page status = %d", page.StatusCode)
	}
	if got := page.Header.Get("Location"); got != "/suxinweb/login?redirect=%2Fplugins%2Finventory%2Fadmin%2F" {
		t.Fatalf("page redirect = %q", got)
	}
	for _, item := range []struct {
		path   string
		status int
	}{
		{"/plugins/inventory/admin/hrm/employees?search=test", http.StatusFound},
		{"/plugins/inventory/admin/assets/app.js", http.StatusUnauthorized},
		{"/plugins/inventory/admin/__host_auth/bootstrap", http.StatusUnauthorized},
	} {
		response, err := client.Get(base + item.path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != item.status {
			t.Fatalf("%s status=%d, want %d", item.path, response.StatusCode, item.status)
		}
		if item.status == http.StatusFound && !strings.Contains(response.Header.Get("Location"), "%3Fsearch%3Dtest") {
			t.Fatal("deep link query was lost")
		}
	}

	api, err := client.Get(base + "/plugins/inventory/api/admin/dashboard")
	if err != nil {
		t.Fatal(err)
	}
	defer api.Body.Close()
	body, _ := io.ReadAll(api.Body)
	if api.StatusCode != http.StatusUnauthorized || !strings.Contains(api.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("api status=%d content-type=%q body=%q", api.StatusCode, api.Header.Get("Content-Type"), body)
	}
	if !strings.Contains(string(body), "GoSuxin") || strings.Contains(string(body), "decrypt Token") {
		t.Fatalf("api returned an internal token error instead of a login prompt: %s", body)
	}
}

func gconvString(value int) string {
	return fmt.Sprintf("%d", value)
}
