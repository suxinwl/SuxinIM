package wecom

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
)

func TestConfigurationHTTPWriteOnlyCredentials(t *testing.T) {
	ctx := setupDB(t)
	t.Setenv("WECOM_CREDENTIAL_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{53}, 32)))
	c := &Client{}
	if err := c.LoadSettings(ctx); err != nil {
		t.Fatal(err)
	}
	s := g.Server(guid.S())
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) {
			if r.Header.Get("X-Test-UID") == "1" {
				r.SetCtx(context.WithValue(r.Context(), "uid", int64(1)))
			}
			r.Middleware.Next()
		})
		Bind(ctx, group, c)
	})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	call := func(method, path, body string, authenticated bool) (int, map[string]any) {
		t.Helper()
		req, _ := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d/api/admin/wecom/%s", s.GetListenedPort(), path), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if authenticated {
			req.Header.Set("X-Test-UID", "1")
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"http-contact-private", "http-app-private"} {
			if bytes.Contains(raw, []byte(secret)) {
				t.Fatal("HTTP response leaked credential")
			}
		}
		var out map[string]any
		if res.StatusCode == http.StatusOK && len(raw) > 0 && json.Unmarshal(raw, &out) != nil {
			t.Fatal("invalid HTTP JSON")
		}
		return res.StatusCode, out
	}
	if code, _ := call("POST", "settings", `{"corp_id":"corp"}`, false); code != 401 {
		t.Fatal("anonymous save allowed")
	}
	_, saved := call("POST", "settings", `{"corp_id":"corp","agent_id":"100001","include_subdepartments":true,"contact_secret":"http-contact-private","app_secret":"http-app-private"}`, true)
	if saved["code"] != float64(0) || c.ContactSecret != "http-contact-private" || c.AppSecret != "http-app-private" {
		t.Fatalf("write-only body not applied: code=%v", saved["code"])
	}
	if countRows(t, ctx, "wecom_credentials") != 2 {
		t.Fatal("encrypted credential rows missing")
	}
	_, settings := call("GET", "settings", "", true)
	data := settings["data"].(map[string]any)
	if _, ok := data["contact_secret"]; ok {
		t.Fatal("GET exposes credential field")
	}
	call("POST", "settings", `{"corp_id":"corp","agent_id":"100001","contact_secret":"","app_secret":""}`, true)
	if c.ContactSecret != "http-contact-private" || c.AppSecret != "http-app-private" {
		t.Fatal("blank fields cleared credentials")
	}
	call("POST", "settings", `{"corp_id":"corp","agent_id":"100001","clear_contact_secret":true,"clear_app_secret":true}`, true)
	if c.ContactSecret != "" || c.AppSecret != "" {
		t.Fatal("explicit clear failed")
	}
	for _, kind := range []string{"contact", "app", "checkin", "approval"} {
		_, out := call("POST", "diagnostics/"+kind, `{}`, true)
		if out["code"] != float64(0) || out["data"].(map[string]any)["status"] != "missing" {
			t.Fatalf("missing diagnostic contract: %s", kind)
		}
	}
	if countRows(t, ctx, "wecom_sync_run") != 0 || countRows(t, ctx, "wecom_sync_cursor") != 0 {
		t.Fatal("diagnostics created sync records")
	}
}
