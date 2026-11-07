package imbridge

import (
	"context"
	"errors"
	"net/http"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
	"github.com/suxinwl/GoSuxin/internal/accountsecurity"
	"github.com/suxinwl/GoSuxin/utility/auth"
)

func (s *SQLStore) security() *accountsecurity.Store { return s.Security }

type logicalClientContextKey struct{}
type authenticatedVersionKey struct{}

func logicalClient(platform, policy int) string {
	switch platform {
	case 1:
		return "ios"
	case 2:
		return "android"
	case 3:
		return "pc"
	case 5:
		if policy == 2 {
			return "web_mobile"
		}
		return "web_desktop"
	}
	return ""
}
func validLogicalPlatform(client string, platform int) bool {
	return platform == 5 && (client == "web_mobile" || client == "web_desktop") || platform == 1 && client == "ios" || platform == 2 && client == "android" || platform == 3 && client == "pc"
}
func sessionLogical(v Session) string {
	if v.LogicalClient != "" {
		return v.LogicalClient
	}
	return logicalClient(v.Platform, v.PolicyPlatform)
}
func (s *SQLStore) effectivePolicy(ctx context.Context, uid int64, client string) (accountsecurity.EffectivePolicy, error) {
	n, e := s.NetworkPolicy(ctx)
	if e != nil {
		return accountsecurity.EffectivePolicy{}, e
	}
	p, e := s.security().Resolve(ctx, uid, n.CurrentZone, client)
	p.NetworkRevision = n.Revision
	p.CurrentIP = n.CurrentIP
	return p, e
}

func CurrentHostPolicy(ctx context.Context, uid int64) (accountsecurity.EffectivePolicy, error) {
	db, e := g.DB().Master()
	if e != nil {
		return accountsecurity.EffectivePolicy{}, e
	}
	store, e := NewSQLStore(db, g.DB().GetConfig().Prefix)
	if e != nil {
		return accountsecurity.EffectivePolicy{}, e
	}
	if svc := live.Load(); svc != nil {
		store = svc.Store
	} else {
		store.Security.IMEnabled = false
	}
	client := "web_desktop"
	r := g.RequestFromCtx(ctx)
	if r == nil {
		return accountsecurity.EffectivePolicy{}, ErrDenied
	}
	claims, e := auth.ParseTokenReadOnly(r)
	if e != nil {
		return accountsecurity.EffectivePolicy{}, ErrDenied
	}
	data, _ := claims.Data.(map[string]interface{})
	if v := gconv.String(data["policyClient"]); v != "" {
		client = v
	}
	return store.effectivePolicy(requestAccessContext(ctx, r.Request), uid, client)
}

// HostSecurityClaims also protects HRM APIs that do not require IM membership.
func HostSecurityClaims(ctx context.Context, uid int64) (map[string]any, error) {
	security, e := accountsecurity.Default(ctx)
	if e != nil {
		return nil, e
	}
	v, e := security.State(ctx, uid)
	if e != nil {
		return nil, e
	}
	if v.MustChange {
		return nil, accountsecurity.ErrPasswordChangeRequired
	}
	if v.Locked {
		return nil, accountsecurity.ErrDenied
	}
	var password, salt string
	var status, deleted int
	e = security.DB.QueryRowContext(ctx, `SELECT COALESCE(password,''),COALESCE(salt,''),status,CASE WHEN deletetime IS NULL THEN 0 ELSE 1 END FROM `+security.Prefix+`admin WHERE id=?`, uid).Scan(&password, &salt, &status, &deleted)
	if e != nil || status != 0 || deleted != 0 {
		return nil, accountsecurity.ErrDenied
	}
	client := "web_desktop"
	if r := g.RequestFromCtx(ctx); r != nil {
		c := requestAccessContext(ctx, r.Request)
		if mobile, _ := c.Value(mobileBrowserContextKey{}).(bool); mobile {
			client = "web_mobile"
		}
		if svc := live.Load(); svc != nil {
			policy, e := svc.Store.effectivePolicy(c, uid, client)
			if e != nil || !policy.Login {
				return nil, accountsecurity.ErrDenied
			}
		}
	}
	return map[string]any{"securityVersion": v.SecurityVersion, "credentialFingerprint": accountsecurity.Hash(password + ":" + salt), "policyClient": client}, nil
}
func checkHostSecurity(ctx context.Context, claims *auth.CustomClaims, supplied ...*accountsecurity.Store) error {
	data, _ := claims.Data.(map[string]interface{})
	var security *accountsecurity.Store
	var e error
	if len(supplied) > 0 {
		security = supplied[0]
	} else {
		security, e = accountsecurity.Default(ctx)
	}
	if e != nil {
		return errors.New("账号安全服务暂不可用")
	}
	version := gconv.Int64(data["securityVersion"])
	fingerprint := gconv.String(data["credentialFingerprint"])
	// Existing signed HRM asset claims contain the same security binding.
	if version == 0 {
		version = gconv.Int64(data["hrmAssetEpoch"])
		fingerprint = gconv.String(data["hrmAssetCredential"])
	}
	return security.CheckSession(ctx, gconv.Int64(data["uid"]), version, fingerprint)
}
func (s *Service) passwordRoutes(bind func(string, http.HandlerFunc)) {
	bind("/password/status", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ChangeTicket string `json:"changeTicket"`
		}
		if decode(r, &in) != nil {
			envelope(w, 400, nil, ErrInvalid)
			return
		}
		v, e := s.Store.security().TicketStatus(r.Context(), in.ChangeTicket)
		if e != nil {
			envelope(w, 403, nil, ErrDenied)
			return
		}
		envelope(w, 200, v, nil)
	})
	bind("/password/cancel", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ChangeTicket string `json:"changeTicket"`
		}
		if decode(r, &in) != nil {
			envelope(w, 400, nil, ErrInvalid)
			return
		}
		e := s.Store.security().CancelTicket(r.Context(), in.ChangeTicket)
		if e != nil {
			envelope(w, 403, nil, ErrDenied)
			return
		}
		envelope(w, 200, map[string]bool{"cancelled": true}, nil)
	})
	bind("/password/change", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			SessionToken string `json:"sessionToken"`
			ChangeTicket string `json:"changeTicket"`
			OldPassword  string `json:"oldPassword"`
			NewPassword  string `json:"newPassword"`
		}
		if decode(r, &in) != nil || (in.ChangeTicket != "" && in.SessionToken != "") {
			envelope(w, 400, nil, ErrInvalid)
			return
		}
		var uid int64
		if in.ChangeTicket == "" {
			if s.Cache == nil {
				envelope(w, 401, nil, ErrExpired)
				return
			}
			v, e := s.Validate(r.Context(), in.SessionToken)
			if e != nil {
				envelope(w, 401, nil, ErrExpired)
				return
			}
			uid = v.UID
		}
		e := s.Store.security().ChangePassword(r.Context(), uid, uid, in.OldPassword, in.NewPassword, in.ChangeTicket, false)
		if e != nil {
			envelope(w, 403, nil, errors.New("密码修改未通过，请检查当前密码或重新登录"))
			return
		}
		if uid > 0 {
			_ = auth.RemoveToken(r.Context(), gconv.String(uid))
		}
		NotifyDepartmentsChanged()
		setAttachmentCookie(w, LoginResult{}, true)
		envelope(w, 200, map[string]bool{"changed": true, "requiresLogin": true}, nil)
	})
}
