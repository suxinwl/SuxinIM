package runtimeplugin

import (
	"net/http/httptest"
	"testing"
)

func TestForwardHostHTTPSOverridesPublicHeader(t *testing.T) {
	for _, scheme := range []string{"http", "https"} {
		r := httptest.NewRequest("GET", scheme+"://plugins.example/inventory", nil)
		r.Header.Set("X-Suxin-Plugin-HTTPS", "1")
		r.Header.Set("X-Suxin-Plugin-Remote-Addr", "203.0.113.99:12345")
		forwardHostHTTPS(r)
		if r.Header.Get("X-Suxin-Plugin-Remote-Addr") != r.RemoteAddr {
			t.Fatal("a public header must not spoof the connecting client address")
		}
		if (r.Header.Get("X-Suxin-Plugin-HTTPS") == "1") != (scheme == "https") {
			t.Fatal("a public transport flag must not override the actual host connection")
		}
	}
}
