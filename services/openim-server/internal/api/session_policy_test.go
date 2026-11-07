package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/openimsdk/protocol/auth"
	"google.golang.org/grpc"
	"openim38.local/identity"
)

type policyParseClient struct{ auth.AuthClient }

func (policyParseClient) ParseToken(context.Context, *auth.ParseTokenReq, ...grpc.CallOption) (*auth.ParseTokenResp, error) {
	return &auth.ParseTokenResp{UserID: "emp_parse_fixture", PlatformID: 1}, nil
}

func TestParseTokenCannotBypassCurrentPolicyOrSQLSecurityVersion(t *testing.T) {
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
		if body["logicalClient"] != "ios" || body["platform"] != float64(1) || body["clientIP"] != "192.0.2.1" {
			t.Error("wrong current platform/address")
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
	_ = state.PutProjection(ctx, identity.Projection{UserID: "emp_parse_fixture", Enabled: true, Version: 1})
	v := identity.Session{UserID: "emp_parse_fixture", SessionID: "sid-parse-fixture-000001", Platform: 1, Epoch: 1, ExpiresAt: time.Now().Add(time.Hour).Unix()}
	if _, err = state.Exchange(ctx, v, "chat", "im"); err != nil {
		t.Fatal(err)
	}
	api := NewAuthApi(policyParseClient{})
	read := func() int {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/auth/parse_token", strings.NewReader(`{"token":"im"}`))
		c.Request.RemoteAddr = "127.0.0.1:1234"
		c.Request.Header.Set("X-Real-IP", "192.0.2.1")
		api.ParseToken(c)
		var out struct{ ErrCode int }
		if json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatal("invalid API response")
		}
		return out.ErrCode
	}
	if code := read(); code != 0 {
		t.Fatal("allowed iOS token denied", code)
	}
	allowed = false
	if code := read(); code == 0 {
		t.Fatal("current denied policy retained native parse success")
	}
	allowed = true
	epoch = 2
	if code := read(); code == 0 {
		t.Fatal("SQL epoch advance retained native parse success")
	}
	if calls != 3 {
		t.Fatal("policy response was cached")
	}
}
