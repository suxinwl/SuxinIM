// Package builtinai hosts AI/MCP as permanent GoSuxin system functionality.
package builtinai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/internal/devtools/access"
	"github.com/suxinwl/GoSuxin/internal/devtools/ai"
	"github.com/suxinwl/GoSuxin/utility/httpstream"
)

func Prepare(ctx context.Context) error {
	if _, err := DefaultService(); err != nil {
		return err
	}
	return prepareMenu(ctx, filepath.Join("devsource", "developer", "install", "install.lock"), ensureMenu)
}

// prepareMenu leaves the installation HTTP endpoints available before the
// installer creates its lock. Installed systems still fail on menu errors.
func prepareMenu(ctx context.Context, lockPath string, initialize func(context.Context) error) error {
	info, err := os.Stat(lockPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取安装锁失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("安装锁必须为普通文件: %s", lockPath)
	}
	return initialize(ctx)
}

// BindAdmin inherits the host's token, API signature, RBAC and audit middleware.
func BindAdmin(ctx context.Context, group *ghttp.RouterGroup) {
	service, err := DefaultService()
	if err != nil {
		panic(err)
	}
	group.Group("/developer/ai", func(admin *ghttp.RouterGroup) { admin.Middleware(access.RequireAdmin); service.Bind(admin) })
}
func BindPublic(server *ghttp.Server) {
	service, err := DefaultService()
	if err != nil {
		panic(err)
	}
	mcp := ai.NewMCP(service, access.Authorize, Tools(service, CallPlugin))
	handler := func(r *ghttp.Request) { mcp.ServeHTTP(httpstream.New(r.Response), r.Request) }
	server.Group("/", func(group *ghttp.RouterGroup) {
		group.ALL("/mcp", handler)
		group.ALL("/plugins/devtools/mcp", handler)
	})
}
