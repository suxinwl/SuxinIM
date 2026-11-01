package imapi

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/openimsdk/chat/pkg/common/mctx"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestForceOfflinePropagatesUpstreamFailureAndRejectsEmptyUser(t *testing.T) {
	count := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		var v struct {
			PlatformID int `json:"platformID"`
		}
		json.NewDecoder(r.Body).Decode(&v)
		w.Header().Set("Content-Type", "application/json")
		if v.PlatformID == 3 {
			fmt.Fprint(w, `{"errCode":500,"errMsg":"synthetic failure"}`)
		} else {
			fmt.Fprint(w, `{"errCode":0,"data":{}}`)
		}
	}))
	defer upstream.Close()
	caller := New(upstream.URL, "secret", "admin")
	ctx := mctx.WithApiToken(context.Background(), "test-token")
	if err := caller.ForceOffLine(ctx, ""); err == nil || count != 0 {
		t.Fatal("empty user accepted")
	}
	if err := caller.ForceOffLine(ctx, "fixture"); err == nil || count < 3 {
		t.Fatal("upstream failure silently swallowed")
	}
}
