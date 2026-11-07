package adminsystem

import (
	"net/http"
	"testing"
)

func TestLogHeadersRemoveCredentialsWithoutChangingRequest(t *testing.T) {
	headers := http.Header{}
	for _, name := range []string{"Authorization", "X-Original-Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie", "Apiverify", "X-Auth-Token", "X-Suxin-Plugin-Secret", "X-Suxin-Plugin-User", "X-Devtools-Token"} {
		headers.Set(name, "credential")
	}
	headers.Set("Content-Type", "application/json")
	clean := redactedLogHeaders(headers)
	if len(clean) != 1 || clean.Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected logged headers: %v", clean)
	}
	if headers.Get("Authorization") != "credential" || headers.Get("Set-Cookie") != "credential" {
		t.Fatal("logging changed live request headers")
	}
}
