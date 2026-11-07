package adminsystem

import (
	"io"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestWeComLogSensitiveRoutesAreExactAndNormalized(t *testing.T) {
	const prefix = "/plugins/biz-suite/api/admin/wecom/"
	for _, suffix := range []string{"settings", "credentials", "credentials/contact", "credentials/app", "diagnostics/contact", "diagnostics/app", "diagnostics/checkin", "diagnostics/approval"} {
		for _, route := range []string{prefix + suffix, prefix + suffix + "/", prefix + "./" + suffix, "//plugins//biz-suite//api/admin/wecom/" + suffix + "//"} {
			if !isWeComLogSensitivePath(route) {
				t.Errorf("sensitive route was not hidden: %q", route)
			}
		}
	}
	for _, route := range []string{
		"", "/", prefix, prefix + "status", prefix + "sync/contact", prefix + "settings-extra",
		prefix + "settings/extra", prefix + "credentials/other", prefix + "credentials/contact/extra",
		prefix + "diagnostics", prefix + "diagnostics/other", prefix + "diagnostics/contact/extra",
		prefix + "settings/../status", prefix + "../settings", prefix + "SETTINGS",
		prefix + "settings?secret=dummy", prefix + "settings#fragment", prefix + "settings%2Fextra",
		"/plugins/other/api/admin/wecom/settings", "/plugins/biz-suite-extra/api/admin/wecom/settings",
		"/plugins/biz-suite/admin/wecom/settings", "/api/admin/wecom/settings", "/other" + prefix + "settings",
	} {
		if isWeComLogSensitivePath(route) {
			t.Errorf("unrelated route was hidden: %q", route)
		}
	}
}

func TestWeComLogBodiesNeverCollectSecretsAndLeaveRequestIntact(t *testing.T) {
	const body = `{"contact_secret":"dummy-contact-secret","app_secret":"dummy-app-secret","credentials":{"secret":"dummy-nested-secret"}}`
	for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"} {
		t.Run(method, func(t *testing.T) {
			request := httptest.NewRequest(method, "/plugins/biz-suite/api/admin/wecom/credentials/contact/?secret=dummy-query-secret", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "dummy-auth")
			originalURL, originalHeaders := request.URL.String(), request.Header.Clone()
			response := map[string]any{"message": "dummy-app-secret", "access_token": "dummy-token"}
			originalResponse := map[string]any{"message": "dummy-app-secret", "access_token": "dummy-token"}
			reqBody, respBody := operationLogBodies(request.URL.Path, func() (any, any) {
				t.Fatal("sensitive request/response collector executed")
				return request.Body, response
			})
			if reqBody != wecomLogBodyHiddenMarker || respBody != wecomLogBodyHiddenMarker {
				t.Fatal("credential bodies were not replaced with the fixed marker")
			}
			remaining, err := io.ReadAll(request.Body)
			if err != nil || string(remaining) != body {
				t.Fatal("redaction consumed or changed the live request body")
			}
			if request.URL.String() != originalURL || !reflect.DeepEqual(request.Header, originalHeaders) || !reflect.DeepEqual(response, originalResponse) {
				t.Fatal("redaction changed routing, authentication or handler response")
			}
		})
	}
}

func TestWeComLogOrdinaryPluginRoutesStillCollectBodies(t *testing.T) {
	for _, route := range []string{"/plugins/biz-suite/api/admin/wecom/status", "/plugins/biz-suite/api/admin/wecom/sync/contact", "/plugins/biz-suite/api/admin/hrm/employees", "/plugins/other/api/admin/wecom/settings"} {
		calls := 0
		request, response := map[string]any{"employee_id": "dummy"}, map[string]any{"status": "success"}
		reqBody, respBody := operationLogBodies(route, func() (any, any) {
			calls++
			return request, response
		})
		if calls != 1 || !reflect.DeepEqual(reqBody, request) || !reflect.DeepEqual(respBody, response) {
			t.Fatalf("ordinary operation audit changed for %s", route)
		}
	}
}
