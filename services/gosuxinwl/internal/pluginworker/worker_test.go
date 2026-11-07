package pluginworker

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHostExitDuringWorkerStartup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, stopped := false, false
	err := serveUntilCancelled(ctx, func() error {
		cancel() // The host's stdin pipe closes while initialization is running.
		started = true
		return nil
	}, func() error {
		if !started {
			t.Fatal("shutdown ran before startup completed")
		}
		stopped = true
		return nil
	})
	if err != nil || !stopped {
		t.Fatal("worker must stop after startup when its host has exited", err)
	}
}

func TestWorkerDoesNotStartAfterHostExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := serveUntilCancelled(ctx, func() error {
		t.Fatal("started a worker after its host exited")
		return nil
	}, func() error { t.Fatal("shut down an unstarted worker"); return nil })
	if err != nil {
		t.Fatal(err)
	}
}

func TestWorkerStartupFailureDoesNotWaitForHost(t *testing.T) {
	want := errors.New("listener unavailable")
	err := serveUntilCancelled(context.Background(), func() error { return want }, func() error {
		t.Fatal("shutdown called after failed startup")
		return nil
	})
	if !errors.Is(err, want) {
		t.Fatal(err)
	}
}

func TestOriginalHTTPSRequiresAuthenticatedHost(t *testing.T) {
	secret := strings.Repeat("a", 48)
	for _, valid := range []bool{false, true} {
		r := httptest.NewRequest("GET", "http://127.0.0.1/inventory", nil)
		r.Header.Set("X-Suxin-Plugin-HTTPS", "1")
		r.Header.Set("X-Suxin-Plugin-Remote-Addr", "203.0.113.8:32100")
		if valid {
			r.Header.Set("X-Suxin-Plugin-Secret", secret)
		}
		if authenticateHostRequest(r, secret) != valid || (r.TLS != nil) != valid {
			t.Fatal("original HTTPS state must only be accepted from the authenticated host")
		}
		if (r.RemoteAddr == "203.0.113.8:32100") != valid {
			t.Fatal("client address must only be restored from the authenticated host")
		}
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1/inventory", nil)
	r.Header.Set("X-Suxin-Plugin-Secret", secret)
	if !authenticateHostRequest(r, secret) || r.TLS != nil {
		t.Fatal("plain HTTP must remain plain HTTP")
	}
}
