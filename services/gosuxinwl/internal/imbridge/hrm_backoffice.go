package imbridge

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
	hostauth "github.com/suxinwl/GoSuxin/utility/auth"
)

var ErrHRMPublicAdminRequired = errors.New("公网 HRM 仅限管理员访问，请使用原内网入口")
var ErrHRMBackofficeUnavailable = errors.New("暂时无法确认 HRM 访问权限，请稍后再试")

// HRMPublicAdminsOnly is deliberately separate from employee IM permissions.
// Existing installations keep their current behavior unless this instance opts in.
func HRMPublicAdminsOnly() bool {
	return os.Getenv("GOSUXIN_HRM_PUBLIC_ADMINS_ONLY") == "true"
}

func hrmBackofficePath(value string) bool {
	value = path.Clean("/" + strings.TrimLeft(value, "/"))
	if value == "/admin" || strings.HasPrefix(value, "/admin/") {
		return true
	}
	parts := strings.Split(strings.Trim(value, "/"), "/")
	return len(parts) >= 3 && parts[0] == "plugins" &&
		(parts[2] == "admin" || (len(parts) >= 4 && parts[2] == "api" && parts[3] == "admin"))
}

// CheckHRMBackofficeLogin must follow the existing password/captcha validation
// and precede creation of any HRM token. Employee IM login does not call it.
func CheckHRMBackofficeLogin(ctx context.Context, uid int64) error {
	if !HRMPublicAdminsOnly() {
		return nil
	}
	r := g.RequestFromCtx(ctx)
	if r == nil {
		return ErrHRMPublicAdminRequired
	}
	db, err := g.DB().Master()
	if err != nil {
		return err
	}
	store, err := NewSQLStore(db, g.DB().GetConfig().Prefix)
	if err != nil {
		return err
	}
	return store.CheckHRMBackofficeAccess(requestAccessContext(ctx, r.Request), uid)
}

func registerHRMBackofficePolicy(store *SQLStore) func() {
	return hostauth.SetTokenRequestPolicy(func(r *ghttp.Request, claims *hostauth.CustomClaims) error {
		if err := checkHostSecurity(r.Context(), claims, store.Security); err != nil {
			return err
		}
		data, _ := claims.Data.(map[string]interface{})
		client := gconv.String(data["policyClient"])
		if client == "" {
			client = "web_desktop"
		}
		policy, err := store.effectivePolicy(requestAccessContext(r.Context(), r.Request), gconv.Int64(data["uid"]), client)
		if err != nil || !policy.Login {
			return ErrHRMPublicAdminRequired
		}
		version := gconv.Int64(data["securityVersion"])
		if version == 0 {
			version = gconv.Int64(data["hrmAssetEpoch"])
		}
		if policy.SecurityVersion != version {
			return ErrHRMPublicAdminRequired
		}
		if !HRMPublicAdminsOnly() || !hrmBackofficePath(r.URL.Path) {
			return nil
		}
		err = store.CheckHRMBackofficeAccess(requestAccessContext(r.Context(), r.Request), gconv.Int64(data["uid"]))
		if err != nil && !errors.Is(err, ErrHRMPublicAdminRequired) {
			return ErrHRMBackofficeUnavailable
		}
		return err
	}, func(r *ghttp.Request, claims *hostauth.CustomClaims) {
		if hrmBackofficePath(r.URL.Path) {
			setHRMAssetCookie(r, hostauth.GetRequestToken(r), claims, false)
		}
	})
}

// CheckHRMBackofficeAccess adds a source restriction, never a business grant.
// The caller must still validate the host session, normal RBAC and cookie CSRF.
// Only an active super role or the exact HRM manage permission qualifies outside
// the configured LANs. IM-only, employee and department roles cannot qualify.
func (s *SQLStore) CheckHRMBackofficeAccess(ctx context.Context, uid int64) error {
	network, err := s.NetworkPolicy(ctx)
	if err != nil {
		return err
	}
	if network.CurrentZone == "lan" {
		return nil
	}
	if network.CurrentZone != "internet" || uid <= 0 {
		return ErrHRMPublicAdminRequired
	}
	if err = s.checkActiveHRMAccount(ctx, uid); err != nil {
		return err
	}
	allowed, err := s.hasPermission(ctx, s.DB, uid, "biz-suite.hrm.manage")
	if err != nil {
		return err
	}
	if !allowed {
		return ErrHRMPublicAdminRequired
	}
	return nil
}

func (s *SQLStore) checkActiveHRMAccount(ctx context.Context, uid int64) error {
	var status, deleted int
	err := s.DB.QueryRowContext(ctx, `SELECT status,CASE WHEN deletetime IS NULL THEN 0 ELSE 1 END FROM `+s.Prefix+`admin WHERE id=?`, uid).Scan(&status, &deleted)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (uid <= 0 || status != 0 || deleted != 0)) {
		return ErrHRMPublicAdminRequired
	}
	return err
}
