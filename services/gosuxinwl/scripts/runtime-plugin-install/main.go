// runtime-plugin-install installs a trusted local archive while the host is
// stopped. It uses the same package validator and worker health check as the UI.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/mysql"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/nosql/redis"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/internal/extend/middleware"
	"github.com/suxinwl/GoSuxin/internal/runtimeplugin"
)

func run() error {
	inspect := flag.Bool("inspect", false, "Validate and describe the archive without installing")
	flag.Parse()
	if flag.NArg() != 1 {
		return fmt.Errorf("usage: runtime-plugin-install [-inspect] <trusted-runtime.zip>")
	}
	archive, err := filepath.Abs(flag.Arg(0))
	if err != nil {
		return err
	}
	manifest, err := runtimeplugin.Inspect(archive)
	if err != nil {
		return err
	}
	if *inspect {
		fmt.Printf("Validated %s %s (%s/%s, %d files)\n", manifest.Name, manifest.Version, manifest.OS, manifest.Arch, len(manifest.Files))
		return nil
	}
	if _, err := os.Stat("devsource/developer/install/install.lock"); err != nil {
		return fmt.Errorf("请先完成当前宿主安装")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	address, err := g.Cfg().Get(ctx, "server.address")
	if err != nil {
		return err
	}
	_, port, err := net.SplitHostPort(address.String())
	if err != nil {
		return fmt.Errorf("无法验证宿主监听端口: %w", err)
	}
	connection, dialErr := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", port), time.Second)
	if dialErr == nil {
		connection.Close()
		return fmt.Errorf("请先停止当前宿主；运行中的宿主请通过插件市场安装")
	}
	registry := runtimeplugin.Default
	stop, err := registry.Start(ctx, g.Server("offline-plugin-install"), middleware.Token, middleware.Auth)
	if err != nil {
		return err
	}
	defer stop()
	if err = registry.Install(ctx, archive); err != nil {
		return err
	}
	fmt.Printf("Installed %s %s; start the GoSuxin host to enable it.\n", manifest.Name, manifest.Version)
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
