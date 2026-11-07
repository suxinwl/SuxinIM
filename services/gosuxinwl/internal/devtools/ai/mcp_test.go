package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

type rpcReply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  map[string]any  `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func invokeMCP(m *MCP, token, body string, headers map[string]string) (*httptest.ResponseRecorder, rpcReply) {
	r := httptest.NewRequest(http.MethodPost, "http://devtools.example/mcp", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json")
	if token != "" {
		r.Header.Set("X-Devtools-Token", token)
	}
	for key, value := range headers {
		r.Header.Set(key, value)
	}
	w := httptest.NewRecorder()
	m.ServeHTTP(w, r)
	var result rpcReply
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	return w, result
}

func requestBody(method string, params any) string {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	return string(b)
}

func mustToken(t *testing.T, s *Service, scopes []string) (Token, string) {
	t.Helper()
	metadata, raw, err := s.CreateToken(17, "MCP test", scopes, 1)
	if err != nil {
		t.Fatal(err)
	}
	return metadata, raw
}

func TestMCPInitializeScopedToolsCallsPromptsAndNoReadSideEffects(t *testing.T) {
	s := testService(t)
	_, raw := mustToken(t, s, []string{"codegen:read"})
	var readCalls, deniedCalls, authorizations atomic.Int32
	m := NewMCP(s, func(ctx context.Context, uid int64) error {
		authorizations.Add(1)
		if uid != 17 {
			return errors.New("wrong UID")
		}
		return nil
	}, []Tool{
		{Name: "codegen_history", Scope: "codegen:read", Description: "历史", InputSchema: map[string]any{"type": "object"}, Call: func(ctx context.Context, uid int64, args json.RawMessage) (any, error) {
			readCalls.Add(1)
			if uid != 17 {
				return nil, errors.New("wrong UID")
			}
			return map[string]any{"items": []any{}}, nil
		}},
		{Name: "ai_generate", Scope: "ai:generate", Description: "AI", InputSchema: map[string]any{"type": "object"}, Call: func(ctx context.Context, uid int64, args json.RawMessage) (any, error) {
			deniedCalls.Add(1)
			return "should not run", nil
		}},
	})
	before := fileSnapshot(t, s.root)
	w, response := invokeMCP(m, raw, requestBody("initialize", map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test-client", "version": "1.0"}}), nil)
	if w.Code != 200 || response.JSONRPC != "2.0" || response.Error != nil || response.Result["protocolVersion"] != "2025-11-25" {
		t.Fatalf("initialize: %d %s", w.Code, w.Body)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("MCP result caching enabled")
	}
	w, response = invokeMCP(m, raw, requestBody("tools/list", map[string]any{}), nil)
	tools, ok := response.Result["tools"].([]any)
	if w.Code != 200 || !ok || len(tools) != 1 || tools[0].(map[string]any)["name"] != "codegen_history" {
		t.Fatalf("scoped tools/list: %s", w.Body)
	}
	if _, exists := tools[0].(map[string]any)["scope"]; exists {
		t.Fatal("private tool fields serialized")
	}
	w, response = invokeMCP(m, raw, requestBody("tools/call", map[string]any{"name": "codegen_history", "arguments": map[string]any{}}), nil)
	if w.Code != 200 || response.Error != nil || response.Result["isError"] != false || readCalls.Load() != 1 {
		t.Fatalf("read call: %s", w.Body)
	}
	w, response = invokeMCP(m, raw, requestBody("tools/call", map[string]any{"name": "ai_generate", "arguments": map[string]any{}}), nil)
	if response.Error != nil || response.Result["isError"] != true || deniedCalls.Load() != 0 {
		t.Fatalf("denied scope executed tool: %s", w.Body)
	}
	w, response = invokeMCP(m, raw, requestBody("prompts/list", map[string]any{}), nil)
	if w.Code != 200 || response.Error != nil || len(response.Result["prompts"].([]any)) != 3 {
		t.Fatalf("prompts list: %s", w.Body)
	}
	w, response = invokeMCP(m, raw, requestBody("prompts/get", map[string]any{"name": "review", "arguments": map[string]string{"input": "查看这段代码"}}), nil)
	if response.Error != nil || !strings.Contains(w.Body.String(), "查看这段代码") {
		t.Fatalf("prompt get: %s", w.Body)
	}
	if authorizations.Load() != 6 {
		t.Fatalf("authorization not checked on every request: %d", authorizations.Load())
	}
	after := fileSnapshot(t, s.root)
	if !equalSnapshot(before, after) {
		t.Fatal("read-only MCP methods modified persisted state")
	}
}

func fileSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		result[relative] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func equalSnapshot(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestMCPTokenRevocationAndLiveAuthorizationFailures(t *testing.T) {
	s := testService(t)
	metadata, raw := mustToken(t, s, nil)
	var enabled atomic.Bool
	enabled.Store(true)
	m := NewMCP(s, func(ctx context.Context, uid int64) error {
		if !enabled.Load() {
			return errors.New("administrator disabled with private information")
		}
		return nil
	}, nil)
	body := requestBody("ping", map[string]any{})
	for _, token := range []string{"", "wrong-token", raw + "bad"} {
		w, _ := invokeMCP(m, token, body, nil)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("missing/invalid token status=%d", w.Code)
		}
	}
	w, _ := invokeMCP(m, raw, body, nil)
	if w.Code != 200 {
		t.Fatal("valid token rejected", w.Code)
	}
	enabled.Store(false)
	w, _ = invokeMCP(m, raw, body, nil)
	if w.Code != 403 || strings.Contains(w.Body.String(), "private information") {
		t.Fatal("live authorization failure not sanitized", w.Code, w.Body)
	}
	enabled.Store(true)
	if err := s.Revoke(17, metadata.ID); err != nil {
		t.Fatal(err)
	}
	w, _ = invokeMCP(m, raw, body, nil)
	if w.Code != 401 {
		t.Fatal("revoked MCP token accepted")
	}
	_, raw = mustToken(t, s, nil)
	m = NewMCP(s, nil, nil)
	w, _ = invokeMCP(m, raw, body, nil)
	if w.Code != 403 {
		t.Fatal("missing authorizer did not fail closed")
	}
}

func TestMCPOriginProtocolContentAndRequestValidation(t *testing.T) {
	s := testService(t)
	_, raw := mustToken(t, s, nil)
	m := NewMCP(s, func(context.Context, int64) error { return nil }, nil)
	for _, test := range []struct {
		name    string
		headers map[string]string
		status  int
	}{
		{"foreign_origin", map[string]string{"Origin": "https://attacker.example"}, 403},
		{"null_origin", map[string]string{"Origin": "null"}, 403},
		{"bad_protocol", map[string]string{"MCP-Protocol-Version": "1900-01-01"}, 400},
		{"bad_content_type", map[string]string{"Content-Type": "text/plain"}, 415},
		{"bad_accept", map[string]string{"Accept": "text/event-stream"}, 406},
	} {
		t.Run(test.name, func(t *testing.T) {
			w, _ := invokeMCP(m, raw, requestBody("ping", map[string]any{}), test.headers)
			if w.Code != test.status {
				t.Fatalf("status=%d expected=%d body=%s", w.Code, test.status, w.Body)
			}
		})
	}
	w, _ := invokeMCP(m, raw, requestBody("ping", map[string]any{}), map[string]string{"Origin": "http://devtools.example", "MCP-Protocol-Version": "2025-06-18"})
	if w.Code != 200 {
		t.Fatal("same-origin supported protocol rejected")
	}
	for _, test := range []struct {
		name, body string
		code       int
	}{
		{"parse_error", `{"jsonrpc":`, -32700},
		{"wrong_version", `{"jsonrpc":"1.0","id":1,"method":"ping"}`, -32600},
		{"missing_method", `{"jsonrpc":"2.0","id":1}`, -32600},
		{"missing_id", `{"jsonrpc":"2.0","method":"ping"}`, -32600},
		{"invalid_id", `{"jsonrpc":"2.0","id":true,"method":"ping"}`, -32600},
		{"unknown_method", requestBody("server/unknown", map[string]any{}), -32601},
		{"invalid_initialize", requestBody("initialize", map[string]any{"unexpected": true}), -32602},
		{"unknown_tool", requestBody("tools/call", map[string]any{"name": "missing"}), -32602},
		{"invalid_tool_params", requestBody("tools/call", map[string]any{"name": "missing", "unexpected": true}), -32602},
		{"unknown_prompt", requestBody("prompts/get", map[string]any{"name": "missing", "arguments": map[string]string{"input": "hello"}}), -32602},
	} {
		t.Run(test.name, func(t *testing.T) {
			w, result := invokeMCP(m, raw, test.body, nil)
			if w.Code != 200 || result.Error == nil || result.Error.Code != test.code {
				t.Fatalf("error=%+v expected=%d body=%s", result.Error, test.code, w.Body)
			}
		})
	}
	w, _ = invokeMCP(m, raw, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil)
	if w.Code != 202 {
		t.Fatal("notification was not acknowledged")
	}
	r := httptest.NewRequest(http.MethodGet, "http://devtools.example/mcp", nil)
	r.Header.Set("X-Devtools-Token", raw)
	w = httptest.NewRecorder()
	m.ServeHTTP(w, r)
	if w.Code != 405 || w.Header().Get("Allow") != "POST" {
		t.Fatal("MCP GET incorrectly supported")
	}
	w, _ = invokeMCP(m, raw, strings.Repeat("x", (1<<20)+1), nil)
	if w.Code != 413 {
		t.Fatal("oversized RPC body accepted")
	}
}

func TestMCPToolErrorsAndUnencodableResultsAreErrors(t *testing.T) {
	s := testService(t)
	_, raw := mustToken(t, s, []string{"codegen:read"})
	m := NewMCP(s, func(context.Context, int64) error { return nil }, []Tool{
		{Name: "failure", Scope: "codegen:read", Call: func(context.Context, int64, json.RawMessage) (any, error) { return nil, errors.New("preview rejected") }},
		{Name: "unencodable", Scope: "codegen:read", Call: func(context.Context, int64, json.RawMessage) (any, error) { return make(chan int), nil }},
	})
	for _, name := range []string{"failure", "unencodable"} {
		w, result := invokeMCP(m, raw, requestBody("tools/call", map[string]any{"name": name, "arguments": map[string]any{}}), nil)
		if w.Code != 200 || result.Error != nil || result.Result["isError"] != true {
			t.Fatalf("%s should produce isError=true: %s", name, w.Body)
		}
	}
}

func TestMCPRateLimitIsPerToken(t *testing.T) {
	s := testService(t)
	_, raw := mustToken(t, s, nil)
	m := NewMCP(s, func(context.Context, int64) error { return nil }, nil)
	for i := 0; i < 60; i++ {
		w, _ := invokeMCP(m, raw, requestBody("ping", map[string]any{}), nil)
		if w.Code != 200 {
			t.Fatalf("request %d unexpectedly limited: %d", i, w.Code)
		}
	}
	w, _ := invokeMCP(m, raw, requestBody("ping", map[string]any{}), nil)
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal("MCP limit did not reject excess requests")
	}
	_, secondRaw := mustToken(t, s, nil)
	w, _ = invokeMCP(m, secondRaw, requestBody("ping", map[string]any{}), nil)
	if w.Code != 200 {
		t.Fatal("one token's rate limit blocked another token")
	}
}

func TestMCPInitializeRequiredFieldsAndLegalMeta(t *testing.T) {
	s := testService(t)
	_, raw := mustToken(t, s, []string{"codegen:read"})
	m := NewMCP(s, func(context.Context, int64) error { return nil }, []Tool{{
		Name: "read", Scope: "codegen:read", Call: func(_ context.Context, uid int64, arguments json.RawMessage) (any, error) {
			return map[string]any{"uid": uid}, nil
		},
	}})
	for _, test := range []struct {
		name   string
		params map[string]any
	}{
		{"missing_protocol", map[string]any{"capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1.0"}}},
		{"missing_capabilities", map[string]any{"protocolVersion": "2025-11-25", "clientInfo": map[string]any{"name": "test", "version": "1.0"}}},
		{"missing_client_info", map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}}},
		{"missing_client_name", map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"version": "1.0"}}},
		{"missing_client_version", map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test"}}},
		{"non_string_client_name", map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": 7, "version": "1.0"}}},
		{"non_string_client_version", map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": true}}},
		{"empty_client_name", map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "", "version": "1.0"}}},
		{"empty_client_version", map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": ""}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			w, result := invokeMCP(m, raw, requestBody("initialize", test.params), nil)
			if w.Code != 200 || result.Error == nil || result.Error.Code != -32602 {
				t.Fatalf("required field not enforced: %s", w.Body)
			}
		})
	}
	for _, test := range []struct {
		method string
		params map[string]any
	}{
		{"initialize", map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1.0"}, "_meta": map[string]any{"progressToken": "request-1"}}},
		{"tools/call", map[string]any{"name": "read", "arguments": map[string]any{}, "_meta": map[string]any{"progressToken": 7}}},
		{"prompts/get", map[string]any{"name": "review", "arguments": map[string]string{"input": "检查代码"}, "_meta": map[string]any{"trace": "test"}}},
	} {
		t.Run(test.method+"_meta", func(t *testing.T) {
			w, result := invokeMCP(m, raw, requestBody(test.method, test.params), nil)
			if w.Code != 200 || result.Error != nil || result.Result == nil {
				t.Fatalf("legal _meta rejected: %s", w.Body)
			}
		})
	}
}

func TestMCPBearerDeveloperTokenAndConflictingHeaders(t *testing.T) {
	s := testService(t)
	_, raw := mustToken(t, s, nil)
	_, other := mustToken(t, s, nil)
	m := NewMCP(s, func(context.Context, int64) error { return nil }, nil)
	body := requestBody("ping", map[string]any{})
	for _, test := range []struct {
		name, legacy, authorization string
		status                      int
	}{
		{"standard_bearer", "", "Bearer " + raw, 200},
		{"legacy_custom", raw, "", 200},
		{"matching_dual_headers", raw, "Bearer " + raw, 200},
		{"ordinary_host_jwt", "", "Bearer eyJhbGciOiJIUzI1NiJ9.e30.invalid", 401},
		{"conflicting_developer_tokens", raw, "Bearer " + other, 401},
		{"legacy_with_foreign_bearer", raw, "Bearer eyJhbGciOiJIUzI1NiJ9.e30.invalid", 401},
	} {
		t.Run(test.name, func(t *testing.T) {
			headers := map[string]string{}
			if test.authorization != "" {
				headers["Authorization"] = test.authorization
			}
			w, response := invokeMCP(m, test.legacy, body, headers)
			if w.Code != test.status {
				t.Fatalf("status=%d expected=%d body=%s", w.Code, test.status, w.Body)
			}
			if test.status == 200 && (response.Error != nil || response.Result == nil) {
				t.Fatal("valid developer token did not produce an RPC result")
			}
		})
	}
}
