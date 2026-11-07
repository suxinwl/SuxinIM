package runtimeplugin

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
)

func TestRegistryRoutesGenericPlugin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("sampleplugin")) }))
	t.Cleanup(server.Close)
	target, _ := url.Parse(server.URL)
	r := &Registry{managers: map[string]*Manager{"sampleplugin": {name: "sampleplugin", dir: "installed", running: &worker{proxy: httputil.NewSingleHostReverseProxy(target)}}}}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/plugins/sampleplugin/admin/", nil))
	if w.Code != 200 || w.Body.String() != "sampleplugin" {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
	for _, path := range []string{"/", "/plugins/unknown/", "/admin/sampleplugin/"} {
		w = httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
}

func TestManagerServeHTTPWithoutProxyDoesNotPanic(t *testing.T) {
	manager := &Manager{name: "sampleplugin", dir: "installed", running: &worker{}}
	recorder := httptest.NewRecorder()
	manager.ServeHTTP(recorder, httptest.NewRequest("GET", "/plugins/sampleplugin/api/ui/dramas", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}
