package imbridge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/internal/extend/middleware"
	"github.com/suxinwl/GoSuxin/internal/service"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"golang.org/x/time/rate"
)

var live atomic.Pointer[Service]

func Open(ctx context.Context) (*Service, func(), error) {
	cfg := ConfigFromEnv()
	if !cfg.Enabled {
		return nil, func() {}, nil
	}
	if e := cfg.Validate(); e != nil {
		return nil, nil, e
	}
	db, e := g.DB().Master()
	if e != nil {
		return nil, nil, errors.New("IM HRM database unavailable")
	}
	store, e := NewSQLStore(db, g.DB().GetConfig().Prefix)
	if e != nil {
		return nil, nil, e
	}
	if cfg.PublicURL != "" {
		store.PublicURL = cfg.PublicURL
	}
	cache, e := NewRedisCache(cfg.RedisURL)
	if e != nil {
		return nil, nil, e
	}
	close := func() { _ = cache.Close() }
	if e = cache.Ping(ctx); e != nil {
		close()
		return nil, nil, errors.New("IM session Redis unavailable")
	}
	remote, e := NewHTTPRemote(cfg.BridgeURL, cfg.HMACKey)
	if e != nil {
		close()
		return nil, nil, e
	}
	return &Service{Store: store, Cache: cache, Remote: remote}, close, nil
}

// Start is called by the host only, never by the plugin worker. Production
// remains unchanged unless the isolated deployment explicitly opts in.
func Start(ctx context.Context, server *ghttp.Server) (func(), error) {
	svc, close, e := Open(ctx)
	if e != nil {
		return nil, e
	}
	if svc == nil {
		if HRMPublicAdminsOnly() {
			return nil, errors.New("HRM public administrator policy requires the enabled IM integration")
		}
		return func() {}, nil
	}
	if e = svc.Store.Ensure(ctx); e != nil {
		close()
		return nil, e
	}
	stopHRMPolicy := registerHRMBackofficePolicy(svc.Store)
	live.Store(svc)
	bindPublic(server, svc)
	bindInternalAccess(server, svc)
	bindHRMResourceChecks(server, svc)
	workCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer closeChannel(done)
		scan := time.NewTicker(30 * time.Second)
		defer scan.Stop()
		drain := time.NewTicker(time.Second)
		defer drain.Stop()
		health := workerHealth{}
		record := func(stage string, result error) {
			if err := health.record(workCtx, svc.Store, stage, result); err != nil && workCtx.Err() == nil {
				g.Log().Warningf(workCtx, "OpenIM worker health update failed %s", Diagnostic(fault("worker.heartbeat", err)))
			}
		}
		run := func() {
			record("scanning", nil)
			e := svc.Store.Reconcile(workCtx)
			record("scan_completed", e)
			if e != nil {
				g.Log().Warningf(workCtx, "OpenIM employee reconciliation failed %s; current projection retained", Diagnostic(e))
			}
		}
		run()
		for {
			select {
			case <-workCtx.Done():
				return
			case <-scan.C:
				run()
			case <-drain.C:
				e := svc.Store.Drain(workCtx, svc.Remote)
				record("drain_completed", e)
				if e != nil {
					g.Log().Warningf(workCtx, "OpenIM outbox worker failed %s; tasks retained for retry", Diagnostic(e))
				}
			}
		}
	}()
	return func() { cancel(); <-done; live.Store(nil); stopHRMPolicy(); close() }, nil
}
func closeChannel(c chan struct{}) { close(c) }

