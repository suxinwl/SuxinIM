package runtimeplugin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	hostauth "github.com/suxinwl/GoSuxin/utility/auth"
)

func runtimePluginCSRFSession(t *testing.T) string {
	t.Helper()
	secret, encrypt, dynamic := hostauth.SecretKey, hostauth.EncryptKey, hostauth.DynamicToken
	timeout, refresh, multi, cache := hostauth.TokenTimeout, hostauth.MaxRefresh, hostauth.MultiLogin, hostauth.TokenCacheKey
	hostauth.SecretKey, hostauth.EncryptKey, hostauth.DynamicToken = gvar.New("csrf-test-signing-secret"), gvar.New("0123456789abcdef0123456789abcdef"), gvar.New("")
	hostauth.TokenTimeout, hostauth.MaxRefresh, hostauth.MultiLogin, hostauth.TokenCacheKey = gvar.New(600), gvar.New(0), gvar.New(false), gvar.New("csrf-test-"+guid.S())
	t.Cleanup(func() {
		hostauth.SecretKey, hostauth.EncryptKey, hostauth.DynamicToken = secret, encrypt, dynamic
		hostauth.TokenTimeout, hostauth.MaxRefresh, hostauth.MultiLogin, hostauth.TokenCacheKey = timeout, refresh, multi, cache
	})
	key := guid.S()
	token, err := hostauth.GenerateToken(context.Background(), key, map[string]interface{}{"uid": 71})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = hostauth.RemoveToken(context.Background(), key) })
	return token
}

func TestRuntimePluginCookieWriteCSRF(t *testing.T) {
	token := runtimePluginCSRFSession(t)
	server := g.Server(guid.S())
	server.Group("/plugins/inventory", func(group *ghttp.RouterGroup) {
		group.Middleware(runtimePluginAuth)
		group.POST("/admin/__host_auth/bootstrap", runtimePluginBootstrap)
		group.ALL("/*path", func(r *ghttp.Request) { r.Response.Write("passed") })
	})
	server.SetPort(0)
	server.SetDumpRouterMap(false)
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	base := fmt.Sprintf("http://127.0.0.1:%d", server.GetListenedPort())
	crossPort := fmt.Sprintf("http://127.0.0.1:%d", server.GetListenedPort()+1)
	xhr := "XMLHttpRequest"
	tests := []struct {
		name, method, origin, referer, requestedWith, authorization, forwardedHost, forwardedProto string
		want                                                                                       int
	}{
		{name: "form post with cookie", method: "POST", origin: base, want: 403},
		{name: "cookie post without xhr or origin", method: "POST", want: 403},
		{name: "same-site cross-origin post", method: "POST", origin: crossPort, requestedWith: xhr, want: 403},
		{name: "cross-site post", method: "POST", origin: "http://attacker.invalid", requestedWith: xhr, want: 403},
		{name: "forwarded host cannot authorize origin", method: "POST", origin: "http://attacker.invalid", forwardedHost: "attacker.invalid", requestedWith: xhr, want: 403},
		{name: "wrong scheme", method: "POST", origin: strings.Replace(base, "http:", "https:", 1), requestedWith: xhr, want: 403},
		{name: "opaque origin", method: "POST", origin: "null", requestedWith: xhr, want: 403},
		{name: "origin containing path", method: "POST", origin: base + "/evil", requestedWith: xhr, want: 403},
		{name: "cross-origin referer", method: "POST", referer: crossPort + "/form", requestedWith: xhr, want: 403},
		{name: "invalid authorization still requires cookie csrf", method: "POST", authorization: "invalid-token", origin: crossPort, requestedWith: xhr, want: 403},
		{name: "same-origin xhr", method: "POST", origin: base, referer: base + "/plugins/inventory/admin/", requestedWith: xhr, want: 200},
		{name: "xhr without optional origin", method: "POST", requestedWith: xhr, want: 200},
		{name: "same-origin put", method: "PUT", origin: base, requestedWith: xhr, want: 200},
		{name: "patch still needs xhr", method: "PATCH", origin: base, want: 403},
		{name: "same-origin delete", method: "DELETE", origin: base, requestedWith: xhr, want: 200},
		{name: "https proxy origin", method: "POST", origin: strings.Replace(base, "http:", "https:", 1), requestedWith: xhr, forwardedProto: "https", want: 200},
		{name: "get remains available", method: "GET", origin: crossPort, want: 200},
		{name: "head remains available", method: "HEAD", origin: crossPort, want: 200},
		{name: "options remains available", method: "OPTIONS", origin: crossPort, want: 200},
		{name: "authorization bootstrap remains compatible", method: "POST", authorization: token, origin: crossPort, want: 200},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := "/plugins/inventory/api/admin/save"
			if test.authorization == token {
				path = "/plugins/inventory/admin/__host_auth/bootstrap"
			}
			request, err := http.NewRequest(test.method, base+path, strings.NewReader("table=type"))
			if err != nil {
				t.Fatal(err)
			}
			request.AddCookie(&http.Cookie{Name: runtimePluginCookieName("inventory"), Value: token})
			for key, value := range map[string]string{
				"Origin": test.origin, "Referer": test.referer, "X-Requested-With": test.requestedWith,
				"Authorization": test.authorization, "X-Forwarded-Host": test.forwardedHost, "X-Forwarded-Proto": test.forwardedProto,
			} {
				if value != "" {
					request.Header.Set(key, value)
				}
			}
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != test.want {
				t.Fatalf("status=%d want=%d", response.StatusCode, test.want)
			}
			if test.want == 403 && strings.Contains(string(body), "passed") {
				t.Fatal("forbidden write reached plugin handler")
			}
			if test.authorization == token {
				if !strings.Contains(string(body), `"authenticated":true`) {
					t.Fatal("authorization bootstrap did not complete")
				}
			} else if test.want == 200 && test.method != "HEAD" && string(body) != "passed" {
				t.Fatal("authorized request did not reach plugin handler")
			}
		})
	}
}
