package runtimeplugin

import (
	"net/http/httptest"
	"testing"
)

func TestProxyStripsHostCredentialsFromEveryPlugin(t *testing.T) {
	for _, name := range []string{"inventory", "ebook", "example"} {
		for _, suffix := range []string{"/admin/", "/api/admin/list", "/public/catalog"} {
			path := "/plugins/" + name + suffix
			request := httptest.NewRequest("GET", path, nil)
			request.Header.Set("Authorization", "Bearer host-token")
			request.Header.Set("Cookie", "gfsessionid=host-session; suxin_plugin_"+name+"_admin=host-token")
			request.Header.Set("X-Request-ID", "request-123")
			stripHostCredentials(name, path, request)
			if request.Header.Get("Authorization") != "" || request.Header.Get("Cookie") != "" {
				t.Fatalf("host credentials leaked: %s", path)
			}
			if request.Header.Get("X-Request-ID") != "request-123" {
				t.Fatal("unrelated request metadata lost")
			}
		}
	}
}
