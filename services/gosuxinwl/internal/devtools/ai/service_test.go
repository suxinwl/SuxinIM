package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testService(t *testing.T) *Service {
	t.Helper()
	s, err := NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestConfigEncryptionKeyRetentionClearAndUIDIsolation(t *testing.T) {
	s := testService(t)
	const secret = "sk-test-private-not-in-storage-or-output"
	config := Config{Endpoint: "https://ai.example.invalid/v1/", Model: "test-model", APIKey: secret}
	if err := s.SaveConfig(1, config, false); err != nil {
		t.Fatal(err)
	}
	public, err := s.PublicConfig(1)
	if err != nil || public["keyConfigured"] != true || public["endpoint"] != "https://ai.example.invalid/v1" {
		t.Fatalf("public config: %+v %v", public, err)
	}
	encoded, _ := json.Marshal(public)
	if strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), "sealedKey") || strings.Contains(string(encoded), "apiKey") {
		t.Fatal("public config exposed private material")
	}
	name, _ := s.configPath(1)
	disk, err := os.ReadFile(name)
	if err != nil || strings.Contains(string(disk), secret) || strings.Contains(string(disk), `"apiKey"`) {
		t.Fatalf("plaintext API key persisted: %s %v", disk, err)
	}
	stored, err := s.readConfig(1)
	if err != nil || stored.APIKey != "" || stored.SealedKey == "" {
		t.Fatal("encrypted config missing")
	}
	decrypted, err := s.unseal(stored.SealedKey)
	if err != nil || decrypted != secret {
		t.Fatal("encrypted key cannot be restored", err)
	}
	other, err := s.PublicConfig(2)
	if err != nil || other["keyConfigured"] != false || other["endpoint"] != "" {
		t.Fatal("UID config isolation failed", other, err)
	}
	config.APIKey = ""
	config.Model = "second-model"
	config.SealedKey = "caller-cannot-inject-ciphertext"
	if err := s.SaveConfig(1, config, false); err != nil {
		t.Fatal(err)
	}
	retained, _ := s.readConfig(1)
	if retained.SealedKey != stored.SealedKey || retained.Model != "second-model" {
		t.Fatal("empty key must retain previous encrypted key")
	}
	reopened, err := NewService(s.root)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err = reopened.unseal(retained.SealedKey)
	if err != nil || decrypted != secret {
		t.Fatal("persisted key cannot be opened after restart", err)
	}
	if err := s.SaveConfig(1, config, true); err != nil {
		t.Fatal(err)
	}
	cleared, _ := s.PublicConfig(1)
	if cleared["keyConfigured"] != false {
		t.Fatal("explicit clear did not remove encrypted API key")
	}
	config.APIKey = "sk-replacement"
	if err := s.SaveConfig(1, config, false); err != nil {
		t.Fatal(err)
	}
	replacement, _ := s.readConfig(1)
	decrypted, err = s.unseal(replacement.SealedKey)
	if err != nil || decrypted != "sk-replacement" {
		t.Fatal("key replacement failed", err)
	}
	for _, id := range []int64{0, -1} {
		if _, err := s.PublicConfig(id); err == nil {
			t.Fatal("invalid UID accepted")
		}
		if err := s.SaveConfig(id, config, false); err == nil {
			t.Fatal("invalid UID configuration write accepted")
		}
	}
}

