package imbridge

import (
	"context"
	"errors"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
)

// BindManager must be placed inside biz-suite's trusted-host identity and RBAC
// group. Paths containing /manager/ require im.manage in its existing gate.
// No caller-supplied upstream URL or management operation is accepted.
func BindManager(group *ghttp.RouterGroup) {
	bindManager(group, Open)
}

func bindManager(group *ghttp.RouterGroup, open func(context.Context) (*Service, func(), error)) {
	group.GET("/api/admin/im/openim/status", func(r *ghttp.Request) {
		envelope(r.Response.BufferWriter, 200, map[string]any{"enabled": ConfigFromEnv().Enabled, "webURL": "/im/"}, nil)
	})
	with := func(fn func(context.Context, *Service, *ghttp.Request) (any, error)) func(*ghttp.Request) {
		return func(r *ghttp.Request) {
			svc, close, e := open(r.Context())
			if e != nil {
				envelope(r.Response.BufferWriter, 503, nil, errors.New("通讯服务连接失败"))
				return
			}
			if svc == nil {
				envelope(r.Response.BufferWriter, 503, nil, errors.New("OpenIM整合尚未启用"))
				return
			}
			defer close()
			c := requestAccessContext(r.Context(), r.Request)
			if HRMPublicAdminsOnly() {
				if err := svc.Store.CheckHRMBackofficeAccess(c, gconv.Int64(c.Value("uid"))); err != nil {
					envelope(r.Response.BufferWriter, 403, nil, ErrHRMPublicAdminRequired)
					return
				}
			} else if strings.Contains(r.URL.Path, "/manager/") {
				// Existing deployments retain their original LAN management gate.
				network, err := svc.Store.NetworkPolicy(c)
				if err != nil || network.CurrentZone != "lan" {
					envelope(r.Response.BufferWriter, 403, nil, ErrDenied)
					return
				}
			}
			v, e := fn(c, svc, r)
			if e != nil {
				envelope(r.Response.BufferWriter, authStatus(e), nil, errors.New(errorText(e)))
				return
			}
			envelope(r.Response.BufferWriter, 200, v, nil)
		}
	}
	bindAccessManager(group, with)
	group.POST("/api/admin/im/openim/ticket", with(func(c context.Context, s *Service, r *ghttp.Request) (any, error) {
		t, e := s.IssueTicket(c, gconv.Int64(c.Value("uid")))
		if e != nil {
			return nil, e
		}
		return map[string]any{"ticket": t, "expiresIn": 30, "url": "/im/?ticket=" + t + "#/login"}, nil
	}))
	group.GET("/api/admin/im/manager/openim/employees", with(func(c context.Context, s *Service, _ *ghttp.Request) (any, error) { return s.Store.List(c, false) }))
	group.GET("/api/admin/im/manager/openim/departments", with(func(c context.Context, s *Service, _ *ghttp.Request) (any, error) { return s.Store.List(c, true) }))
	group.GET("/api/admin/im/manager/openim/preview", with(func(c context.Context, s *Service, _ *ghttp.Request) (any, error) { return s.Store.Preview(c) }))
	group.POST("/api/admin/im/manager/openim/provision", with(func(c context.Context, s *Service, r *ghttp.Request) (any, error) {
		var in struct {
			Confirm bool `json:"confirm"`
		}
		if e := decode(r.Request, &in); e != nil || !in.Confirm {
			return nil, errors.New("请先预览并明确确认首次开通")
		}
		return map[string]bool{"accepted": true}, s.Store.ConfirmProvision(c)
	}))
	group.POST("/api/admin/im/manager/openim/retry", with(func(c context.Context, s *Service, _ *ghttp.Request) (any, error) {
		return map[string]bool{"accepted": true}, s.Store.Retry(c)
	}))
	group.GET("/api/admin/im/manager/openim/groups", with(func(c context.Context, s *Service, r *ghttp.Request) (any, error) {
		page, size := r.Get("page").Int(), r.Get("pageSize").Int()
		if page < 1 {
			page = 1
		}
		if size < 1 || size > 100 {
			size = 50
		}
		return s.Remote.Manage(c, "groups/search", map[string]any{"keyword": strings.TrimSpace(r.Get("keyword").String()), "page": page, "pageSize": size})
	}))
	group.GET("/api/admin/im/manager/openim/groups/members", with(func(c context.Context, s *Service, r *ghttp.Request) (any, error) {
		id := strings.TrimSpace(r.Get("groupID").String())
		if id == "" || len(id) > 64 {
			return nil, errors.New("群组参数无效")
		}
		return s.Remote.Manage(c, "groups/members", map[string]string{"groupID": id})
	}))
	group.POST("/api/admin/im/manager/openim/groups/mute", with(func(c context.Context, s *Service, r *ghttp.Request) (any, error) {
		var in struct {
			GroupID string `json:"groupID"`
			Muted   bool   `json:"muted"`
		}
		if e := decode(r.Request, &in); e != nil {
			return nil, e
		}
		if in.GroupID == "" || strings.HasPrefix(in.GroupID, "dept_") {
			return nil, errors.New("部门群由HRM统一维护")
		}
		return s.Remote.Manage(c, "groups/mute", in)
	}))
	group.POST("/api/admin/im/manager/openim/groups/notice", with(func(c context.Context, s *Service, r *ghttp.Request) (any, error) {
		var in struct {
			GroupID string `json:"groupID"`
			Notice  string `json:"notice"`
		}
		if e := decode(r.Request, &in); e != nil {
			return nil, e
		}
		if in.GroupID == "" || strings.HasPrefix(in.GroupID, "dept_") || len(in.Notice) > 4096 {
			return nil, errors.New("群公告参数无效，部门群由HRM维护")
		}
		return s.Remote.Manage(c, "groups/notice", in)
	}))
}