// RevokeUID is a no-op when the feature is disabled. Callers must treat an
// error as a failed security action; queued revocations survive a restart.
func RevokeUID(ctx context.Context, uid int64) error {
	if svc := live.Load(); svc != nil {
		return svc.RevokeUID(ctx, uid)
	}
	return nil
}
func bindPublic(server *ghttp.Server, s *Service) {
	server.BindHookHandler("/common/im-sso/*path", ghttp.HookBeforeServe, func(r *ghttp.Request) { sanitizeSSOLogURL(r.Request) })
	limiter := rate.NewLimiter(20, 40)
	wrap := func(fn http.HandlerFunc) func(*ghttp.Request) {
		return func(r *ghttp.Request) {
			defer func() {
				if recover() != nil {
					g.Log().Warning(r.Context(), "OpenIM SSO handler panic recovered")
					r.Response.ClearBuffer()
					envelope(r.Response.BufferWriter, 503, nil, errors.New("通讯服务暂不可用"))
				}
			}()
			if !ssoCORS(r.Response.BufferWriter, r.Request, ConfigFromEnv().PublicURL) {
				return
			}
			if !limiter.Allow() {
				envelope(r.Response.BufferWriter, 429, nil, errors.New("请求过于频繁"))
				return
			}
			fn(r.Response.BufferWriter, r.Request.WithContext(requestAccessContext(r.Context(), r.Request)))
		}
	}
	server.BindHandler("OPTIONS:/common/im-sso/*path", wrap(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	server.BindHandler("GET:/common/im-sso/captcha", wrap(func(w http.ResponseWriter, r *http.Request) {
		v, e := gf.GenerateCaptcha(r.Context())
		if e != nil {
			envelope(w, 503, nil, errors.New("验证码生成失败"))
			return
		}
		c := v.(gf.CaptchaResult)
		envelope(w, 200, map[string]any{"id": c.Id, "captchaId": c.Id, "show": c.Show, "enabled": c.Show, "img": c.Base64Blog, "image": c.Base64Blog, "expireTime": c.ExpireTime}, nil)
	}))
	server.BindHandler("POST:/common/im-sso/login", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Account   string `json:"account"`
			Password  string `json:"password"`
			Platform  string `json:"platform"`
			Captcha   string `json:"captcha"`
			CaptchaID string `json:"captchaId"`
		}
		if e := decode(r, &in); e != nil {
			envelope(w, 400, nil, e)
			return
		}
		if len(in.Account) > 128 || len(in.Password) != 32 {
			envelope(w, 403, nil, ErrDenied)
			return
		}
		if _, e := Platform(in.Platform); e != nil {
			envelope(w, 400, nil, e)
			return
		}
		uid, e := s.Store.ResolveAccount(r.Context(), in.Account)
		if e != nil {
			envelope(w, 403, nil, ErrDenied)
			return
		}
		username, e := dao.Admin.Ctx(r.Context()).WherePri(uid).Value("username")
		if e != nil {
			envelope(w, 403, nil, ErrDenied)
			return
		}
		_, e = service.AdminUser().AuthenticatePassword(r.Context(), username.String(), in.Password, in.CaptchaID, in.Captcha)
		if e != nil {
			envelope(w, 403, nil, ErrDenied)
			return
		}
		result, e := s.Login(r.Context(), uid, in.Platform)
		if e != nil {
			envelope(w, authStatus(e), nil, errors.New(errorText(e)))
			return
		}
		setAttachmentCookie(w, result, false)
		envelope(w, 200, result, nil)
	}))
	server.BindHandler("POST:/common/im-sso/refresh", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			SessionToken string `json:"sessionToken"`
		}
		if e := decode(r, &in); e != nil {
			envelope(w, 400, nil, e)
			return
		}
		v, e := s.Refresh(r.Context(), in.SessionToken)
		if e != nil {
			envelope(w, authStatus(e), nil, errors.New(errorText(e)))
			return
		}
		setAttachmentCookie(w, v, false)
		envelope(w, 200, v, nil)
	}))
	server.BindHandler("POST:/common/im-sso/logout", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			SessionToken string `json:"sessionToken"`
			All          bool   `json:"all"`
		}
		if e := decode(r, &in); e != nil {
			envelope(w, 400, nil, e)
			return
		}
		if e := s.Logout(r.Context(), in.SessionToken, in.All); e != nil {
			envelope(w, authStatus(e), nil, errors.New(errorText(e)))
			return
		}
		setAttachmentCookie(w, LoginResult{}, true)
		envelope(w, 200, map[string]bool{"loggedOut": true}, nil)
	}))
	server.BindHandler("POST:/common/im-sso/exchange", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Ticket   string `json:"ticket"`
			Platform string `json:"platform"`
		}
		if e := decode(r, &in); e != nil {
			envelope(w, 400, nil, e)
			return
		}
		v, e := s.Exchange(r.Context(), in.Ticket, in.Platform)
		if e != nil {
			envelope(w, authStatus(e), nil, errors.New(errorText(e)))
			return
		}
		setAttachmentCookie(w, v, false)
		envelope(w, 200, v, nil)
	}))
	server.BindHandler("POST:/common/im-sso/directory", wrap(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			SessionToken string `json:"sessionToken"`
			Keyword      string `json:"keyword"`
			Page         int    `json:"page"`
			PageSize     int    `json:"pageSize"`
		}
		if e := decode(r, &in); e != nil {
			envelope(w, 400, nil, e)
			return
		}
		if _, e := s.Validate(r.Context(), in.SessionToken); e != nil {
			envelope(w, authStatus(e), nil, errors.New(errorText(e)))
			return
		}
		v, e := s.Store.Directory(r.Context(), strings.TrimSpace(in.Keyword), in.Page, in.PageSize)
		if e != nil {
			envelope(w, 503, nil, errors.New("通讯录暂不可用"))
			return
		}
		envelope(w, 200, v, nil)
	}))
	server.Group("/common/im-sso", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.Token)
		group.POST("/ticket", func(r *ghttp.Request) {
			ticket, e := s.IssueTicket(requestAccessContext(r.Context(), r.Request), gconv.Int64(r.Context().Value("uid")))
			if e != nil {
				envelope(r.Response.BufferWriter, authStatus(e), nil, errors.New(errorText(e)))
				return
			}
			envelope(r.Response.BufferWriter, 200, map[string]any{"ticket": ticket, "expiresIn": 30, "url": "/im/?ticket=" + ticket + "#/login"}, nil)
		})
	})
}
func internalError(label string, e error) error {
	if e != nil {
		return fmt.Errorf("%s unavailable", label)
	}
	return nil
}
