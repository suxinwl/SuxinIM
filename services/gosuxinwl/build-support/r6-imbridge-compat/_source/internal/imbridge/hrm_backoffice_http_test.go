package imbridge

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	hostauth "github.com/suxinwl/GoSuxin/utility/auth"
)

func hrmBackofficeTestSession(t *testing.T, uid int64) string {
	t.Helper()
	secret, encrypt, dynamic := hostauth.SecretKey, hostauth.EncryptKey, hostauth.DynamicToken
	timeout, refresh, multi, cache := hostauth.TokenTimeout, hostauth.MaxRefresh, hostauth.MultiLogin, hostauth.TokenCacheKey
	hostauth.SecretKey, hostauth.EncryptKey, hostauth.DynamicToken = gvar.New("backoffice-test-signing-secret"), gvar.New("0123456789abcdef0123456789abcdef"), gvar.New("")
	hostauth.TokenTimeout, hostauth.MaxRefresh, hostauth.MultiLogin, hostauth.TokenCacheKey = gvar.New(600), gvar.New(0), gvar.New(false), gvar.New("backoffice-test-"+guid.S())
	t.Cleanup(func() {
		hostauth.SecretKey, hostauth.EncryptKey, hostauth.DynamicToken = secret, encrypt, dynamic
		hostauth.TokenTimeout, hostauth.MaxRefresh, hostauth.MultiLogin, hostauth.TokenCacheKey = timeout, refresh, multi, cache
	})
	key := guid.S()
	token, err := hostauth.GenerateToken(context.Background(), key, map[string]interface{}{"uid": uid})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = hostauth.RemoveToken(context.Background(), key) })
	return token
}

func TestHRMBackofficeTokenPolicyGuardsHostAndPluginButNotEmployeeIM(t *testing.T) {
	t.Setenv("GOSUXIN_HRM_PUBLIC_ADMINS_ONLY", "true")
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	opaque := hrmBackofficeTestSession(t, 1)
	t.Cleanup(registerHRMBackofficePolicy(store))
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.BindHandler("/suxinweb", func(r *ghttp.Request) { r.Response.Write("public-shell") })
	for _, route := range []string{"/admin/user/getMenu", "/plugins/biz-suite/api/admin/im/manager/openim/certificate", "/common/im-sso/directory"} {
		server.BindHandler(route, func(r *ghttp.Request) {
			if _, err := hostauth.ParseToken(r); err != nil {
				r.Response.WriteStatus(http.StatusForbidden)
				return
			}
			r.Response.Write("authorized")
		})
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	request := func(route, source string, status int) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s", server.GetListenedPort(), route), nil)
		req.Header.Set("Authorization", opaque)
		req.Header.Set("X-GoSuxin-Client-IP", source)
		req.Header.Set("X-Forwarded-For", "192.168.10.9")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != status {
			t.Fatalf("%s from %s status=%d want=%d", route, source, resp.StatusCode, status)
		}
	}
	for _, route := range []string{"/admin/user/getMenu", "/plugins/biz-suite/api/admin/im/manager/openim/certificate"} {
		request(route, "203.0.113.9", 403)
		request(route, "192.168.10.9", 200)
	}
	for _, override := range []string{"/admin/user/getMenu", "/plugins/biz-suite/api/admin/im/manager/openim/certificate", "/internal/hrm-assets/check"} {
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/suxinweb/", server.GetListenedPort()), nil)
		req.Header.Set("Authorization", opaque)
		req.Header.Set("X-GoSuxin-Client-IP", "203.0.113.9")
		req.Header.Set("X-Url-Path", override)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var body [64]byte
		count, _ := resp.Body.Read(body[:])
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || string(body[:count]) != "public-shell" || len(resp.Cookies()) != 0 {
			t.Fatal("client route override changed the handler or bypassed backoffice policy")
		}
	}
	request("/common/im-sso/directory", "203.0.113.9", 200)
	_, err := store.DB.ExecContext(ctx, `INSERT INTO gf_auth_role(id,rules,btns,status) VALUES(101,'*','',0)`)
	must(t, err)
	_, err = store.DB.ExecContext(ctx, `INSERT INTO gf_auth_role_access(uid,role_id) VALUES(1,101)`)
	must(t, err)
	request("/admin/user/getMenu", "203.0.113.9", 200)
	request("/plugins/biz-suite/api/admin/im/manager/openim/certificate", "203.0.113.9", 200)
	_, err = store.DB.ExecContext(ctx, `UPDATE gf_auth_role SET status=1 WHERE id=101`)
	must(t, err)
	request("/admin/user/getMenu", "203.0.113.9", 403)
	request("/plugins/biz-suite/api/admin/im/manager/openim/certificate", "203.0.113.9", 403)
	_, err = store.DB.ExecContext(ctx, `DROP TABLE biz_openim_access_network`)
	must(t, err)
	request("/admin/user/getMenu", "192.168.10.9", 403)
}
