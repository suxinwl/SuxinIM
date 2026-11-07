package imbridge

import (
	"context"
	"database/sql"
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
	if r == nil {
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

// HRMAssetRequestAllowed protects the host's own static route as well as the
// Nginx auth_request route. The enterprise private filesystem alias is separate.
func HRMAssetRequestAllowed(r *ghttp.Request) bool {
	service := live.Load()
	if service == nil {
		return !ConfigFromEnv().Enabled
	}
	if (r.Method != http.MethodGet && r.Method != http.MethodHead) || !validHRMAssetURI(r.RequestURI) || len(r.Header.Values("Authorization")) > 1 {
		return false
	}
	credential := r.Header.Get("Authorization")
	if credential == "" {
		count := 0
		for _, cookie := range r.Request.Cookies() {
			if cookie.Name == hrmAssetCookieName {
				credential, count = cookie.Value, count+1
			}
		}
		if count != 1 {
			return false
		}
	}
	original := r.Header.Get("Authorization")
	r.Header.Set("Authorization", credential)
	claims, err := hostauth.ParseTokenReadOnly(r)
	r.Header.Set("Authorization", original)
	if err != nil {
		return false
	}
	data, _ := claims.Data.(map[string]interface{})
	uid := gconv.Int64(data["uid"])
	ctx := requestAccessContext(r.Context(), r.Request)
	if service.Store.checkActiveHRMAccount(ctx, uid) != nil || service.Store.checkHRMAssetSession(ctx, uid, data) != nil || service.Store.checkHRMAssetPermission(ctx, uid, r.RequestURI) != nil {
		return false
	}
	return !HRMPublicAdminsOnly() || service.Store.CheckHRMBackofficeAccess(ctx, uid) == nil
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
	s.hrmResourceCheck(r, validHRMAssetURI, true)
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
	s.hrmResourceCheck(r, validHRMBackofficeToolURI, false)
}

func (s *Service) hrmResourceCheck(r *ghttp.Request, validURI func(string) bool, asset bool) {
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
	ctx := requestAccessContext(r.Context(), r.Request)
	if err = s.Store.checkActiveHRMAccount(ctx, uid); err == nil {
		if asset {
			err = s.Store.checkHRMAssetSession(ctx, uid, data)
			if err == nil {
				err = s.Store.checkHRMAssetPermission(ctx, uid, r.Header.Get("X-Original-URI"))
			}
		}
		if err == nil && HRMPublicAdminsOnly() {
			err = s.Store.CheckHRMBackofficeAccess(ctx, uid)
		}
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

// HRMAssetLoginClaims binds uploads to the security state at password login.
// Refresh preserves these signed values; stale sessions cannot adopt a new epoch.
func HRMAssetLoginClaims(ctx context.Context, uid int64, username, password, salt string) (map[string]interface{}, error) {
	data := map[string]interface{}{"uid": uid, "username": username}
	var store *SQLStore
	if service := live.Load(); service != nil {
		store = service.Store
	} else if HRMPublicAdminsOnly() || ConfigFromEnv().Enabled {
		db, err := g.DB().Master()
		if err != nil {
			return nil, err
		}
		store, err = NewSQLStore(db, g.DB().GetConfig().Prefix)
		if err != nil {
			return nil, err
		}
	}
	if store != nil {
		epoch, err := store.Epoch(ctx, uid)
		if err != nil {
			return nil, err
		}
		data["hrmAssetEpoch"] = epoch
		data["hrmAssetCredential"] = digest(password + ":" + salt)
	}
	return data, nil
}

func (s *SQLStore) checkHRMAssetSession(ctx context.Context, uid int64, data map[string]interface{}) error {
	var password, salt string
	err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(password,''),COALESCE(salt,'') FROM `+s.Prefix+`admin WHERE id=?`, uid).Scan(&password, &salt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrHRMPublicAdminRequired
	}
	if err != nil {
		return err
	}
	var epoch int64
	err = s.DB.QueryRowContext(ctx, `SELECT epoch FROM biz_openim_epoch WHERE uid=?`, uid).Scan(&epoch)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrHRMPublicAdminRequired
	}
	if err != nil {
		return err
	}
	if epoch <= 0 || gconv.Int64(data["hrmAssetEpoch"]) != epoch || gconv.String(data["hrmAssetCredential"]) != digest(password+":"+salt) {
		return ErrHRMPublicAdminRequired
	}
	return nil
}

func trustedHRMUploadPath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" {
		return ""
	}
	if u.IsAbs() {
		trusted := false
		for _, origin := range []string{PublicEndpoints().HRM, LANEndpoints().HRM} {
			allowed, e := url.Parse(origin)
			if e == nil && strings.EqualFold(u.Scheme, allowed.Scheme) && strings.EqualFold(u.Host, allowed.Host) {
				trusted = true
			}
		}
		if !trusted {
			return ""
		}
	} else if u.Host != "" {
		return ""
	}
	if !validHRMAssetURI(u.EscapedPath()) {
		return ""
	}
	return u.Path
}

func (s *SQLStore) checkHRMAssetPermission(ctx context.Context, uid int64, resource string) error {
	requested := trustedHRMUploadPath(resource)
	if requested == "" {
		return ErrHRMPublicAdminRequired
	}
	// A super role can administer the whole local resource library.
	rows, err := s.DB.QueryContext(ctx, `SELECT COALESCE(r.rules,''),COALESCE(r.btns,'') FROM `+s.Prefix+`auth_role r JOIN `+s.Prefix+`auth_role_access x ON x.role_id=r.id WHERE x.uid=? AND r.status=0`, uid)
	if err != nil {
		return err
	}
	var ids []int64
	super := false
	for rows.Next() {
		var rules, btns string
		if err = rows.Scan(&rules, &btns); err != nil {
			rows.Close()
			return err
		}
		super = super || rules == "*"
		ids = append(ids, parseIDs(btns)...)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if super {
		return nil
	}
	var avatar string
	if err = s.DB.QueryRowContext(ctx, `SELECT COALESCE(avatar,'') FROM `+s.Prefix+`admin WHERE id=?`, uid).Scan(&avatar); err != nil {
		return err
	}
	if trustedHRMUploadPath(avatar) == requested {
		return nil
	}
	if allowed, e := s.hasPermission(ctx, s.DB, uid, "biz-suite.im.read"); e != nil {
		return e
	} else if allowed {
		if directoryAvatar, e := s.IsDirectoryAvatarResource(ctx, resource); e != nil {
			return e
		} else if directoryAvatar {
			return nil
		}
	}
	library, err := s.hasPermission(ctx, s.DB, uid, "biz-suite.hrm.manage")
	if err != nil {
		return err
	}
	// This exact host permission grants the existing shared attachment library;
	// HRM self/department and IM permissions alone do not grant arbitrary files.
	for _, id := range ids {
		var count int
		if err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.Prefix+`auth_rule WHERE id=? AND status=0 AND type=2 AND path=?`, id, "/admin/datacenter/attachment/getMyFiles").Scan(&count); err != nil {
			return err
		}
		library = library || count > 0
	}
	if !library {
		return ErrHRMPublicAdminRequired
	}
	attachments, err := s.DB.QueryContext(ctx, `SELECT COALESCE(url,''),COALESCE(cover_url,'') FROM `+s.Prefix+`attachment WHERE location='local'`)
	if err != nil {
		return err
	}
	defer attachments.Close()
	for attachments.Next() {
		var original, cover string
		if err = attachments.Scan(&original, &cover); err != nil {
			return err
		}
		if trustedHRMUploadPath(original) == requested || trustedHRMUploadPath(cover) == requested {
			return nil
		}
	}
	if err = attachments.Err(); err != nil {
		return err
	}
	return ErrHRMPublicAdminRequired
}

func bindHRMResourceChecks(server *ghttp.Server, s *Service) {
	server.BindHandler("GET:/internal/hrm-assets/check", s.hrmAssetCheck)
	server.BindHandler("GET:/internal/hrm-backoffice/check", s.hrmBackofficeToolCheck)
}
