package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Tool struct {
	Name        string                                                     `json:"name"`
	Description string                                                     `json:"description"`
	InputSchema map[string]any                                             `json:"inputSchema"`
	Scope       string                                                     `json:"-"`
	Call        func(context.Context, int64, json.RawMessage) (any, error) `json:"-"`
}
type Authorizer func(context.Context, int64) error
type MCP struct {
	service   *Service
	authorize Authorizer
	tools     []Tool
	mu        sync.Mutex
	rates     map[string]rate
}
type rate struct {
	start time.Time
	count int
}
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func NewMCP(s *Service, authorize Authorizer, tools []Tool) *MCP {
	return &MCP{service: s, authorize: authorize, tools: tools, rates: map[string]rate{}}
}
func (m *MCP) allow(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if len(m.rates) > 1000 {
		for k, v := range m.rates {
			if now.Sub(v.start) > time.Minute {
				delete(m.rates, k)
			}
		}
	}
	v := m.rates[id]
	if now.Sub(v.start) > time.Minute {
		v = rate{start: now}
	}
	v.count++
	m.rates[id] = v
	return v.count <= 60
}
func rpcWrite(w http.ResponseWriter, id json.RawMessage, result any, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	v := map[string]any{"jsonrpc": "2.0", "id": id}
	if code != 0 {
		v["error"] = map[string]any{"code": code, "message": message}
	} else {
		v["result"] = result
	}
	_ = json.NewEncoder(w).Encode(v)
}
func decodeStrict(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return errors.New("参数无效")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("参数存在额外内容")
	}
	return nil
}

