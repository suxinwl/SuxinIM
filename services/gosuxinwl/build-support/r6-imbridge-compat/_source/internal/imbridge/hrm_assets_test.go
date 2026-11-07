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

func TestHRMAssetsRequireCurrentHostSessionAndSourcePolicyWithoutRefresh(t *testing.T) {
	t.Setenv("GOSUXIN_HRM_PUBLIC_ADMINS_ONLY", "true")
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	opaque := hrmBackofficeTestSession(t, 1)
	hostauth.MaxRefresh = gvar.New(900)
	before, _, err := hostauth.GetTokenData(context.Background(), opaque)
	must(t, err)
	if !hostauth.IsRefresh(before.JwtToken) {
		t.Fatal("fixture must exercise a session inside the refresh window")
	}
	svc := &Service{Store: store}
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) {
			if r.Header.Get("X-Test-Nonloopback") == "1" {
				r.RemoteAddr = "203.0.113.9:32100"
			}
			r.Middleware.Next()
		})
		group.GET("/internal/hrm-assets/check", svc.hrmAssetCheck)
		group.GET("/internal/hrm-backoffice/check", svc.hrmBackofficeToolCheck)
	})
	if err = server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	request := func(ip, uri, cookie, authorization string, untrusted bool, want int, endpoints ...string) {
		t.Helper()
		endpoint := "/internal/hrm-assets/check"
		if len(endpoints) != 0 {
			endpoint = endpoints[0]
		}
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s", server.GetListenedPort(), endpoint), nil)
		req.Header.Set("X-GoSuxin-Client-IP", ip)
		req.Header.Set("X-Original-URI", uri)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: hrmAssetCookieName, Value: cookie})
		}
		if authorization != "" {
			req.Header.Set("X-Original-Authorization", authorization)
		}
		if untrusted {
			req.Header.Set("X-Test-Nonloopback", "1")
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != want || len(response.Cookies()) != 0 {
			t.Fatalf("asset authorization status=%d want=%d cookies=%d", response.StatusCode, want, len(response.Cookies()))
		}
	}
	asset := "/resource/uploads/hrm/photo.png?v=1"
	request("192.168.10.9", asset, opaque, "", false, 204)
	request("192.168.10.9", asset, "", opaque, false, 204)
	request("203.0.113.9", asset, opaque, "", false, 403)
	request("192.168.10.9", asset, "", "", false, 401)
	request("192.168.10.9", asset, "invalid", "", false, 401)
	request("192.168.10.9", asset, opaque, "invalid", false, 401)
	request("192.168.10.9", "/resource/uploads/../private/key.pem", opaque, "", false, 403)
	request("192.168.10.9", "/resource/uploads/%252e%252e/private/key.pem", opaque, "", false, 403)
	request("192.168.10.9", "/resource/uploads/hrm%2fphoto.png", opaque, "", false, 403)
	request("192.168.10.9", "/internal/im-access/check", opaque, "", false, 403)
	request("192.168.10.9", asset, opaque, "", true, 403)
	request("", asset, opaque, "", false, 403)
	tool := "/common/basetool/getDicData?type=department"
	request("192.168.10.9", tool, "", "Bearer "+opaque, false, 204, "/internal/hrm-backoffice/check")
	request("203.0.113.9", tool, "", "Bearer "+opaque, false, 403, "/internal/hrm-backoffice/check")
	request("192.168.10.9", tool, "", "", false, 401, "/internal/hrm-backoffice/check")
	request("192.168.10.9", tool, "", "Bearer "+opaque, true, 403, "/internal/hrm-backoffice/check")
	request("192.168.10.9", "/common/basetool/table/getTables", "", "Bearer "+opaque, false, 403, "/internal/hrm-backoffice/check")
	after, _, err := hostauth.GetTokenData(context.Background(), opaque)
	must(t, err)
	if after.JwtToken != before.JwtToken || after.UuId != before.UuId {
		t.Fatal("asset checks refreshed or replaced the HRM session")
	}
	_, err = store.DB.ExecContext(ctx, `INSERT INTO gf_auth_role(id,rules,btns,status) VALUES(101,'*','',0)`)
	must(t, err)
	_, err = store.DB.ExecContext(ctx, `INSERT INTO gf_auth_role_access(uid,role_id) VALUES(1,101)`)
	must(t, err)
	request("203.0.113.9", asset, opaque, "", false, 204)
	request("203.0.113.9", tool, "", "Bearer "+opaque, false, 204, "/internal/hrm-backoffice/check")
	_, err = store.DB.ExecContext(ctx, `UPDATE gf_auth_role SET status=1 WHERE id=101`)
	must(t, err)
	request("203.0.113.9", asset, opaque, "", false, 403)
	request("203.0.113.9", tool, "", "Bearer "+opaque, false, 403, "/internal/hrm-backoffice/check")
	_, err = store.DB.ExecContext(ctx, `UPDATE gf_admin SET status=1 WHERE id=1`)
	must(t, err)
	request("192.168.10.9", asset, opaque, "", false, 403)
	_, err = store.DB.ExecContext(ctx, `UPDATE gf_admin SET status=0 WHERE id=1`)
	must(t, err)
	_, err = store.DB.ExecContext(ctx, `DROP TABLE biz_openim_access_network`)
	must(t, err)
	request("192.168.10.9", asset, opaque, "", false, 503)
	request("192.168.10.9", tool, "", "Bearer "+opaque, false, 503, "/internal/hrm-backoffice/check")
	must(t, hostauth.RemoveToken(context.Background(), afterKey(t, opaque)))
	request("192.168.10.9", asset, opaque, "", false, 401)
}

