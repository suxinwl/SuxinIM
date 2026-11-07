package runtimeplugin

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	hostauth "github.com/suxinwl/GoSuxin/utility/auth"
	"github.com/suxinwl/GoSuxin/utility/tools/tcache"
)

func TestRuntimeCookieTracksHostExpiryAndCannotReviveLogout(t *testing.T) {
	_ = runtimePluginCSRFSession(t) // Isolates all host keys and token cache prefix.
	hostauth.TokenTimeout, hostauth.MaxRefresh = gvar.New(13800), gvar.New(600)
	key := guid.S()
	token, err := hostauth.GenerateToken(context.Background(), key, map[string]interface{}{"uid": 71})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = hostauth.RemoveToken(context.Background(), key) })
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/plugins/biz-suite", func(group *ghttp.RouterGroup) {
		group.Middleware(runtimePluginAuth)
		group.GET("/api/admin/im/session", func(r *ghttp.Request) { r.Response.Write("validated") })
	})
	if err = server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	request := func() (*http.Response, string) {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/plugins/biz-suite/api/admin/im/session", server.GetListenedPort()), nil)
		req.AddCookie(&http.Cookie{Name: runtimePluginCookieName("biz-suite"), Value: token})
		req.Header.Set("X-Forwarded-Proto", "https")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return resp, string(body)
	}
	assertCookieExpiry := func(expected time.Time, minimumAge int) {
		t.Helper()
		resp, body := request()
		if resp.StatusCode != 200 || body != "validated" {
			t.Fatalf("session not authenticated: status %d", resp.StatusCode)
		}
		for _, cookie := range resp.Cookies() {
			if cookie.Name != runtimePluginCookieName("biz-suite") {
				continue
			}
			if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/plugins/biz-suite/" || cookie.Value != token {
				t.Fatal("plugin cookie lost its security attributes or changed host token")
			}
			if cookie.MaxAge < minimumAge || cookie.MaxAge > 14400 || cookie.Expires.Sub(expected).Abs() > 2*time.Second {
				t.Fatalf("cookie expiry does not track host session: age=%d", cookie.MaxAge)
			}
			return
		}
		t.Fatal("missing plugin cookie")
	}
	data, _, _ := hostauth.GetTokenData(context.Background(), token)
	claims, _ := hostauth.JwtParseToken(data.JwtToken)
	assertCookieExpiry(claims.ExpiresAt.Time, 14398)
	setExpiry := func(expiry time.Time) {
		t.Helper()
		claims.ExpiresAt = jwt.NewNumericDate(expiry)
		data.JwtToken, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(hostauth.SecretKey.String()))
		if err != nil {
			t.Fatal(err)
		}
		if err = tcache.SetCache(hostauth.TokenCacheKey.String()+key, data, 4*time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	// Outside the last 10 minutes the cookie follows the remaining time, not a
	// fresh four-hour expiry independent of the server.
	setExpiry(time.Now().Add(2 * time.Hour))
	assertCookieExpiry(claims.ExpiresAt.Time, 7198)
	setExpiry(time.Now().Add(5 * time.Minute))
	assertCookieExpiry(time.Now().Add(4*time.Hour), 14398)
	// A server-expired session is rejected even if the browser retains a cookie.
	setExpiry(time.Now().Add(-time.Minute))
	if resp, _ := request(); resp.StatusCode != http.StatusUnauthorized {
		t.Fatal("expired host session was revived")
	}
	setExpiry(time.Now().Add(time.Hour))
	if err = hostauth.RemoveToken(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if resp, _ := request(); resp.StatusCode != http.StatusUnauthorized {
		t.Fatal("logged-out host session was revived")
	}
}