// ServeHTTP implements the stateless JSON variant of MCP Streamable HTTP 2025-11-25.
// Write operations remain in the authenticated UI; MCP tokens grant explicit scopes.
func (m *MCP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if origin := r.Header.Get("Origin"); origin != "" {
		u, e := url.Parse(origin)
		if e != nil || !strings.EqualFold(u.Host, r.Host) {
			http.Error(w, "Origin rejected", http.StatusForbidden)
			return
		}
	}
	rawToken := r.Header.Get("X-Devtools-Token")
	authorization := r.Header.Get("Authorization")
	if authorization != "" {
		parts := strings.Fields(authorization)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || (rawToken != "" && rawToken != parts[1]) {
			http.Error(w, "Invalid developer token", http.StatusUnauthorized)
			return
		}
		rawToken = parts[1]
	}
	t, e := m.service.ValidateToken(rawToken)
	if e != nil {
		http.Error(w, "Invalid developer token", http.StatusUnauthorized)
		return
	}
	if m.authorize == nil || m.authorize(r.Context(), t.UID) != nil {
		http.Error(w, "Developer access denied", http.StatusForbidden)
		return
	}
	if !m.allow(t.ID) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "Only JSON POST is supported", http.StatusMethodNotAllowed)
		return
	}
	contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if contentType != "application/json" {
		http.Error(w, "Content-Type must be application/json", 415)
		return
	}
	if p := r.Header.Get("MCP-Protocol-Version"); p != "" && p != "2025-11-25" && p != "2025-06-18" && p != "2025-03-26" {
		http.Error(w, "Unsupported MCP protocol version", 400)
		return
	}
	accept := r.Header.Get("Accept")
	if accept != "" && !strings.Contains(accept, "application/json") && !strings.Contains(accept, "*/*") {
		http.Error(w, "Accept must include application/json", 406)
		return
	}
	body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if e != nil {
		http.Error(w, "Request too large", 413)
		return
	}
	var q rpcRequest
	if json.Unmarshal(body, &q) != nil {
		rpcWrite(w, nil, nil, -32700, "Parse error")
		return
	}
	if q.JSONRPC != "2.0" || q.Method == "" {
		rpcWrite(w, q.ID, nil, -32600, "Invalid request")
		return
	}
	if len(q.ID) == 0 {
		if strings.HasPrefix(q.Method, "notifications/") {
			w.WriteHeader(202)
			return
		}
		rpcWrite(w, nil, nil, -32600, "Request id required")
		return
	}
	var id any
	if json.Unmarshal(q.ID, &id) != nil {
		rpcWrite(w, nil, nil, -32600, "Invalid request id")
		return
	}
	switch id.(type) {
	case string, float64:
	default:
		rpcWrite(w, nil, nil, -32600, "Invalid request id")
		return
	}
	switch q.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string          `json:"protocolVersion"`
			Meta            json.RawMessage `json:"_meta"`
			Capabilities    map[string]any  `json:"capabilities"`
			ClientInfo      map[string]any  `json:"clientInfo"`
		}
		if decodeStrict(q.Params, &p) != nil || p.ProtocolVersion == "" || p.Capabilities == nil || p.ClientInfo == nil || p.ClientInfo["name"] == nil || p.ClientInfo["version"] == nil {
			rpcWrite(w, q.ID, nil, -32602, "Invalid initialize parameters")
			return
		}
		name, nameOK := p.ClientInfo["name"].(string)
		clientVersion, versionOK := p.ClientInfo["version"].(string)
		if !nameOK || !versionOK || name == "" || clientVersion == "" {
			rpcWrite(w, q.ID, nil, -32602, "Invalid client information")
			return
		}
		version := p.ProtocolVersion
		if version != "2025-03-26" && version != "2025-06-18" {
			version = "2025-11-25"
		}
		rpcWrite(w, q.ID, map[string]any{"protocolVersion": version, "capabilities": map[string]any{"tools": map[string]any{}, "prompts": map[string]any{}}, "serverInfo": map[string]any{"name": "gosuxin-devtools", "version": "1.0.0"}, "instructions": "GoSuxin 开发工具。代码生成预览与 Excel 校验不写业务数据库；生成、回滚和导入确认请在后台页面操作。"}, 0, "")
	case "ping":
		rpcWrite(w, q.ID, map[string]any{}, 0, "")
	case "tools/list":
		tools := []Tool{}
		for _, v := range m.tools {
			if t.Allows(v.Scope) {
				tools = append(tools, v)
			}
		}
		rpcWrite(w, q.ID, map[string]any{"tools": tools}, 0, "")
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Meta      json.RawMessage `json:"_meta"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if decodeStrict(q.Params, &p) != nil {
			rpcWrite(w, q.ID, nil, -32602, "Invalid tool parameters")
			return
		}
		var selected *Tool
		for i := range m.tools {
			if m.tools[i].Name == p.Name {
				selected = &m.tools[i]
				break
			}
		}
		if selected == nil {
			rpcWrite(w, q.ID, nil, -32602, "Unknown tool")
			return
		}
		if !t.Allows(selected.Scope) {
			rpcWrite(w, q.ID, map[string]any{"content": []map[string]string{{"type": "text", "text": "令牌没有此工具的权限"}}, "isError": true}, 0, "")
			return
		}
		v, err := selected.Call(r.Context(), t.UID, p.Arguments)
		message := ""
		if err != nil {
			message = err.Error()
		} else {
			b, marshalError := json.Marshal(v)
			if marshalError != nil {
				err = marshalError
				message = "工具结果无法编码"
			} else {
				message = string(b)
			}
		}
		rpcWrite(w, q.ID, map[string]any{"content": []map[string]string{{"type": "text", "text": message}}, "isError": err != nil}, 0, "")
	case "prompts/list":
		list := []map[string]any{}
		for _, name := range []string{"schema", "review", "explain"} {
			list = append(list, map[string]any{"name": name, "description": Instructions[name], "arguments": []map[string]any{{"name": "input", "description": "需求或代码", "required": true}}})
		}
		rpcWrite(w, q.ID, map[string]any{"prompts": list}, 0, "")
	case "prompts/get":
		var p struct {
			Name      string            `json:"name"`
			Meta      json.RawMessage   `json:"_meta"`
			Arguments map[string]string `json:"arguments"`
		}
		if decodeStrict(q.Params, &p) != nil {
			rpcWrite(w, q.ID, nil, -32602, "Invalid prompt parameters")
			return
		}
		instruction, ok := Instructions[p.Name]
		input := p.Arguments["input"]
		if !ok || input == "" || len(input) > 32768 {
			rpcWrite(w, q.ID, nil, -32602, "Invalid prompt or input")
			return
		}
		rpcWrite(w, q.ID, map[string]any{"description": fmt.Sprintf("GoSuxin %s", p.Name), "messages": []map[string]any{{"role": "user", "content": map[string]string{"type": "text", "text": instruction + "\n\n" + input}}}}, 0, "")
	default:
		rpcWrite(w, q.ID, nil, -32601, "Method not found")
	}
}
