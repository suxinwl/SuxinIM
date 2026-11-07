package imbridge

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSSOLogsCannotInheritQueryTicketOrConsumeBody(t *testing.T) {
	r := httptest.NewRequest("POST", "/common/im-sso/exchange?ticket=do-not-log", strings.NewReader(`{"ticket":"retain-for-handler"}`))
	r.Header.Set("Referer", "https://kf.szputy.com:9880/im/?ticket=do-not-log")
	sanitizeSSOLogURL(r)
	if strings.Contains(r.URL.String(), "do-not-log") || strings.Contains(r.RequestURI, "do-not-log") || r.Referer() != "" {
		t.Fatal("URL credentials survived log sanitizer")
	}
	body, e := io.ReadAll(r.Body)
	if e != nil || string(body) != `{"ticket":"retain-for-handler"}` {
		t.Fatal("log sanitizer changed handler credentials")
	}
}
