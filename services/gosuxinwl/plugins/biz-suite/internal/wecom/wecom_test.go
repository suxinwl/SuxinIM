package wecom

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestNormalizeStatus(t *testing.T) {
	for input, want := range map[string]string{"1": "pending", "2": "approved", "3": "rejected", "4": "canceled", "6": "canceled", "7": "canceled", "10": "approved", "": "unknown"} {
		if got := normalizeStatus(input); got != want {
			t.Fatalf("normalizeStatus(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestAccessTokenCachesUntilExpiry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":0,"access_token":"token-1","expires_in":7200}`))
	}))
	defer server.Close()
	c := &Client{HTTP: server.Client(), CorpID: "corp", AgentID: "agent", ContactSecret: "contact-secret", AppSecret: "secret", BaseURL: server.URL}
	first, err := c.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first != "token-1" || second != first {
		t.Fatalf("unexpected tokens: %q %q", first, second)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one token request, got %d", calls.Load())
	}
}