func TestDeveloperTokensStoreOnlyHashesRespectUIDScopeExpiryAndRevocation(t *testing.T) {
	s := testService(t)
	metadata, raw, err := s.CreateToken(1, "编辑器", nil, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(raw, "sxdev_") || len(raw) < 40 || metadata.UID != 1 || !metadata.Allows("codegen:read") || !metadata.Allows("excel:read") || metadata.Allows("ai:generate") {
		t.Fatal("unexpected default permissions")
	}
	hash := sha256.Sum256([]byte(raw))
	if metadata.ID != hex.EncodeToString(hash[:]) {
		t.Fatal("token ID is not the bearer-token digest")
	}
	name := filepath.Join(s.root, "tokens", metadata.ID+".json")
	bytes, err := os.ReadFile(name)
	if err != nil || strings.Contains(string(bytes), raw) {
		t.Fatal("raw developer token persisted")
	}
	validated, err := s.ValidateToken(raw)
	if err != nil || validated.ID != metadata.ID || validated.UID != 1 {
		t.Fatal("valid developer token rejected", err)
	}
	if _, err := s.ValidateToken(raw + "tampered"); err == nil {
		t.Fatal("modified developer token accepted")
	}
	if err := s.Revoke(2, metadata.ID); err == nil {
		t.Fatal("another UID revoked a token")
	}
	if _, err := s.ValidateToken(raw); err != nil {
		t.Fatal("failed cross-UID revoke changed token", err)
	}
	other, err := s.Tokens(2)
	if err != nil || len(other) != 0 {
		t.Fatal("token listing leaked across UIDs")
	}
	listed, err := s.Tokens(1)
	if err != nil || len(listed) != 1 || listed[0].ID != metadata.ID {
		t.Fatal("token listing omitted token")
	}
	listedJSON, _ := json.Marshal(listed)
	if strings.Contains(string(listedJSON), raw) {
		t.Fatal("token listing exposed bearer token")
	}
	if err := s.Revoke(1, metadata.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ValidateToken(raw); err == nil {
		t.Fatal("revoked token accepted")
	}
	expiring, expiredRaw, err := s.CreateToken(1, "过期测试", []string{"ai:generate"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !expiring.Allows("ai:generate") || expiring.Allows("codegen:read") {
		t.Fatal("explicit scopes ignored")
	}
	expiring.ExpiresAt = time.Now().Add(-time.Minute)
	if err := atomicJSON(filepath.Join(s.root, "tokens", expiring.ID+".json"), expiring); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ValidateToken(expiredRaw); err == nil {
		t.Fatal("expired token accepted")
	}
	for _, id := range []string{"../outside", strings.Repeat("a", 63), strings.Repeat("g", 64)} {
		if err := s.Revoke(1, id); err == nil {
			t.Fatal("invalid token path accepted")
		}
	}
	for _, test := range []struct {
		uid    int64
		name   string
		scopes []string
		days   int
	}{
		{0, "invalid", nil, 1}, {1, "", nil, 1}, {1, "valid", nil, 0}, {1, "valid", nil, 31},
		{1, "valid", []string{"codegen:write"}, 1}, {1, "valid", []string{"codegen:read", "codegen:read"}, 1},
	} {
		if _, _, err := s.CreateToken(test.uid, test.name, test.scopes, test.days); err == nil {
			t.Fatalf("invalid token request accepted: %+v", test)
		}
	}
}

func TestAIEndpointPrivateNetworkPolicyAndValidation(t *testing.T) {
	for _, endpoint := range []string{
		"http://api.example.invalid/v1", "https://localhost/v1", "https://127.0.0.1/v1", "https://[::1]/v1", "https://[::]/v1", "https://0.0.0.0/v1", "https://10.1.2.3/v1", "https://169.254.169.254/v1", "https://192.168.10.8/v1",
	} {
		if _, err := validateEndpoint(endpoint, false); err == nil {
			t.Fatalf("private or insecure endpoint accepted: %s", endpoint)
		}
	}
	for _, endpoint := range []string{"ftp://host/v1", "https://user:password@host/v1", "https://host/v1?token=secret", "https://host/v1#secret", "https:///v1"} {
		if _, err := validateEndpoint(endpoint, true); err == nil {
			t.Fatalf("invalid endpoint accepted: %s", endpoint)
		}
	}
	if _, err := validateEndpoint("http://127.0.0.1:1234/v1", true); err != nil {
		t.Fatal("explicit local endpoint rejected", err)
	}
	if _, err := validateEndpoint("https://api.example.invalid/v1", false); err != nil {
		t.Fatal("HTTPS public endpoint rejected", err)
	}
	for _, value := range []string{"127.0.0.1", "::1", "10.0.0.1", "192.168.1.1", "172.16.0.1", "169.254.0.1", "0.0.0.0", "224.0.0.1", "fe80::1"} {
		if !isPrivate(net.ParseIP(value)) {
			t.Fatalf("non-public address not detected: %s", value)
		}
	}
}

func TestAIChatCompletionsSuccessAndPromptContract(t *testing.T) {
	s := testService(t)
	const key = "sk-httptest-secret"
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer "+key || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("incorrect AI request method/path/auth")
		}
		var payload struct {
			Model    string                           `json:"model"`
			Stream   bool                             `json:"stream"`
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("invalid AI payload: %v", err)
		}
		if payload.Model != "local-test" || payload.Stream || len(payload.Messages) != 2 || payload.Messages[0].Role != "system" || payload.Messages[0].Content != Instructions["schema"] || payload.Messages[1].Role != "user" || payload.Messages[1].Content != "生成图书字段" {
			t.Errorf("incorrect AI prompt contract: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"local-test","choices":[{"message":{"content":"{\"module\":\"book_review\"}"}}],"usage":{"total_tokens":17}}`))
	}))
	defer server.Close()
	if err := s.SaveConfig(1, Config{Endpoint: server.URL + "/v1", Model: "local-test", APIKey: key, AllowPrivate: true}, false); err != nil {
		t.Fatal(err)
	}
	result, err := s.Generate(context.Background(), 1, "schema", "  生成图书字段  ")
	if err != nil || result["text"] != `{"module":"book_review"}` || result["model"] != "local-test" || calls.Load() != 1 {
		t.Fatalf("AI completion failed: %+v %v", result, err)
	}
	if _, err := s.Generate(context.Background(), 2, "schema", "生成字段"); err == nil {
		t.Fatal("another UID used the configured AI provider")
	}
	if calls.Load() != 1 {
		t.Fatal("cross-UID call reached provider")
	}
	for _, test := range []struct{ mode, prompt string }{{"unknown", "prompt"}, {"schema", ""}, {"schema", strings.Repeat("x", 32769)}} {
		if _, err := s.Generate(context.Background(), 1, test.mode, test.prompt); err == nil {
			t.Fatal("invalid AI generation input accepted")
		}
	}
}

func TestAIProviderErrorsAreSanitizedAndRedirectsAreNotFollowed(t *testing.T) {
	const secret = "sk-provider-credential-never-echo"
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{"provider_error", http.StatusUnauthorized, `{"error":{"message":"` + secret + `"}}`},
		{"invalid_json", http.StatusOK, "not JSON " + secret},
		{"no_choices", http.StatusOK, `{"choices":[]}`},
		{"empty_content", http.StatusOK, `{"choices":[{"message":{"content":""}}]}`},
		{"oversized", http.StatusOK, strings.Repeat("x", (2<<20)+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := testService(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			if err := s.SaveConfig(1, Config{Endpoint: server.URL, Model: "test", APIKey: secret, AllowPrivate: true}, false); err != nil {
				t.Fatal(err)
			}
			_, err := s.Generate(context.Background(), 1, "review", "review source")
			if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), server.URL) {
				t.Fatal("provider failure missing or exposed credentials/address", err)
			}
		})
	}
	t.Run("redirect", func(t *testing.T) {
		s := testService(t)
		var targetCalls atomic.Int32
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1); w.WriteHeader(200) }))
		defer target.Close()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
		defer server.Close()
		if err := s.SaveConfig(1, Config{Endpoint: server.URL, Model: "test", APIKey: secret, AllowPrivate: true}, false); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Generate(context.Background(), 1, "review", "review"); err == nil {
			t.Fatal("redirect was accepted")
		}
		if targetCalls.Load() != 0 {
			t.Fatal("provider redirect followed and could expose API key")
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		s := testService(t)
		if err := s.SaveConfig(1, Config{Endpoint: "http://127.0.0.1:1", Model: "test", APIKey: secret, AllowPrivate: true}, false); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := s.Generate(ctx, 1, "review", "review"); err == nil || strings.Contains(err.Error(), secret) {
			t.Fatal("cancelled provider call not sanitized", err)
		}
	})
}

func TestAIConcurrencyLimitDoesNotQueueUnboundedRequests(t *testing.T) {
	s := testService(t)
	entered, release := make(chan struct{}, 2), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"done"}}]}`))
	}))
	defer server.Close()
	defer close(release)
	if err := s.SaveConfig(1, Config{Endpoint: server.URL, Model: "test", AllowPrivate: true}, false); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := s.Generate(context.Background(), 1, "review", "review"); results <- err }()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("provider requests did not enter")
		}
	}
	if _, err := s.Generate(context.Background(), 1, "review", "third call"); err == nil {
		t.Fatal("third concurrent generation entered the provider")
	}
}
