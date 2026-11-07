package builtinai

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/internal/runtimeplugin"
)

func TestPluginResponseCapsBodyAtEightMiB(t *testing.T) {
	w := &pluginResponse{header: http.Header{}}
	data := bytes.Repeat([]byte("x"), 8<<20)
	n, err := w.Write(data)
	if err != nil || n != len(data) || w.status != 200 || w.body.Len() != 8<<20 || w.exceeded {
		t.Fatal("valid limit-sized response rejected")
	}
	n, err = w.Write([]byte("overflow"))
	if err == nil || n != 0 || !w.exceeded || w.body.Len() != 8<<20 {
		t.Fatal("response cap failed or buffer grew after overflow")
	}
	large := &pluginResponse{header: http.Header{}}
	n, err = large.Write(bytes.Repeat([]byte("x"), (8<<20)+1))
	if err == nil || n != 0 || !large.exceeded || large.body.Len() != 0 {
		t.Fatal("single oversized response bypassed cap")
	}
}

func TestPluginResponseKeepsFirstStatusAndHeaders(t *testing.T) {
	w := &pluginResponse{header: http.Header{}}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"message":"unavailable"}`)); err != nil {
		t.Fatal(err)
	}
	if w.status != http.StatusServiceUnavailable || w.Header().Get("Content-Type") != "application/json" {
		t.Fatal("proxy status/header capture changed")
	}
}

func TestCallPluginRejectsInvalidUIDNamesAndDisabledPlugins(t *testing.T) {
	original := runtimeplugin.Default
	runtimeplugin.Default = &runtimeplugin.Registry{}
	t.Cleanup(func() { runtimeplugin.Default = original })
	for _, uid := range []int64{0, -1} {
		if _, err := CallPlugin(context.Background(), uid, "devtools", http.MethodGet, "/api/admin/codegen/history", nil); err == nil || !strings.Contains(err.Error(), "身份无效") {
			t.Fatalf("invalid trusted UID accepted: %d %v", uid, err)
		}
	}
	for _, name := range []string{"", "../outside", "arbitrary", "inventory"} {
		if _, err := CallPlugin(context.Background(), 17, name, http.MethodGet, "/api/admin/codegen/history", nil); err == nil || !strings.Contains(err.Error(), "不支持") {
			t.Fatalf("unapproved plugin name accepted: %s %v", name, err)
		}
	}
	for _, name := range []string{"devtools", "exceltemplate"} {
		if _, err := CallPlugin(context.Background(), 17, name, http.MethodGet, "/api/admin/test", nil); err == nil || !strings.Contains(err.Error(), "未安装或已卸载") {
			t.Fatalf("disabled plugin did not fail closed: %s %v", name, err)
		}
	}
}
