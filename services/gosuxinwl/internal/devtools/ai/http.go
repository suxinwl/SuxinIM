package ai

import (
	"encoding/json"
	"errors"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"io"
	"net/http"
)

func readJSON(r *ghttp.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 128<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return errors.New("请求 JSON 字段或格式无效")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("请求 JSON 存在额外内容")
	}
	return nil
}
func reply(r *ghttp.Request, data any, err error) {
	r.Response.Header().Set("Cache-Control", "no-store")
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	r.Response.WriteJson(gf.Success().SetData(data))
}

// Bind is mounted below the authenticated super-administrator group.
func (s *Service) Bind(group *ghttp.RouterGroup) {
	uid := func(r *ghttp.Request) int64 { return gconv.Int64(r.Context().Value("uid")) }
	group.GET("/config", func(r *ghttp.Request) { v, e := s.PublicConfig(uid(r)); reply(r, v, e) })
	group.POST("/config", func(r *ghttp.Request) {
		var v struct {
			Endpoint     string `json:"endpoint"`
			Model        string `json:"model"`
			APIKey       string `json:"apiKey"`
			AllowPrivate bool   `json:"allowPrivate"`
			ClearKey     bool   `json:"clearKey"`
		}
		if e := readJSON(r, &v); e != nil {
			reply(r, nil, e)
			return
		}
		e := s.SaveConfig(uid(r), Config{Endpoint: v.Endpoint, Model: v.Model, APIKey: v.APIKey, AllowPrivate: v.AllowPrivate}, v.ClearKey)
		reply(r, nil, e)
	})
	group.POST("/generate", func(r *ghttp.Request) {
		var v struct {
			Mode   string `json:"mode"`
			Prompt string `json:"prompt"`
		}
		if e := readJSON(r, &v); e != nil {
			reply(r, nil, e)
			return
		}
		result, e := s.Generate(r.Context(), uid(r), v.Mode, v.Prompt)
		reply(r, result, e)
	})
	group.GET("/tokens", func(r *ghttp.Request) { v, e := s.Tokens(uid(r)); reply(r, v, e) })
	group.POST("/tokens", func(r *ghttp.Request) {
		var v struct {
			Name   string   `json:"name"`
			Scopes []string `json:"scopes"`
			Days   int      `json:"days"`
		}
		if e := readJSON(r, &v); e != nil {
			reply(r, nil, e)
			return
		}
		t, raw, e := s.CreateToken(uid(r), v.Name, v.Scopes, v.Days)
		reply(r, map[string]any{"token": raw, "metadata": t}, e)
	})
	group.POST("/tokens/revoke", func(r *ghttp.Request) {
		var v struct {
			ID string `json:"id"`
		}
		if e := readJSON(r, &v); e != nil {
			reply(r, nil, e)
			return
		}
		reply(r, nil, s.Revoke(uid(r), v.ID))
	})
	group.GET("/interfaces", func(r *ghttp.Request) {
		reply(r, map[string]any{"mcpPath": "/mcp", "tokenHeader": "Authorization", "authScheme": "Bearer", "compatibleTokenHeader": "X-Devtools-Token", "protocolVersion": "2025-11-25", "stateless": true, "builtIn": true, "apiBase": "/admin/developer/ai", "tools": []string{"codegen_preview", "codegen_history", "codegen_detail", "excel_templates", "excel_validate", "ai_generate"}}, nil)
	})
}
