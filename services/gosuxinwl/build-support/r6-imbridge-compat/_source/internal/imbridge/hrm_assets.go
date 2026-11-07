package imbridge

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strings"
	"time"
	"unicode"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
	hostauth "github.com/suxinwl/GoSuxin/utility/auth"
)

const hrmAssetCookieName = "GoSuxinHRMAsset"

func setHRMAssetCookie(r *ghttp.Request, opaque string, claims *hostauth.CustomClaims, clear bool) {
	if !HRMPublicAdminsOnly() || r == nil {
		return
	}
	cookie := &http.Cookie{Name: hrmAssetCookieName, Value: opaque, Path: "/resource/uploads/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}
	if clear {
		cookie.Value, cookie.MaxAge, cookie.Expires = "", -1, time.Unix(1, 0)
	} else {
		if opaque == "" || claims == nil || claims.ExpiresAt == nil {
			return
		}
		remaining := time.Until(claims.ExpiresAt.Time)
		if remaining <= 0 {
			return
		}
		cookie.Expires = claims.ExpiresAt.Time
		cookie.MaxAge = int((remaining + time.Second - 1) / time.Second)
	}
	r.Cookie.SetHttpCookie(cookie)
}

// SetHRMAssetLoginCookie gives authenticated HRM pages access to their uploads
// without putting a bearer token in image URLs or JavaScript-readable storage.
func SetHRMAssetLoginCookie(ctx context.Context, opaque string) {
	if !HRMPublicAdminsOnly() {
		return
	}
	data, _, err := hostauth.GetTokenData(ctx, opaque)
	if err != nil {
		return
	}
	claims, err := hostauth.JwtParseToken(data.JwtToken)
	if err == nil {
		setHRMAssetCookie(g.RequestFromCtx(ctx), opaque, claims, false)
	}
}

func ClearHRMAssetCookie(ctx context.Context) {
	setHRMAssetCookie(g.RequestFromCtx(ctx), "", nil, true)
}

func validHRMAssetURI(raw string) bool {
	if raw == "" || len(raw) > 8192 || strings.ContainsAny(raw, "\x00\r\n\\") {
		return false
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil || u.IsAbs() || u.Host != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/resource/uploads/") || u.Path == "/resource/uploads/" {
		return false
	}
	if path.Clean(u.Path) != u.Path || strings.ContainsAny(u.Path, "\x00\r\n\\%") || strings.IndexFunc(u.Path, unicode.IsControl) >= 0 {
		return false
	}
	encoded := strings.ToLower(u.EscapedPath())
	for _, forbidden := range []string{"%2f", "%5c", "%2e", "%00"} {
		if strings.Contains(encoded, forbidden) {
			return false
		}
	}
	return true
}

func hrmAssetAuthorization(r *ghttp.Request) string {
	values := r.Header.Values("X-Original-Authorization")
	if len(values) > 1 {
		return ""
	}
	if len(values) == 1 && strings.TrimSpace(values[0]) != "" {
		return values[0]
	}
	credential := ""
	count := 0
	for _, cookie := range r.Request.Cookies() {
		if cookie.Name == hrmAssetCookieName {
			count++
			if count > 1 {
				return ""
			}
			credential = cookie.Value
		}
	}
	return credential
}

func (s *Service) hrmAssetCheck(r *ghttp.Request) {
	s.hrmResourceCheck(r, validHRMAssetURI)
}

func validHRMBackofficeToolURI(raw string) bool {
	if raw == "" || len(raw) > 8192 || strings.ContainsAny(raw, "\x00\r\n\\") {
		return false
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil || u.IsAbs() || u.Host != "" || u.Fragment != "" || u.EscapedPath() != u.Path {
		return false
	}
	return u.Path == "/common/basetool/getDicData" || u.Path == "/common/basetool/checkHaseRule"
}

func (s *Service) hrmBackofficeToolCheck(r *ghttp.Request) {
	s.hrmResourceCheck(r, validHRMBackofficeToolURI)
}

func (s *Service) hrmResourceCheck(r *ghttp.Request, validURI func(string) bool) {
	status := http.StatusForbidden
	defer func() { r.Response.Header().Set("Cache-Control", "no-store"); r.Response.WriteHeader(status) }()
	if r.Method != http.MethodGet || !isAccessLoopback(r.Request) || r.URL.RawQuery != "" {
		return
	}
	if len(r.Header.Values("X-Original-URI")) != 1 || !validURI(r.Header.Get("X-Original-URI")) || len(r.Header.Values("X-GoSuxin-Client-IP")) != 1 {
		return
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("X-GoSuxin-Client-IP")))
	if err != nil || ip.Zone() != "" {
		return
	}
	authorization := hrmAssetAuthorization(r)
	if authorization == "" {
		status = http.StatusUnauthorized
		return
	}
	originalAuthorization := r.Header.Get("Authorization")
	r.Header.Set("Authorization", authorization)
	claims, err := hostauth.ParseTokenReadOnly(r)
	r.Header.Set("Authorization", originalAuthorization)
	if err != nil {
		status = http.StatusUnauthorized
		return
	}
	data, _ := claims.Data.(map[string]interface{})
	uid := gconv.Int64(data["uid"])
	ctx := WithClientIP(r.Context(), ip.Unmap().String())
	if err = s.Store.checkActiveHRMAccount(ctx, uid); err == nil {
		err = s.Store.CheckHRMBackofficeAccess(ctx, uid)
	}
	if errors.Is(err, ErrHRMPublicAdminRequired) {
		return
	}
	if err != nil {
		status = http.StatusServiceUnavailable
		return
	}
	status = http.StatusNoContent
}

func bindHRMResourceChecks(server *ghttp.Server, s *Service) {
	server.BindHandler("GET:/internal/hrm-assets/check", s.hrmAssetCheck)
	server.BindHandler("GET:/internal/hrm-backoffice/check", s.hrmBackofficeToolCheck)
}
