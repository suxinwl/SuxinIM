package runtimeplugin

import (
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	hostauth "github.com/suxinwl/GoSuxin/utility/auth"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

const (
	runtimePluginCookiePrefix  = "suxin_plugin_"
	runtimePluginCookieSuffix  = "_admin"
	runtimePluginBootstrapPath = "/__host_auth/bootstrap"
)

// runtimePluginRoute identifies only routes owned by the runtime plugin
// authentication groups. The plugin name is checked against the registry's
// package-name grammar before it is used in a cookie or redirect.
func runtimePluginRoute(path string) (plugin string, api bool, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "plugins" && pluginNamePattern.MatchString(parts[1]) && parts[2] == "admin" {
		return parts[1], false, true
	}
	if len(parts) >= 4 && parts[0] == "plugins" && pluginNamePattern.MatchString(parts[1]) && parts[2] == "api" && parts[3] == "admin" {
		return parts[1], true, true
	}
	return "", false, false
}

func runtimePluginCookieName(plugin string) string {
	return runtimePluginCookiePrefix + plugin + runtimePluginCookieSuffix
}

func runtimePluginCookiePath(plugin string) string {
	return "/plugins/" + plugin + "/"
}

func runtimePluginShellPath(requestPath string) bool {
	_, api, ok := runtimePluginRoute(requestPath)
	if !ok || api {
		return false
	}
	shell := "/plugins/" + strings.Split(strings.Trim(requestPath, "/"), "/")[1] + "/admin"
	suffix := strings.TrimPrefix(strings.TrimRight(requestPath, "/"), shell)
	// Extensionless admin paths are SPA deep links; assets and auth endpoints
	// retain a 401 response instead of receiving a login HTML page.
	return suffix == "" || (!strings.HasPrefix(suffix, "/__host_auth") && path.Ext(suffix) == "")
}

func runtimePluginSecureRequest(r *ghttp.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func setRuntimePluginCookie(r *ghttp.Request, plugin, token string, expiresAt time.Time) {
	remaining := time.Until(expiresAt)
	if token == "" || remaining <= 0 {
		return
	}
	r.Cookie.SetHttpCookie(&http.Cookie{
		Name:     runtimePluginCookieName(plugin),
		Value:    token,
		Path:     runtimePluginCookiePath(plugin),
		Expires:  expiresAt,
		MaxAge:   int((remaining + time.Second - 1) / time.Second),
		HttpOnly: true,
		Secure:   runtimePluginSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func expireRuntimePluginCookie(r *ghttp.Request, plugin string) {
	r.Cookie.SetHttpCookie(&http.Cookie{
		Name:     runtimePluginCookieName(plugin),
		Value:    "",
		Path:     runtimePluginCookiePath(plugin),
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   runtimePluginSecureRequest(r),
		SameSite: http.SameSiteLaxMode,
	})
}

func runtimePluginCookieValue(r *ghttp.Request, plugin string) string {
	if cookie := r.Cookie.Get(runtimePluginCookieName(plugin)); cookie != nil {
		return strings.TrimSpace(cookie.String())
	}
	return ""
}

func runtimePluginRedirectTarget(r *ghttp.Request) string {
	if r.RequestURI != "" {
		if parsed, err := url.ParseRequestURI(r.RequestURI); err == nil && parsed.Path != "" && strings.HasPrefix(parsed.Path, "/plugins/") {
			target := parsed.EscapedPath()
			if parsed.RawQuery != "" {
				target += "?" + parsed.RawQuery
			}
			return target
		}
	}
	path := r.URL.EscapedPath()
	if path == "" {
		path = r.URL.Path
	}
	if path == "" {
		path = "/"
	}
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	return path
}

func runtimePluginRedirect(r *ghttp.Request) {
	target := runtimePluginRedirectTarget(r)
	r.Response.RedirectTo("/suxinweb/login?redirect="+url.QueryEscape(target), http.StatusFound)
}

func runtimePluginUnauthorized(r *ghttp.Request) {
	r.Response.WriteHeader(http.StatusUnauthorized)
	r.Response.WriteJson(gf.Failed().SetCode(hostauth.JwtTokenInvalid).SetMsg("宿主登录已失效，请返回 GoSuxin 后台重新登录"))
	r.ExitAll()
}

func runtimePluginSameOrigin(r *ghttp.Request, value string, originOnly bool) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" || parsed.Fragment != "" {
		return false
	}
	if originOnly && (parsed.Path != "" || parsed.RawQuery != "") {
		return false
	}
	scheme := "http"
	if runtimePluginSecureRequest(r) {
		scheme = "https"
	}
	if parsed.Scheme != scheme {
		return false
	}
	// Host belongs to the request's actual public origin. A client-supplied
	// X-Forwarded-Host must never make a cross-origin request look same-origin.
	requestOrigin, err := url.Parse(scheme + "://" + r.Host)
	if err != nil || requestOrigin.Hostname() == "" || requestOrigin.User != nil {
		return false
	}
	port := func(u *url.URL) string {
		if value := u.Port(); value != "" {
			return value
		}
		if u.Scheme == "https" {
			return "443"
		}
		return "80"
	}
	return strings.EqualFold(parsed.Hostname(), requestOrigin.Hostname()) && port(parsed) == port(requestOrigin)
}

func runtimePluginCookieWriteAllowed(r *ghttp.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Requested-With")), "XMLHttpRequest") {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" && !runtimePluginSameOrigin(r, origin, true) {
		return false
	}
	if referer := r.Header.Get("Referer"); referer != "" && !runtimePluginSameOrigin(r, referer, false) {
		return false
	}
	return true
}

// runtimePluginAuth accepts the host token only long enough to exchange it for
// an HttpOnly cookie with the same expiry as the host session. Once present,
// the cookie is made authoritative
// for the request and is still checked through the host token cache on every
// request. A client-supplied plugin identity header is discarded here and again
// by the reverse proxy before the request reaches the plugin.
func runtimePluginAuth(r *ghttp.Request) {
	plugin, api, ok := runtimePluginRoute(r.URL.Path)
	if !ok {
		r.Middleware.Next()
		return
	}
	r.Header.Del("X-Suxin-Plugin-User")

	originalAuthorization := r.Header.Get("Authorization")
	authenticated := false
	// Prefer a current host token whenever the browser supplied one. This lets a
	// user switch GoSuxin accounts without being pinned to an older plugin
	// cookie for the remainder of its lifetime.
	if originalAuthorization != "" {
		r.Header.Set("Authorization", originalAuthorization)
		if claims, err := hostauth.ParseToken(r); err == nil {
			setRuntimePluginCookie(r, plugin, hostauth.GetRequestToken(r), claims.ExpiresAt.Time)
			authenticated = true
		}
	}
	if !authenticated {
		if cookieToken := runtimePluginCookieValue(r, plugin); cookieToken != "" {
			// Check cookie writes before ParseToken: successful parsing may refresh
			// the host session or issue scoped resource cookies through its policy.
			if !runtimePluginCookieWriteAllowed(r) {
				r.Header.Del("Authorization")
				r.Response.WriteHeader(http.StatusForbidden)
				r.Response.WriteJson(gf.Failed().SetCode(http.StatusForbidden).SetMsg("插件管理请求来源验证失败"))
				r.ExitAll()
				return
			}
			r.Header.Set("Authorization", cookieToken)
			if claims, err := hostauth.ParseToken(r); err == nil {
				setRuntimePluginCookie(r, plugin, hostauth.GetRequestToken(r), claims.ExpiresAt.Time)
				authenticated = true
			} else {
				expireRuntimePluginCookie(r, plugin)
			}
		}
	}
	if !authenticated {
		g.Log().Warningf(r.Context(), "plugin authentication rejected path=%s %s", r.URL.Path, hostauth.RequestTimingDiagnostic(r))
		r.Header.Del("Authorization")
	}

	if !authenticated {
		if !api && (r.Method == http.MethodGet || r.Method == http.MethodHead) && runtimePluginShellPath(r.URL.Path) {
			runtimePluginRedirect(r)
			return
		}
		runtimePluginUnauthorized(r)
		return
	}
	r.Middleware.Next()
}

// runtimePluginBootstrap is intentionally small. The preceding middleware has
// performed the token exchange and the normal host middleware that follows it
// performs the complete RBAC check before this response is returned.
func runtimePluginBootstrap(r *ghttp.Request) {
	plugin, _, ok := runtimePluginRoute(r.URL.Path)
	if !ok || !strings.HasSuffix(strings.TrimRight(r.URL.Path, "/"), runtimePluginBootstrapPath) {
		r.Response.WriteStatus(http.StatusNotFound)
		r.ExitAll()
		return
	}
	r.Response.WriteJson(gf.Success().SetData(gf.Map{"authenticated": true, "plugin": plugin}))
	r.ExitAll()
}
