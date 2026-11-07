package runtimeplugin

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	hostauth "github.com/suxinwl/GoSuxin/utility/auth"
)

func TestBizSuitePublicManagerCookieStillRequiresCSRF(t *testing.T) {
	token := runtimePluginCSRFSession(t)
	t.Cleanup(hostauth.SetTokenRequestPolicy(nil, func(r *ghttp.Request, _ *hostauth.CustomClaims) {
		r.Cookie.SetHttpCookie(&http.Cookie{Name: "successful-session-resource-cookie", Value: "fixture", Path: "/resource/uploads/", HttpOnly: true, Secure: true})
	}))
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/plugins/biz-suite", func(group *ghttp.RouterGroup) {
		group.Middleware(runtimePluginAuth)
		group.POST("/api/admin/im/manager/openim/certificate", func(r *ghttp.Request) { r.Response.Write("manager handler reached") })
	})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	for _, test := range []struct {
		name, origin, xhr string
		status            int
	}{
		{"cookie without CSRF marker", "https://kf.szputy.com:9880", "", 403},
		{"cookie from foreign origin", "https://attacker.invalid", "XMLHttpRequest", 403},
		{"same origin cookie manager request", "https://kf.szputy.com:9880", "XMLHttpRequest", 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/plugins/biz-suite/api/admin/im/manager/openim/certificate", server.GetListenedPort()), strings.NewReader(`{}`))
			request.Host = "kf.szputy.com:9880"
			request.AddCookie(&http.Cookie{Name: runtimePluginCookieName("biz-suite"), Value: token})
			request.Header.Set("X-GoSuxin-Client-IP", "203.0.113.9")
			request.Header.Set("X-Forwarded-Proto", "https")
			request.Header.Set("Origin", test.origin)
			request.Header.Set("X-Requested-With", test.xhr)
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil || response.StatusCode != test.status || (strings.Contains(string(body), "manager handler reached") != (test.status == 200)) {
				t.Fatalf("manager csrf status=%d want=%d", response.StatusCode, test.status)
			}
			if test.status == 403 && len(response.Cookies()) != 0 {
				t.Fatal("forbidden cookie write issued a new cookie")
			}
		})
	}
}
