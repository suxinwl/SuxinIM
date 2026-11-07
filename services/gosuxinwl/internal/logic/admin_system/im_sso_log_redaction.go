package adminsystem

import (
	"path"
	"strings"
)

const imSSOLogBodyHiddenMarker = "[REDACTED: IM SSO credentials and tickets]"

func isIMSSOLogSensitivePath(requestPath string) bool {
	value := path.Clean("/" + requestPath)
	return value == "/admin/user/saveInfo" || value == "/admin/user/login" || value == "/admin/system/account/save" || value == "/admin/system/account/resetPassword" || value == "/common/im-sso" || strings.HasPrefix(value, "/common/im-sso/") || value == "/plugins/biz-suite/api/admin/im/openim/ticket" || value == "/plugins/biz-suite/api/admin/im/manager/openim/certificate" || strings.HasPrefix(value, "/internal/im-access/")
}
