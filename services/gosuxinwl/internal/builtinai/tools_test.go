package builtinai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/suxinwl/GoSuxin/internal/devtools/ai"
	"github.com/suxinwl/GoSuxin/internal/runtimeplugin"
)

func testAIService(t *testing.T) *ai.Service {
	t.Helper()
	s, err := ai.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func toolsByName(tools []ai.Tool) map[string]ai.Tool {
	result := map[string]ai.Tool{}
	for _, tool := range tools {
		result[tool.Name] = tool
	}
	return result
}

const previewArguments = `{"module":"book_review","table":"gf_book_review","title":"图书评审","fields":[{"name":"title","label":"名称","type":"string","required":true,"length":120}]}`
const detailID = "20261010120000123456-0011223344556677"

func TestBuiltinToolsPublishSixScopedObjectSchemas(t *testing.T) {
	tools := Tools(testAIService(t), func(context.Context, int64, string, string, string, json.RawMessage) (any, error) { return nil, nil })
	if len(tools) != 6 {
		t.Fatalf("expected six tools, got %d", len(tools))
	}
	expected := map[string]string{"codegen_preview": "codegen:read", "codegen_history": "codegen:read", "codegen_detail": "codegen:read", "excel_templates": "excel:read", "excel_validate": "excel:read", "ai_generate": "ai:generate"}
	for _, tool := range tools {
		if expected[tool.Name] != tool.Scope || tool.Call == nil {
			t.Fatalf("incorrect tool contract: %s %s", tool.Name, tool.Scope)
		}
		if tool.InputSchema["type"] != "object" || tool.InputSchema["additionalProperties"] != false {
			t.Fatalf("tool schema must be a strict object: %s", tool.Name)
		}
		encoded, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var shape struct {
			Required json.RawMessage `json:"required"`
		}
		if json.Unmarshal(encoded, &shape) != nil || len(shape.Required) == 0 || string(shape.Required) == "null" {
			t.Fatalf("required must serialize as an array: %s", tool.Name)
		}
		if tool.Name == "codegen_history" || tool.Name == "excel_templates" {
			if string(shape.Required) != "[]" {
				t.Fatalf("no-argument schema requires empty array: %s", encoded)
			}
		}
		delete(expected, tool.Name)
	}
	if len(expected) != 0 {
		t.Fatal("tool names missing")
	}
	index := toolsByName(tools)
	fields := index["codegen_preview"].InputSchema["properties"].(map[string]any)["fields"].(map[string]any)
	if fields["maxItems"] != 40 || fields["minItems"] != 1 {
		t.Fatal("codegen field array constraints missing")
	}
	rows := index["excel_validate"].InputSchema["properties"].(map[string]any)["rows"].(map[string]any)
	if rows["maxItems"] != 10000 {
		t.Fatal("Excel row cap missing")
	}
}

func TestBuiltinPluginToolsForwardTrustedUIDFixedRoutesAndBody(t *testing.T) {
	type captured struct {
		uid                  int64
		plugin, method, path string
		body                 json.RawMessage
		marker               any
	}
	calls := []captured{}
	caller := func(ctx context.Context, uid int64, plugin, method, path string, body json.RawMessage) (any, error) {
		calls = append(calls, captured{uid, plugin, method, path, append(json.RawMessage(nil), body...), ctx.Value("test-marker")})
		return map[string]any{"proxied": true}, nil
	}
	tools := toolsByName(Tools(testAIService(t), caller))
	ctx := context.WithValue(context.Background(), "test-marker", "preserved-context")
	ctx = context.WithValue(ctx, "uid", int64(99))
	ctx = context.WithValue(ctx, "user", map[string]any{"uid": int64(88)})
	for _, test := range []struct{ name, plugin, method, path, args string }{
		{"codegen_preview", "devtools", http.MethodPost, "/api/admin/codegen/preview", previewArguments},
		{"codegen_history", "devtools", http.MethodGet, "/api/admin/codegen/history", `{}`},
		{"codegen_detail", "devtools", http.MethodGet, "/api/admin/codegen/detail?id=" + detailID, `{"id":"` + detailID + `"}`},
		{"excel_templates", "exceltemplate", http.MethodGet, "/api/admin/excel/templates", `{}`},
		{"excel_validate", "exceltemplate", http.MethodPost, "/api/admin/excel/validate", `{"templateId":"catalog","rows":[{"title":"示例"}]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := len(calls)
			result, err := tools[test.name].Call(ctx, 17, json.RawMessage(test.args))
			if err != nil || result == nil || len(calls) != before+1 {
				t.Fatalf("plugin tool did not call once: %v", err)
			}
			got := calls[len(calls)-1]
			if got.uid != 17 || got.plugin != test.plugin || got.method != test.method || got.path != test.path || got.marker != "preserved-context" {
				t.Fatalf("incorrect proxy contract: %+v", got)
			}
			if test.method == http.MethodPost && string(got.body) != test.args {
				t.Fatal("POST body was changed")
			}
			if test.method == http.MethodGet && len(got.body) != 0 {
				t.Fatal("GET unexpectedly carried request body")
			}
		})
	}
}

func TestBuiltinInvalidArgumentsDoNotCallPluginProxy(t *testing.T) {
	var calls atomic.Int32
	caller := func(context.Context, int64, string, string, string, json.RawMessage) (any, error) {
		calls.Add(1)
		return nil, nil
	}
	tools := toolsByName(Tools(testAIService(t), caller))
	for _, test := range []struct{ name, args string }{
		{"codegen_preview", `not-json`}, {"codegen_preview", `[]`}, {"codegen_preview", `null`}, {"codegen_preview", `{}`},
		{"codegen_preview", `{"module":"book_review"}`},
		{"codegen_preview", strings.Replace(previewArguments, `"table":"gf_book_review"`, `"table":1`, 1)},
		{"codegen_preview", strings.Replace(previewArguments, `"type":"string"`, `"type":"execute"`, 1)},
		{"codegen_preview", strings.Replace(previewArguments, `"required":true`, `"required":"true"`, 1)},
		{"codegen_preview", strings.Replace(previewArguments, `"label":"名称",`, ``, 1)},
		{"codegen_preview", strings.Replace(previewArguments, `"label":"名称"`, `"label":null`, 1)},
		{"codegen_preview", strings.TrimSuffix(previewArguments, "}") + `,"endpoint":"/arbitrary"}`},
		{"codegen_history", `{"uid":2}`}, {"codegen_history", `null`}, {"codegen_history", `[]`}, {"codegen_history", `{} {}`},
		{"codegen_detail", `{}`}, {"codegen_detail", `{"id":1}`}, {"codegen_detail", `{"id":"","uid":2}`},
		{"excel_templates", `{"name":"other-plugin"}`}, {"excel_templates", `null`}, {"excel_templates", `[]`},
		{"excel_validate", `{}`}, {"excel_validate", `{"templateId":"" ,"rows":[]}`},
		{"excel_validate", `{"templateId":"catalog"}`}, {"excel_validate", `{"templateId":"catalog","rows":null}`},
		{"excel_validate", `{"templateId":"catalog","rows":[null]}`},
		{"excel_validate", `{"templateId":"catalog","rows":{}}`},
		{"excel_validate", `{"templateId":"catalog","rows":[],"uid":2}`},
		{"ai_generate", `{"mode":"review","prompt":"hello","uid":2}`},
	} {
		t.Run(test.name+"_"+strconv.Itoa(len(test.args)), func(t *testing.T) {
			before := calls.Load()
			if _, err := tools[test.name].Call(context.Background(), 17, json.RawMessage(test.args)); err == nil {
				t.Errorf("invalid arguments accepted: %s", test.args)
			}
			if calls.Load() != before {
				t.Errorf("invalid arguments reached worker proxy: %s", test.args)
			}
		})
	}
	oversizedRows := bytes.Repeat([]byte("{},"), 10000)
	raw := json.RawMessage(`{"templateId":"catalog","rows":[` + string(oversizedRows) + `{}]}`)
	before := calls.Load()
	if _, err := tools["excel_validate"].Call(context.Background(), 17, raw); err == nil || calls.Load() != before {
		t.Fatal("oversized row set reached worker proxy")
	}
}

func TestBuiltinDisabledPluginsReturnClearErrorsWithoutReadingTheirStorage(t *testing.T) {
	original := runtimeplugin.Default
	runtimeplugin.Default = &runtimeplugin.Registry{}
	t.Cleanup(func() { runtimeplugin.Default = original })
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(working) })
	// Deliberately invalid storage proves the disabled path neither loads
	// source-generator history nor accesses Excel datasets in the host process.
	for _, name := range []string{"storage/plugins/devtools/data/codegen/users/17/history.json", "storage/plugins/exceltemplate/data/excel/templates/catalog.json"} {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(full, []byte("not valid JSON; must never be read"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	before := storageSnapshot(t, root)
	tools := toolsByName(Tools(nil, CallPlugin))
	for _, test := range []struct{ name, args string }{{"codegen_preview", previewArguments}, {"codegen_history", `{}`}, {"codegen_detail", `{"id":"` + detailID + `"}`}, {"excel_templates", `{}`}, {"excel_validate", `{"templateId":"catalog","rows":[]}`}} {
		if _, err := tools[test.name].Call(context.Background(), 17, json.RawMessage(test.args)); err == nil || !strings.Contains(err.Error(), "未安装或已卸载") {
			t.Fatalf("%s did not report disabled plugin clearly: %v", test.name, err)
		}
	}
	after := storageSnapshot(t, root)
	if len(before) != len(after) {
		t.Fatal("disabled tool changed storage")
	}
	for name, data := range before {
		if after[name] != data {
			t.Fatalf("disabled tool changed %s", name)
		}
	}
}

func storageSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	if err := filepath.WalkDir(root, func(name string, d fs.DirEntry, err error) error {
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
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestBuiltinAIUsesLocalServiceWithoutPluginCaller(t *testing.T) {
	s := testAIService(t)
	var providerCalls, pluginCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("wrong AI endpoint: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"本地主程序AI回复"}}],"model":"local"}`))
	}))
	defer server.Close()
	if err := s.SaveConfig(17, ai.Config{Endpoint: server.URL + "/v1", Model: "local", AllowPrivate: true}, false); err != nil {
		t.Fatal(err)
	}
	tools := toolsByName(Tools(s, func(context.Context, int64, string, string, string, json.RawMessage) (any, error) {
		pluginCalls.Add(1)
		return nil, errors.New("AI must not proxy a plugin")
	}))
	value, err := tools["ai_generate"].Call(context.Background(), 17, json.RawMessage(`{"mode":"review","prompt":"检查分层设计"}`))
	if err != nil || providerCalls.Load() != 1 || pluginCalls.Load() != 0 {
		t.Fatalf("AI was not executed by local service: %v", err)
	}
	result, ok := value.(map[string]any)
	if !ok || result["text"] != "本地主程序AI回复" {
		t.Fatal("AI result missing")
	}
	if _, err := tools["ai_generate"].Call(context.Background(), 18, json.RawMessage(`{"mode":"review","prompt":"read"}`)); err == nil || providerCalls.Load() != 1 {
		t.Fatal("AI configuration UID isolation failed")
	}
	for _, raw := range []string{`{"mode":"unknown","prompt":"read"}`, `{"mode":"review","prompt":""}`} {
		if _, err := tools["ai_generate"].Call(context.Background(), 17, json.RawMessage(raw)); err == nil {
			t.Fatal("invalid AI arguments accepted")
		}
	}
}

func TestBuiltinSourceDoesNotImportPluginStores(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range file.Imports {
			value, err := strconv.Unquote(item.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(value, "/internal/devtools/codegen") || strings.Contains(value, "/internal/devtools/excel") {
				t.Fatalf("host MCP imported plugin storage package: %s", value)
			}
		}
	}
}
