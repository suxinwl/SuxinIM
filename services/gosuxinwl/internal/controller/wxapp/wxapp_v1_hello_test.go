package wxapp

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
	"github.com/suxinwl/GoSuxin/internal/extend/middleware"
	"github.com/suxinwl/GoSuxin/utility/auth"
)

func TestScaffoldHelloReturns404WithoutIssuingOrReplacingHostToken(t *testing.T) {
	secret, encrypt, dynamic := auth.SecretKey, auth.EncryptKey, auth.DynamicToken
	timeout, refresh, multi, prefix := auth.TokenTimeout, auth.MaxRefresh, auth.MultiLogin, auth.TokenCacheKey
	auth.SecretKey, auth.EncryptKey, auth.DynamicToken = gvar.New("disabled-scaffold-test-secret"), gvar.New("0123456789abcdef0123456789abcdef"), gvar.New("")
	auth.TokenTimeout, auth.MaxRefresh, auth.MultiLogin, auth.TokenCacheKey = gvar.New(600), gvar.New(0), gvar.New(false), gvar.New("scaffold-test-"+guid.S())
	t.Cleanup(func() {
		_ = auth.RemoveToken(context.Background(), "123")
		auth.SecretKey, auth.EncryptKey, auth.DynamicToken = secret, encrypt, dynamic
		auth.TokenTimeout, auth.MaxRefresh, auth.MultiLogin, auth.TokenCacheKey = timeout, refresh, multi, prefix
	})
	opaque, err := auth.GenerateToken(context.Background(), "123", map[string]interface{}{"uid": 41})
	if err != nil {
		t.Fatal(err)
	}
	before, _, err := auth.GetTokenData(context.Background(), opaque)
	if err != nil {
		t.Fatal(err)
	}
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.HandlerResponse)
		R.BindController(context.Background(), group)
	})
	if err = server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	response, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/wxapp/hello", server.GetListenedPort()))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusNotFound || len(response.Cookies()) != 0 || strings.Contains(strings.ToLower(string(body)), "token") {
		t.Fatal("scaffold endpoint exposed a successful or credential-bearing response")
	}
	after, _, err := auth.GetTokenData(context.Background(), opaque)
	if err != nil || after.UuId != before.UuId || after.JwtToken != before.JwtToken {
		t.Fatal("scaffold endpoint generated or replaced the former fixture-key host session")
	}
}
