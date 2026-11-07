// Package pluginworker is the shared SDK for precompiled business workers.
package pluginworker

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/mysql"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/nosql/redis"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/internal/adminweb"
	"github.com/suxinwl/GoSuxin/internal/extend/middleware"
	_ "github.com/suxinwl/GoSuxin/internal/logic/admin_system"
	"github.com/suxinwl/GoSuxin/internal/plugins"
	"github.com/suxinwl/GoSuxin/utility/httpstream"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
)

func Run(name string, bind func(context.Context, *ghttp.RouterGroup)) {
	addr, secret, dir := os.Getenv("SUXIN_PLUGIN_ADDR"), os.Getenv("SUXIN_PLUGIN_SECRET"), os.Getenv("SUXIN_PLUGIN_PACKAGE")
	if addr == "" || len(secret) != 48 || dir == "" || os.Getenv("SUXIN_PLUGIN_NAME") != name {
		panic("Start this worker through the GoSuxin plugin manager")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	go func() { _, _ = io.Copy(io.Discard, os.Stdin); cancel() }()
	s := g.Server()
	config := ghttp.NewConfig()
	config.Address = addr
	config.DumpRouterMap = false
	if err := s.SetConfig(config); err != nil {
		panic(err)
	}
	s.BindHookHandler("/*path", ghttp.HookBeforeServe, func(r *ghttp.Request) {
		if !authenticateHostRequest(r.Request, secret) {
			r.Response.WriteStatus(403)
			r.ExitAll()
			return
		}
		middleware.TrustedPluginIdentity(r)
	})
	stop, err := plugins.Start(ctx, s)
	if err != nil {
		panic(err)
	}
	defer stop()
	s.BindHandler("GET:/__plugin_health", func(r *ghttp.Request) { r.Response.Write("ok") })
	max, _ := g.Cfg("upload").Get(ctx, "MaxBodySize")
	if max.Int64() > 0 {
		s.SetClientMaxBodySize(max.Int64() * 1024 * 1024)
	}
	frontend := adminweb.Handler(filepath.Join(dir, "admin"))
	shell := func(r *ghttp.Request) {
		if !middleware.TrustedPluginIdentity(r) {
			r.Response.WriteStatus(401)
			r.ExitAll()
			return
		}
		request := r.Request.Clone(r.Context())
		if parsed, err := url.ParseRequestURI(r.RequestURI); err == nil {
			request.URL = parsed
		}
		request.URL.Path = "/suxinweb" + strings.TrimPrefix(request.URL.Path, "/admin")
		request.URL.RawPath = ""
		frontend.ServeHTTP(httpstream.New(r.Response), request)
	}
	s.BindHandler("GET:/admin", shell)
	s.BindHandler("GET:/admin/*path", shell)
	s.BindHandler("HEAD:/admin/*path", shell)
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.HandlerResponse)
		group.Group("/admin", func(admin *ghttp.RouterGroup) {
			admin.Middleware(middleware.Token, middleware.Auth)
			plugins.BindAdmin(admin)
		})
		group.Group("/common", func(public *ghttp.RouterGroup) { plugins.BindPublic(public) })
		if bind != nil {
			bind(ctx, group)
		}
	})
	if err := serveUntilCancelled(ctx, s.Start, s.Shutdown); err != nil {
		panic(err)
	}
}

func serveUntilCancelled(ctx context.Context, start, shutdown func() error) error {
	if ctx.Err() != nil {
		return nil
	}
	// Shutdown must follow Start even when the host exits during initialization.
	// Shutting down an unstarted server leaves its later listener orphaned.
	if err := start(); err != nil {
		return err
	}
	<-ctx.Done()
	return shutdown()
}

func authenticateHostRequest(r *http.Request, secret string) bool {
	if len(secret) != 48 || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Suxin-Plugin-Secret")), []byte(secret)) != 1 {
		return false
	}
	// This records the authenticated host's original transport; the internal
	// hop is still loopback HTTP. Certificate details are not transferred.
	if r.Header.Get("X-Suxin-Plugin-HTTPS") == "1" {
		r.TLS = &tls.ConnectionState{}
	}
	// Preserve the connecting client's address for device controls, audit logs
	// and rate limits; a loopback hop must not combine all visitors into one IP.
	if remote := r.Header.Get("X-Suxin-Plugin-Remote-Addr"); remote != "" {
		if ip, _, err := net.SplitHostPort(remote); err == nil && net.ParseIP(ip) != nil {
			r.RemoteAddr = remote
		}
	}
	return true
}
