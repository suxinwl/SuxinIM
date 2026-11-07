package builtinai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/suxinwl/GoSuxin/internal/devtools/ai"
	"io"
	"net/url"
	"regexp"
	"strings"
)

func parse(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '{' || raw[len(raw)-1] != '}' {
		return errors.New("工具参数必须为对象")
	}
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return errors.New("工具参数无效")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("工具参数存在额外内容")
	}
	return nil
}

type previewSpec struct {
	Module string `json:"module"`
	Table  string `json:"table"`
	Title  string `json:"title"`
	Fields []struct {
		Name     string  `json:"name"`
		Label    *string `json:"label"`
		Type     string  `json:"type"`
		Required *bool   `json:"required"`
		Length   *int    `json:"length"`
	} `json:"fields"`
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]{1,47}$`)

func validateSpec(v previewSpec) error {
	if !identifier.MatchString(v.Module) || !identifier.MatchString(v.Table) || strings.TrimSpace(v.Title) == "" || len(v.Fields) < 1 || len(v.Fields) > 40 {
		return errors.New("字段设计的模块、表、名称或字段数量无效")
	}
	names := map[string]bool{"id": true, "owner_id": true}
	for _, f := range v.Fields {
		if !identifier.MatchString(f.Name) || names[f.Name] || f.Required == nil || f.Label == nil {
			return errors.New("字段名称、必填标记无效或重复")
		}
		names[f.Name] = true
		switch f.Type {
		case "string", "text", "int", "float", "bool", "datetime":
		default:
			return errors.New("字段类型无效")
		}
		if f.Length != nil && (*f.Length < 0 || *f.Length > 2000) {
			return errors.New("字段长度无效")
		}
	}
	return nil
}
func schema(properties map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}
func Tools(service *ai.Service, call PluginCaller) []ai.Tool {
	field := schema(map[string]any{"name": map[string]string{"type": "string"}, "label": map[string]string{"type": "string"}, "type": map[string]any{"type": "string", "enum": []string{"string", "text", "int", "float", "bool", "datetime"}}, "required": map[string]string{"type": "boolean"}, "length": map[string]any{"type": "integer", "minimum": 0, "maximum": 2000}}, "name", "label", "type", "required")
	spec := schema(map[string]any{"module": map[string]string{"type": "string"}, "table": map[string]string{"type": "string"}, "title": map[string]string{"type": "string"}, "fields": map[string]any{"type": "array", "items": field, "minItems": 1, "maxItems": 40}}, "module", "table", "title", "fields")
	empty := func(raw json.RawMessage) error { var v struct{}; return parse(raw, &v) }
	return []ai.Tool{
		{Name: "codegen_preview", Description: "通过已安装的代码生成插件预览源文件", Scope: "codegen:read", InputSchema: spec, Call: func(ctx context.Context, uid int64, raw json.RawMessage) (any, error) {
			var spec previewSpec
			if err := parse(raw, &spec); err != nil {
				return nil, err
			}
			if err := validateSpec(spec); err != nil {
				return nil, err
			}
			return call(ctx, uid, "devtools", "POST", "/api/admin/codegen/preview", raw)
		}},
		{Name: "codegen_history", Description: "查询代码生成插件的当前管理员历史", Scope: "codegen:read", InputSchema: schema(map[string]any{}), Call: func(ctx context.Context, uid int64, raw json.RawMessage) (any, error) {
			if e := empty(raw); e != nil {
				return nil, e
			}
			return call(ctx, uid, "devtools", "GET", "/api/admin/codegen/history", nil)
		}},
		{Name: "codegen_detail", Description: "查询代码生成版本与文件差异", Scope: "codegen:read", InputSchema: schema(map[string]any{"id": map[string]string{"type": "string"}}, "id"), Call: func(ctx context.Context, uid int64, raw json.RawMessage) (any, error) {
			var p struct {
				ID string `json:"id"`
			}
			if e := parse(raw, &p); e != nil {
				return nil, e
			}
			if p.ID == "" {
				return nil, errors.New("版本ID不能为空")
			}
			return call(ctx, uid, "devtools", "GET", "/api/admin/codegen/detail?id="+url.QueryEscape(p.ID), nil)
		}},
		{Name: "excel_templates", Description: "列出独立Excel插件中的当前管理员模板", Scope: "excel:read", InputSchema: schema(map[string]any{}), Call: func(ctx context.Context, uid int64, raw json.RawMessage) (any, error) {
			if e := empty(raw); e != nil {
				return nil, e
			}
			return call(ctx, uid, "exceltemplate", "GET", "/api/admin/excel/templates", nil)
		}},
		{Name: "excel_validate", Description: "调用独立Excel插件校验行，不写业务数据", Scope: "excel:read", InputSchema: schema(map[string]any{"templateId": map[string]string{"type": "string"}, "rows": map[string]any{"type": "array", "items": map[string]string{"type": "object"}, "maxItems": 10000}}, "templateId", "rows"), Call: func(ctx context.Context, uid int64, raw json.RawMessage) (any, error) {
			var p struct {
				TemplateID string           `json:"templateId"`
				Rows       []map[string]any `json:"rows"`
			}
			if e := parse(raw, &p); e != nil {
				return nil, e
			}
			if p.TemplateID == "" || p.Rows == nil || len(p.Rows) > 10000 {
				return nil, errors.New("模板ID或行数量无效")
			}
			for _, row := range p.Rows {
				if row == nil {
					return nil, errors.New("每行数据必须为对象")
				}
			}
			return call(ctx, uid, "exceltemplate", "POST", "/api/admin/excel/validate", raw)
		}},
		{Name: "ai_generate", Description: "调用主系统内置AI进行设计、审查或解释", Scope: "ai:generate", InputSchema: schema(map[string]any{"mode": map[string]any{"type": "string", "enum": []string{"schema", "review", "explain"}}, "prompt": map[string]any{"type": "string", "maxLength": 32768}}, "mode", "prompt"), Call: func(ctx context.Context, uid int64, raw json.RawMessage) (any, error) {
			var p struct {
				Mode   string `json:"mode"`
				Prompt string `json:"prompt"`
			}
			if e := parse(raw, &p); e != nil {
				return nil, e
			}
			return service.Generate(ctx, uid, p.Mode, p.Prompt)
		}},
	}
}
