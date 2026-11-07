package adminsystem

import (
	"io"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestAILogSensitivePathNamespaceAndSlashNormalization(t *testing.T) {
	for _, requestPath := range []string{
		"/admin/developer/ai", "/admin/developer/ai/", "/admin/developer/ai/config",
		"/admin/developer/ai/tokens/create", "/admin/developer/ai/generate", "/admin/developer/ai/mcp",
		"//admin///developer//ai///config//", "/admin/developer/./ai/config", "/admin/developer/tools/../ai/generate",
	} {
		if !isAILogSensitivePath(requestPath) {
			t.Errorf("AI path was not hidden: %q", requestPath)
		}
	}
	for _, requestPath := range []string{
		"", "/", "/admin/developer", "/admin/developer/a", "/admin/developer/ai-tools",
		"/admin/developer/aiconfig", "/admin/developer/ais", "/admin/developer/AI/config",
		"/admin/developer/ai/../config", "/other/admin/developer/ai", "/plugins/devtools/api/admin/ai/config",
	} {
		if isAILogSensitivePath(requestPath) {
			t.Errorf("unrelated path was hidden: %q", requestPath)
		}
	}
}

func TestAILogBodiesDoNotReadSerializeOrChangeLiveRequest(t *testing.T) {
	const body = `{"apiKey":"synthetic-api-key","privatecode":"synthetic-private-source","prompt":"synthetic-prompt"}`
	request := httptest.NewRequest("POST", "/admin/developer/ai/generate?_t=123&prompt=synthetic-query-prompt", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "synthetic-auth")
	originalURL := request.URL.String()
	originalHeaders := request.Header.Clone()
	response := map[string]any{"token": "synthetic-raw-token", "result": "synthetic-generated-source"}
	originalResponse := map[string]any{"token": "synthetic-raw-token", "result": "synthetic-generated-source"}
	collectorCalls := 0
	reqBody, respBody := operationLogBodies(request.URL.Path, func() (any, any) {
		collectorCalls++
		t.Fatal("sensitive body collector must never execute")
		return request.Body, response
	})
	if collectorCalls != 0 || reqBody != aiLogBodyHiddenMarker || respBody != aiLogBodyHiddenMarker {
		t.Fatalf("unexpected hidden body values: request=%v response=%v collectorCalls=%d", reqBody, respBody, collectorCalls)
	}
	if strings.Contains(reqBody.(string), "synthetic-") || strings.Contains(respBody.(string), "synthetic-") {
		t.Fatal("hidden marker contains sensitive input")
	}
	if request.URL.String() != originalURL || !reflect.DeepEqual(request.Header, originalHeaders) {
		t.Fatal("redaction changed request URL or headers")
	}
	remaining, err := io.ReadAll(request.Body)
	if err != nil || string(remaining) != body {
		t.Fatal("redaction consumed or changed the original request body")
	}
	if !reflect.DeepEqual(response, originalResponse) {
		t.Fatal("redaction changed the handler response")
	}
}

func TestNonAIInterfacesKeepExistingBodyCollection(t *testing.T) {
	request := map[string]any{"title": "ordinary", "clientsecret": "[REDACTED]"}
	response := map[string]any{"status": true, "items": []string{"one"}}
	for _, requestPath := range []string{"/admin/system/storage", "/admin/developer/ai-tools/config", "/admin/developer/aiconfig"} {
		calls := 0
		reqBody, respBody := operationLogBodies(requestPath, func() (any, any) {
			calls++
			return request, response
		})
		if calls != 1 || !reflect.DeepEqual(reqBody, request) || !reflect.DeepEqual(respBody, response) {
			t.Fatalf("existing body collection changed for %s", requestPath)
		}
		if reqBody.(map[string]any)["clientsecret"] != "[REDACTED]" {
			t.Fatal("existing non-AI body redaction was changed")
		}
	}
}
