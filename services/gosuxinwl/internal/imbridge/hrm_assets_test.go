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
	epoch, err := store.Epoch(ctx, 1)
	must(t, err)
	opaque := hrmBackofficeTestSession(t, 1, map[string]interface{}{"uid": int64(1), "hrmAssetEpoch": epoch, "hrmAssetCredential": digest("preserved-password:original-salt")})
	_, err = store.DB.ExecContext(ctx, `UPDATE gf_admin SET avatar='/resource/uploads/hrm/photo.png' WHERE id=1`)
	must(t, err)
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
		setTestTrustedEntry(req, ip)
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
	for _, uri := range []string{"", "/common/basetool/getDicData/", "//common/basetool/getDicData", "/common/basetool/%67etDicData", "/common/basetool/./getDicData", "/common/basetool/table/getTables", "/common/basetool/table/weigh", "/resource/uploads/a.png", "https://hr.szputy.com:9880/common/basetool/getDicData"} {
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

func TestHRMUploadsOrdinaryStaffBothEntrancesAndSecurityRevocation(t *testing.T) {
	t.Setenv("GOSUXIN_HRM_PUBLIC_ADMINS_ONLY", "false")
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	for _, statement := range []string{
		`ALTER TABLE gf_auth_rule ADD COLUMN path TEXT`,
		`CREATE TABLE gf_attachment(id INTEGER PRIMARY KEY,location TEXT,url TEXT,cover_url TEXT)`,
		`INSERT INTO gf_attachment VALUES(1,'local','/resource/uploads/hrm/document.pdf','')`,
		`UPDATE gf_admin SET avatar='/resource/uploads/own.png' WHERE id=1`,
		`INSERT INTO gf_auth_rule(id,type,permission,status,path) VALUES(101,2,'',0,'/admin/datacenter/attachment/getMyFiles'),(102,2,'biz-suite.hrm.manage',0,'')`,
		`INSERT INTO gf_auth_role(id,rules,btns,status) VALUES(101,'','101',0),(102,'','102',0)`,
	} {
		_, err := store.DB.ExecContext(ctx, statement)
		must(t, err)
	}
	setAvatarProjection(t, store, "https://hr.szputy.com:9880/resource/uploads/directory.png")
	epoch, err := store.Epoch(ctx, 1)
	must(t, err)
	claims := map[string]interface{}{"uid": int64(1), "hrmAssetEpoch": epoch, "hrmAssetCredential": digest("preserved-password:original-salt")}
	opaque := hrmBackofficeTestSession(t, 1, claims)
	service := &Service{Store: store}
	previousLive := live.Swap(service)
	t.Cleanup(func() { live.Store(previousLive) })
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.BindHandler("GET:/internal/hrm-assets/check", service.hrmAssetCheck)
	server.BindHandler("GET:/resource/uploads/*path", func(r *ghttp.Request) {
		if !HRMAssetRequestAllowed(r) {
			r.Response.WriteHeader(http.StatusForbidden)
			return
		}
		r.Response.WriteHeader(http.StatusNoContent)
	})
	must(t, server.Start())
	t.Cleanup(func() { _ = server.Shutdown() })
	request := func(ip, resource, credential string, want int) {
		t.Helper()
		for _, endpoint := range []string{"/internal/hrm-assets/check", resource} {
			req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s", server.GetListenedPort(), endpoint), nil)
			setTestTrustedEntry(req, ip)
			req.Header.Set("X-Original-URI", resource)
			if credential != "" {
				req.AddCookie(&http.Cookie{Name: hrmAssetCookieName, Value: credential})
			}
			response, err := http.DefaultClient.Do(req)
			must(t, err)
			response.Body.Close()
			expected := want
			if endpoint != "/internal/hrm-assets/check" && want == 401 {
				expected = 403
			}
			if response.StatusCode != expected || len(response.Cookies()) != 0 {
				t.Fatalf("resource route %s status=%d want=%d", endpoint, response.StatusCode, expected)
			}
		}
	}
	for _, source := range []string{"192.168.10.9", "203.0.113.9"} {
		request(source, "/resource/uploads/own.png", opaque, 204)
		request(source, "/resource/uploads/directory.png", opaque, 204)
		request(source, "/resource/uploads/hrm/document.pdf", opaque, 403)
		request(source, "/resource/uploads/hrm/document.pdf", "", 401)
	}
	_, err = store.DB.ExecContext(ctx, `INSERT INTO gf_auth_role_access(uid,role_id) VALUES(1,101)`)
	must(t, err)
	request("203.0.113.9", "/resource/uploads/hrm/document.pdf", opaque, 204)
	request("203.0.113.9", "/resource/uploads/unregistered-secret.pdf", opaque, 403)
	_, err = store.DB.ExecContext(ctx, `UPDATE gf_auth_rule SET status=1 WHERE id=101`)
	must(t, err)
	request("203.0.113.9", "/resource/uploads/hrm/document.pdf", opaque, 403)
	_, err = store.DB.ExecContext(ctx, `INSERT INTO gf_auth_role_access(uid,role_id) VALUES(1,102)`)
	must(t, err)
	request("203.0.113.9", "/resource/uploads/hrm/document.pdf", opaque, 204)
	for _, statement := range []string{`UPDATE gf_admin SET status=1 WHERE id=1`, `UPDATE gf_admin SET status=0,password='changed' WHERE id=1`} {
		_, err = store.DB.ExecContext(ctx, statement)
		must(t, err)
		request("192.168.10.9", "/resource/uploads/own.png", opaque, 403)
	}
	_, err = store.DB.ExecContext(ctx, `UPDATE gf_admin SET password='preserved-password' WHERE id=1`)
	must(t, err)
	request("203.0.113.9", "/resource/uploads/own.png", opaque, 204)
	_, err = store.BumpEpoch(ctx, 1)
	must(t, err)
	request("192.168.10.9", "/resource/uploads/own.png", opaque, 403)
	must(t, hostauth.RemoveToken(ctx, afterKey(t, opaque)))
	request("192.168.10.9", "/resource/uploads/own.png", opaque, 401)
}
