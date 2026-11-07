package adminweb

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdminWebHistory(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{"index.html": "<html>Suxin SPA</html>", "app.js": "console.log('Suxin')"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	handler := Handler(root)
	for _, tc := range []struct {
		name, method, target, accept string
		status                       int
		body                         string
	}{
		{"entry", "GET", "/suxinweb/", "text/html", 200, "Suxin SPA"},
		{"deep link", "GET", "/suxinweb/datacenter/configuration?tab=upload", "text/html", 200, "Suxin SPA"},
		{"head", "HEAD", "/suxinweb/home", "text/html", 200, ""},
		{"asset", "GET", "/suxinweb/app.js", "*/*", 200, "console.log"},
		{"missing asset", "GET", "/suxinweb/assets/missing.js", "text/html", 404, ""},
		{"extensionless asset", "GET", "/suxinweb/assets/missing", "text/html", 404, ""},
		{"api", "GET", "/admin/user/info", "text/html", 404, ""},
		{"non navigation", "GET", "/suxinweb/missing", "application/json", 404, ""},
		{"post", "POST", "/suxinweb/home", "text/html", 405, ""},
		{"slash", "GET", "/suxinweb?q=1", "text/html", 308, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.target, nil)
			r.Header.Set("Accept", tc.accept)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.body) {
				t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
			}
			if tc.name == "head" && w.Body.Len() != 0 {
				t.Fatal("HEAD returned a body")
			}
			if tc.name == "slash" && w.Header().Get("Location") != "/suxinweb/?q=1" {
				t.Fatal("redirect lost query")
			}
		})
	}
}
