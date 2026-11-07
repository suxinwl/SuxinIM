package imbridge

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDesktopCORSAllowsOpaqueOriginWithoutCredentials(t *testing.T) {
	r := httptest.NewRequest(http.MethodOptions, "/common/im-sso/login", nil)
	r.Header.Set("Origin", "null")
	r.Header.Set("Access-Control-Request-Headers", "content-type")
	r.Header.Set("Access-Control-Request-Private-Network", "true")
	w := httptest.NewRecorder()
	if ssoCORS(w, r, "") {
		t.Fatal("preflight must finish before authentication")
	}
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "null" || w.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("incorrect desktop CORS policy")
	}
	if w.Header().Get("Access-Control-Allow-Private-Network") != "true" {
		t.Fatal("private network preflight missing")
	}
}
func TestWebOriginIsRestricted(t *testing.T) {
	for _, origin := range []string{"https://evil.invalid", "https://kf.szputy.com:9999", "file://untrusted"} {
		r := httptest.NewRequest(http.MethodPost, "/common/im-sso/login", nil)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		if ssoCORS(w, r, "") || w.Code != 403 || w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("untrusted origin accepted")
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/common/im-sso/login", nil)
	r.Header.Set("Origin", "https://kf.szputy.com:9880")
	w := httptest.NewRecorder()
	if !ssoCORS(w, r, "") || w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("web same origin policy rejected")
	}
}
func TestIsolatedLoopbackSameOriginWorksOverSSHTunnel(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:19880/common/im-sso/login", nil)
	r.Header.Set("Origin", "http://127.0.0.1:19880")
	w := httptest.NewRecorder()
	if !ssoCORS(w, r, "") {
		t.Fatal("local SSH tunnel same-origin request rejected")
	}
	r.Header.Set("Origin", "http://127.0.0.1:9999")
	w = httptest.NewRecorder()
	if ssoCORS(w, r, "") {
		t.Fatal("unrelated loopback web origin accepted")
	}
}
