package imbridge

import (
	"context"
	"errors"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
)

type managerWrap func(func(context.Context, *Service, *ghttp.Request) (any, error)) func(*ghttp.Request)

func bindAccessManager(group *ghttp.RouterGroup, with managerWrap) {
	base := "/api/admin/im/manager/openim/"
	group.GET(base+"access/networks", with(func(c context.Context, s *Service, _ *ghttp.Request) (any, error) { return s.Store.NetworkPolicy(c) }))
	group.POST(base+"access/networks", with(func(c context.Context, s *Service, r *ghttp.Request) (any, error) {
		var in struct {
			CIDRs            []string `json:"cidrs"`
			ExpectedRevision *int64   `json:"expectedRevision"`
		}
		if decode(r.Request, &in) != nil || in.ExpectedRevision == nil {
			return nil, ErrInvalid
		}
		return s.Store.SaveNetworkPolicy(c, in.CIDRs, *in.ExpectedRevision, gconv.Int64(c.Value("uid")))
	}))
	group.GET(base+"access/employees", with(func(c context.Context, s *Service, _ *ghttp.Request) (any, error) {
		rows, err := s.Store.EmployeeAccessList(c)
		return map[string]any{"items": rows}, err
	}))
	group.POST(base+"access/employee", with(func(c context.Context, s *Service, r *ghttp.Request) (any, error) {
		var in struct {
			EmployeeID            string `json:"employeeID"`
			AllowLAN              *bool  `json:"allowLAN"`
			AllowPC               *bool  `json:"allowPC"`
			AllowMobile           *bool  `json:"allowMobile"`
			AllowInternet         *bool  `json:"allowInternet"`
			AllowInternetDownload *bool  `json:"allowInternetDownload"`
			ExpectedRevision      *int64 `json:"expectedRevision"`
		}
		if decode(r.Request, &in) != nil || in.ExpectedRevision == nil || in.AllowLAN == nil || in.AllowPC == nil || in.AllowMobile == nil || in.AllowInternet == nil || in.AllowInternetDownload == nil {
			return nil, ErrInvalid
		}
		return s.SaveEmployeeAccess(c, EmployeeAccess{EmployeeID: in.EmployeeID, AllowLAN: *in.AllowLAN, AllowPC: *in.AllowPC, AllowMobile: *in.AllowMobile, AllowInternet: *in.AllowInternet, AllowInternetDownload: *in.AllowInternetDownload}, *in.ExpectedRevision, gconv.Int64(c.Value("uid")))
	}))
	group.GET(base+"certificate", with(func(c context.Context, s *Service, _ *ghttp.Request) (any, error) { return certificateStatus() }))
	group.POST(base+"certificate", with(func(c context.Context, s *Service, r *ghttp.Request) (any, error) {
		var in certificateSubmission
		if err := decodeCertificate(r.Request, &in); err != nil {
			return nil, err
		}
		if gconv.Int64(c.Value("uid")) <= 0 {
			return nil, ErrDenied
		}
		v, err := submitCertificate(in, gconv.Int64(c.Value("uid")))
		if errors.Is(err, errCertificateUnavailable) {
			return nil, errCertificateUnavailable
		}
		return v, err
	}))
}
