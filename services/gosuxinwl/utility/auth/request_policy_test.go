package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
)

func TestTokenRequestPolicyRejectsBeforeRefreshAndResponseCookies(t *testing.T) {
	key, opaque, cached := testSession(t)
	dynamic := DynamicToken
	DynamicToken = gvar.New("")
	t.Cleanup(func() { DynamicToken = dynamic })
	changeTestExpiry(t, key, cached, time.Now().Add(5*time.Minute))
	original := cached.JwtToken
	denied := errors.New("request policy denied")
	checks := 0
	cleanup := SetTokenRequestPolicy(func(_ *ghttp.Request, claims *CustomClaims) error {
		checks++
		if claims.Data.(map[string]interface{})["uid"] != float64(71) {
			t.Error("policy did not receive verified identity")
		}
		return denied
	})
	t.Cleanup(cleanup)
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.BindHandler("/admin/probe", func(r *ghttp.Request) {
		if _, err := ParseToken(r); err != nil {
			r.Response.WriteStatus(http.StatusForbidden)
			return
		}
		r.Response.Write("authorized")
	})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	request, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/admin/probe", server.GetListenedPort()), nil)
	request.Header.Set("Authorization", opaque)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusForbidden || checks != 1 || len(response.Cookies()) != 0 || string(body) == "authorized" {
		t.Fatalf("rejected request status=%d policyChecks=%d cookies=%d authorizedBody=%t readError=%v", response.StatusCode, checks, len(response.Cookies()), string(body) == "authorized", err)
	}
	after, _, err := GetTokenData(context.Background(), opaque)
	if err != nil || after.JwtToken != original {
		t.Fatal("rejected request refreshed the cached session")
	}
	cleanup()
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	after, _, err = GetTokenData(context.Background(), opaque)
	if err != nil || response.StatusCode != http.StatusOK || after.JwtToken == original {
		t.Fatal("removing the deployment policy failed to restore normal refresh")
	}
}
