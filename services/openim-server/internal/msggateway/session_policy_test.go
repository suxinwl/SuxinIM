package msggateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"openim38.local/identity"
)

func TestEveryWebsocketFrameChecksCurrentLogicalPolicyBeforeDecoding(t *testing.T) {
	t.Setenv("HRM_SSO_REQUIRED", "true")
	r := miniredis.RunT(t)
	t.Setenv("HRM_REDIS_URL", "redis://"+r.Addr()+"/2")
	t.Setenv("HRM_BRIDGE_HMAC_KEY", strings.Repeat("k", 32))
	allowed := true
	epoch := 1
	calls := 0
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["logicalClient"] != "web_mobile" || body["platform"] != float64(5) {
			t.Error("websocket mobile client changed category")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"errCode": 0, "data": map[string]any{"allowed": allowed, "securityVersion": epoch}})
	}))
	defer policy.Close()
	t.Setenv("HRM_ACCESS_CHECK_URL", policy.URL+"/internal/im-access/check")
	state, err := identity.Global()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_ = state.PutProjection(ctx, identity.Projection{UserID: "emp_frame_fixture", Enabled: true, Version: 1})
	v := identity.Session{UserID: "emp_frame_fixture", SessionID: "sid-frame-fixture-000001", Platform: 5, LogicalClient: "web_mobile", Epoch: 1, ExpiresAt: time.Now().Add(time.Hour).Unix()}
	if _, err = state.Exchange(ctx, v, "chat", "im"); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "https://192.168.10.11:9885/ws", nil)
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("X-Real-IP", "192.0.2.1")
	c := &Client{UserID: v.UserID, PlatformID: 5, token: "im", hbCtx: ctx, ctx: &UserConnContext{Req: request}, Encoder: NewJsonEncoder()}
	// Invalid JSON is reached only after a successful live policy decision.
	if err = c.handleMessage([]byte("invalid JSON")); err == nil || err == ErrClientClosed {
		t.Fatal("allowed frame did not reach decoder")
	}
	allowed = false
	if err = c.handleMessage(nil); err != ErrClientClosed {
		t.Fatal("denied frame reached decoder/RPC")
	}
	allowed = true
	epoch = 2
	if err = c.handleMessage(nil); err != ErrClientClosed {
		t.Fatal("SQL security change waited for outbox/monitor")
	}
	if calls != 3 {
		t.Fatal("websocket frame policy was cached")
	}
}
