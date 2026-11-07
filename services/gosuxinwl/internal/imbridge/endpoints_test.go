package imbridge

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEndpointSelectionRejectsRedirectHostsAndPreservesEntryPair(t *testing.T) {
	for _, value := range []struct{ host, want string }{{"192.168.10.11:9880", "https://192.168.10.11:9885"}, {"192.168.10.11:9885", "https://192.168.10.11:9885"}, {"hr.szputy.com:9880", "https://im.szputy.com:9885"}, {"im.szputy.com:9885", "https://im.szputy.com:9885"}} {
		r := httptest.NewRequest("POST", "https://"+value.host+"/common/im-sso/ticket", nil)
		r.Header.Set("X-Forwarded-Host", "evil.invalid")
		url, err := ticketURL(r, "once-only")
		if err != nil || url != value.want+"/?ticket=once-only#/login" {
			t.Fatalf("entry %s selected %s: %v", value.host, url, err)
		}
	}
	r := httptest.NewRequest("POST", "https://evil.invalid/", nil)
	r.Header.Set("X-Forwarded-Host", "hr.szputy.com:9880")
	if _, err := SelectEndpoints(r); err == nil {
		t.Fatal("untrusted Host accepted")
	}
}
func TestManagementRejectsInjectedFieldsAndDepartmentMutations(t *testing.T) {
	for _, value := range []struct{ operation, body string }{{"config/get", `{"adminToken":"injected"}`}, {"groups/search", `{"showNumber":1000}`}, {"groups/dismiss", `{"groupID":"dept_owned"}`}, {"unknown", `{}`}, {"groups/search", `{} {}`}} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(value.body))
		if _, err := decodeManagement(r, value.operation); err == nil {
			t.Fatalf("invalid operation body accepted: %s", value.operation)
		}
	}
	if _, err := decodeManagement(httptest.NewRequest("POST", "/", strings.NewReader(`{"pageNumber":1,"showNumber":20,"keyword":"群"}`)), "groups/search"); err != nil {
		t.Fatal(err)
	}
}