func TestHRMBackofficeToolsRequireExactRoutes(t *testing.T) {
	for _, uri := range []string{"/common/basetool/getDicData", "/common/basetool/getDicData?type=department", "/common/basetool/checkHaseRule"} {
		if !validHRMBackofficeToolURI(uri) {
			t.Errorf("required tool route rejected: %s", uri)
		}
	}
	for _, uri := range []string{"", "/common/basetool/getDicData/", "//common/basetool/getDicData", "/common/basetool/%67etDicData", "/common/basetool/./getDicData", "/common/basetool/table/getTables", "/common/basetool/table/weigh", "/resource/uploads/a.png", "https://kf.szputy.com:9880/common/basetool/getDicData"} {
		if validHRMBackofficeToolURI(uri) {
			t.Errorf("nonexact tool route accepted: %s", uri)
		}
	}
}

func afterKey(t *testing.T, opaque string) string {
	t.Helper()
	_, key, err := hostauth.GetTokenData(context.Background(), opaque)
	must(t, err)
	return key
}

func TestHRMAssetURIAcceptsOnlyCanonicalUploadPaths(t *testing.T) {
	for _, uri := range []string{"/resource/uploads/a.png", "/resource/uploads/hrm/%E5%91%98%E5%B7%A5%20%E5%A4%B4%E5%83%8F.png?v=1"} {
		if !validHRMAssetURI(uri) {
			t.Errorf("valid upload path rejected: %s", uri)
		}
	}
	for _, uri := range []string{"", "/resource/uploads/", "/resource/uploads", "//resource/uploads/a.png", "/resource/uploads//a.png", "/resource/uploads/./a.png", "/resource/uploads/a/../b.png", "/resource/uploads/%2e%2e/a.png", "/resource/uploads/%252e%252e/a.png", "/resource/uploads/a%2fb.png", "/resource/uploads/a%5cb.png", "/resource/uploads/a%00b.png", "/resource/uploads/a\\b.png", "https://other.invalid/resource/uploads/a.png", "/other/a.png"} {
		if validHRMAssetURI(uri) {
			t.Errorf("unsafe upload path accepted: %s", uri)
		}
	}
}
