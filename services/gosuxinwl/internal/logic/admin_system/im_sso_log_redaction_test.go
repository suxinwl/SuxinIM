package adminsystem

import "testing"

func TestSSOAuditNeverCollectsTokensPasswordsOrTicketResponses(t *testing.T) {
	for _, path := range []string{"/admin/user/saveInfo", "/admin/user/login", "/admin/system/account/save", "/admin/system/account/resetPassword", "/common/im-sso/password/change", "/common/im-sso/password/status"} {
		request, response := operationLogBodies(path, func() (any, any) { t.Fatal("password body collector ran"); return nil, nil })
		if request != imSSOLogBodyHiddenMarker || response != imSSOLogBodyHiddenMarker {
			t.Fatal("password request or response not redacted")
		}
	}
	for _, path := range []string{"/common/im-sso/login", "/common/im-sso/refresh", "/common/im-sso/exchange", "/common/im-sso/logout", "/common/im-sso/ticket", "/plugins/biz-suite/api/admin/im/openim/ticket", "//plugins//biz-suite/api/admin/im/openim/./ticket/", "/plugins/biz-suite/api/admin/im/manager/openim/certificate", "//plugins/biz-suite/api/admin/im/manager/openim/./certificate/", "/internal/im-access/check"} {
		req, resp := operationLogBodies(path, func() (any, any) { t.Fatal("sensitive collector ran"); return nil, nil })
		if req != imSSOLogBodyHiddenMarker || resp != imSSOLogBodyHiddenMarker {
			t.Fatal("SSO audit body not redacted")
		}
	}
	for _, path := range []string{"/common/im-sso-other", "/plugins/biz-suite/api/admin/im/manager/openim/preview"} {
		if isIMSSOLogSensitivePath(path) {
			t.Fatal("unrelated audit suppressed")
		}
	}
}
