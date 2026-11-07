package adminsystem

import "path"

const wecomLogBodyHiddenMarker = "[REDACTED: 企业微信凭据与诊断正文已隐藏]"

// Match the public host routes, before the reverse proxy strips the plugin
// prefix. Redact every method, including invalid or rejected credential writes.
// URL.Path is already URL-decoded; cleaning also covers normalized route aliases.
func isWeComLogSensitivePath(requestPath string) bool {
	switch path.Clean("/" + requestPath) {
	case "/plugins/biz-suite/api/admin/wecom/settings",
		"/plugins/biz-suite/api/admin/wecom/credentials",
		"/plugins/biz-suite/api/admin/wecom/credentials/contact",
		"/plugins/biz-suite/api/admin/wecom/credentials/app",
		"/plugins/biz-suite/api/admin/wecom/diagnostics/contact",
		"/plugins/biz-suite/api/admin/wecom/diagnostics/app",
		"/plugins/biz-suite/api/admin/wecom/diagnostics/checkin",
		"/plugins/biz-suite/api/admin/wecom/diagnostics/approval":
		return true
	default:
		return false
	}
}
