package imbridge

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrivateBridgeSignsExactBodyAndOmitsHRMPrivateFields(t *testing.T) {
	key := strings.Repeat("test-key-", 5)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		b, e := io.ReadAll(r.Body)
		if e != nil {
			t.Error(e)
		}
		bodyHash := sha256.Sum256(b)
		canonical := r.Header.Get("X-HRM-Timestamp") + "\n" + r.Header.Get("X-HRM-Nonce") + "\nPOST\n" + r.URL.Path + "\n" + hex.EncodeToString(bodyHash[:])
		h := hmac.New(sha256.New, []byte(key))
		h.Write([]byte(canonical))
		if !hmac.Equal([]byte(hex.EncodeToString(h.Sum(nil))), []byte(r.Header.Get("X-HRM-Signature"))) {
			t.Error("invalid HMAC")
		}
		var v map[string]any
		if e = json.Unmarshal(b, &v); e != nil {
			t.Error(e)
		}
		if _, ok := v["authFingerprint"]; ok {
			t.Error("private HRM password fingerprint crossed bridge")
		}
		if _, ok := v["profile"]; ok {
			t.Error("unexpected private profile wire field")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"errCode":0,"data":{"appliedVersion":1}}`)
	}))
	defer server.Close()
	remote, e := NewHTTPRemote(server.URL, key)
	must(t, e)
	must(t, remote.User(context.Background(), Projection{UserID: UserID("test"), Account: "0001", Nickname: "Test", Version: 1, AuthFingerprint: "private-hash", HostUID: 22}))
	if calls != 1 {
		t.Fatal("wrong bridge call count")
	}
	for _, raw := range []string{"https://127.0.0.1:10010", "http://192.168.10.11:10010", "http://user:pass@127.0.0.1:10010", "http://127.0.0.1:10010?key=unsafe"} {
		if _, e = NewHTTPRemote(raw, key); e == nil {
			t.Fatalf("unsafe private URL accepted: %s", raw)
		}
	}
}
func TestDefaultDisabledConfiguration(t *testing.T) {
	t.Setenv("GOSUXIN_IM_ENABLED", "")
	cfg := ConfigFromEnv()
	if cfg.Enabled {
		t.Fatal("integration enabled by default")
	}
	must(t, cfg.Validate())
	for name, want := range map[string]int{"android": 2, "windows": 3, "web": 5} {
		got, e := Platform(name)
		must(t, e)
		if got != want {
			t.Fatalf("incorrect SDK platform %s", name)
		}
	}
	if _, e := Platform("ios"); e == nil {
		t.Fatal("out-of-scope platform allowed")
	}
}
