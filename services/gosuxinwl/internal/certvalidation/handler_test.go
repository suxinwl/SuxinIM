package certvalidation

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestPublicChallenge(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".well-known", "pki-validation")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(dir, "fileauth.txt")
	if err := os.WriteFile(filename, []byte("exact-challenge"), 0600); err != nil {
		t.Fatal(err)
	}
	handler := Handler(root)
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		r := httptest.NewRequest(method, "https://kf.szputy.com"+Path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 || w.Header().Get("Location") != "" || w.Header().Get("Content-Length") != "15" {
			t.Fatalf("%s: status=%d headers=%v", method, w.Code, w.Header())
		}
		if method == http.MethodGet && w.Body.String() != "exact-challenge" {
			t.Fatalf("challenge was transformed: %q", w.Body.String())
		}
		if method == http.MethodHead && w.Body.Len() != 0 {
			t.Fatal("HEAD returned a body")
		}
	}
	// Renewed challenge is available without restarting the host.
	if err := os.WriteFile(filename, []byte("renewed"), 0600); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, Path, nil))
	if w.Code != 200 || w.Body.String() != "renewed" {
		t.Fatalf("renewal: %d %q", w.Code, w.Body.String())
	}
}

func TestChallengeDoesNotExposeOtherFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	handler := Handler(root)
	for _, path := range []string{Path, "/.well-known/", "/.well-known/pki-validation/", "/.well-known/pki-validation/../../config.yaml", "/config.yaml"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != 404 {
			t.Fatalf("%s: got %d", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodPost, Path, nil))
	if w.Code != 405 || w.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST: %d %v", w.Code, w.Header())
	}
}
