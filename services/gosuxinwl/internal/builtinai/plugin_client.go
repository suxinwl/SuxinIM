package builtinai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/suxinwl/GoSuxin/internal/runtimeplugin"
	"net/http"
	"strings"
	"time"
)

type PluginCaller func(context.Context, int64, string, string, string, json.RawMessage) (any, error)
type pluginResponse struct {
	header   http.Header
	status   int
	body     bytes.Buffer
	exceeded bool
}

func (w *pluginResponse) Header() http.Header { return w.header }
func (w *pluginResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *pluginResponse) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	if w.body.Len()+len(b) > 8<<20 {
		w.exceeded = true
		return 0, errors.New("插件响应超过大小限制")
	}
	return w.body.Write(b)
}

// CallPlugin uses the private worker proxy and respects plugin uninstall state.
func CallPlugin(ctx context.Context, uid int64, name, method, path string, body json.RawMessage) (any, error) {
	if uid < 1 {
		return nil, errors.New("管理员身份无效")
	}
	if name != "devtools" && name != "exceltemplate" {
		return nil, errors.New("不支持的开发工具插件")
	}
	if !runtimeplugin.Default.Enabled(name) {
		return nil, fmt.Errorf("%s 插件未安装或已卸载", name)
	}
	if !strings.HasPrefix(path, "/api/admin/") || strings.ContainsAny(path, "\\\r\n") || strings.Contains(path, "..") {
		return nil, errors.New("插件接口地址无效")
	}
	if method != http.MethodGet && method != http.MethodPost {
		return nil, errors.New("插件接口方法无效")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ctx = context.WithValue(ctx, "user", map[string]any{"uid": uid})
	request, err := http.NewRequestWithContext(ctx, method, "http://gosuxin.local/plugins/"+name+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	writer := &pluginResponse{header: http.Header{}}
	runtimeplugin.Default.ServeHTTP(writer, request)
	if writer.exceeded {
		return nil, errors.New("插件响应超过大小限制")
	}
	if writer.status != 200 {
		return nil, fmt.Errorf("插件接口暂不可用（HTTP %d）", writer.status)
	}
	var result struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal(writer.body.Bytes(), &result) != nil {
		return nil, errors.New("插件返回格式无效")
	}
	if result.Code != 0 {
		return nil, errors.New(result.Message)
	}
	var value any
	if len(result.Data) == 0 {
		return nil, nil
	}
	if err = json.Unmarshal(result.Data, &value); err != nil {
		return nil, errors.New("插件数据格式无效")
	}
	return value, nil
}
