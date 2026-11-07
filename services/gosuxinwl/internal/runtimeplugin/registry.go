package runtimeplugin

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/internal/extend/middleware"
	"github.com/suxinwl/GoSuxin/internal/plugins"
	"github.com/suxinwl/GoSuxin/utility/httpstream"
)

var pluginNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

type Registry struct {
	operations sync.Mutex
	mu         sync.RWMutex
	managers   map[string]*Manager
	ctx        context.Context
}

var Default = &Registry{managers: map[string]*Manager{}}

func PackageName(file string) string { m, _ := Inspect(file); return m.Name }

func (r *Registry) manager(name string) (*Manager, error) {
	if !pluginNamePattern.MatchString(name) {
		return nil, os.ErrInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if m := r.managers[name]; m != nil {
		return m, nil
	}
	m := &Manager{name: name}
	if r.ctx != nil {
		if err := m.start(r.ctx); err != nil {
			return nil, err
		}
	}
	r.managers[name] = m
	return m, nil
}

func (r *Registry) Statuses() []map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	statuses := make([]map[string]interface{}, 0, len(r.managers))
	for _, m := range r.managers {
		statuses = append(statuses, m.Status())
	}
	return statuses
}

func (r *Registry) Install(ctx context.Context, file string) error {
	r.operations.Lock()
	defer r.operations.Unlock()
	manifest, err := Inspect(file)
	if err != nil {
		return err
	}
	manager, err := r.manager(manifest.Name)
	if err != nil {
		return err
	}
	if plugins.Active(manifest.Name) {
		plugins.SuspendNative(manifest.Name)
	}
	if err := manager.Install(ctx, file); err != nil {
		if !r.Has(manifest.Name) {
			_ = plugins.ResumeNative(manifest.Name)
		}
		return err
	}
	if !plugins.Active(manifest.Name) {
		if err := middleware.EnsureRuntimePluginRule(ctx, manifest.Name, manifest.Title); err != nil {
			g.Log().Warningf(ctx, "runtime plugin %s menu provisioning failed: %v", manifest.Name, err)
		}
	}
	return nil
}

func (r *Registry) Start(ctx context.Context, s *ghttp.Server, auth ...ghttp.HandlerFunc) (func(), error) {
	if len(auth) == 0 {
		return nil, http.ErrNoCookie
	}
	entries, _ := os.ReadDir(filepath.Join("storage", "plugins"))
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != "uploads" && pluginNamePattern.MatchString(entry.Name()) {
			_, _ = r.manager(entry.Name())
		}
	}
	r.mu.RLock()
	managers := make([]*Manager, 0, len(r.managers))
	for _, m := range r.managers {
		managers = append(managers, m)
	}
	r.mu.RUnlock()
	for _, m := range managers {
		if err := m.start(ctx); err != nil {
			return nil, err
		}
		// The rule is deliberately provisioned without granting it to any role.
		// Super administrators can use it immediately; ordinary roles must be
		// explicitly assigned the generated button rule in the host RBAC UI.
		status := m.Status()
		if status["installed"] == true && !plugins.Active(m.name) {
			if err := middleware.EnsureRuntimePluginRule(ctx, m.name, status["title"].(string)); err != nil {
				g.Log().Warningf(ctx, "runtime plugin %s menu provisioning failed: %v", m.name, err)
			}
		}
	}
	r.mu.Lock()
	r.ctx = ctx
	r.mu.Unlock()
	plugins.RuntimeManaged = r.Has
	plugins.RuntimeEnabled = r.Enabled
	plugins.RuntimeSetInstalled = r.SetInstalled
	plugins.RuntimeServe = r.ServeNative
	handler := func(q *ghttp.Request) {
		req := q.Request.Clone(q.Context())
		if parsed, err := url.ParseRequestURI(q.RequestURI); err == nil {
			req.URL = parsed
		}
		r.ServeHTTP(httpstream.New(q.Response), req)
	}
	s.Group("/plugins/:plugin", func(group *ghttp.RouterGroup) {
		// The admin shell is protected as well as its API. Otherwise a user could
		// load the plugin HTML before the API correctly rejected the request.
		group.Group("/admin", func(admin *ghttp.RouterGroup) {
			admin.Middleware(runtimeAdminPageRedirect)
			admin.Middleware(auth...)
			admin.POST("/__host_auth/bootstrap", runtimePluginBootstrap)
			admin.ALL("/*path", handler)
		})
		group.Group("/api/admin", func(admin *ghttp.RouterGroup) {
			admin.Middleware(runtimeAdminPageRedirect)
			admin.Middleware(auth...)
			admin.ALL("/*path", handler)
		})
		group.ALL("/*path", handler)
	})
	return func() {
		r.mu.RLock()
		defer r.mu.RUnlock()
		for _, m := range r.managers {
			m.Close()
		}
	}, nil
}

func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	m := r.managers[name]
	r.mu.RUnlock()
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.dir != ""
}
func (r *Registry) Enabled(name string) bool {
	r.mu.RLock()
	m := r.managers[name]
	r.mu.RUnlock()
	return m != nil && m.Status()["installed"] == true
}
func (r *Registry) SetInstalled(name string, installed bool) error {
	r.operations.Lock()
	defer r.operations.Unlock()
	r.mu.RLock()
	m := r.managers[name]
	r.mu.RUnlock()
	if m == nil {
		return os.ErrNotExist
	}
	return m.SetInstalled(installed)
}

// These aliases preserve all existing clients and RBAC paths during migration.
func NativePath(name, path string) bool {
	var prefixes []string
	switch name {
	case "ebook":
		prefixes = []string{"/admin/album", "/common/album", "/albums", "/albums-en"}
	case "privatecode":
		prefixes = []string{"/admin/privatecode"}
	case "analysis":
		prefixes = []string{"/admin/analysis"}
	}
	for _, prefix := range prefixes {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}
func (r *Registry) ServeNative(name string, q *ghttp.Request) bool {
	if !r.Has(name) || !NativePath(name, q.URL.Path) {
		return false
	}
	r.mu.RLock()
	m := r.managers[name]
	r.mu.RUnlock()
	request := q.Request.Clone(q.Context())
	if parsed, err := url.ParseRequestURI(q.RequestURI); err == nil {
		request.URL = parsed
	}
	m.ServeHTTP(httpstream.New(q.Response), request)
	return true
}

func (r *Registry) FilterMenus(rows gdb.Result) gdb.Result {
	result := make(gdb.Result, 0, len(rows))
	hidden := map[int64]bool{}
	for _, row := range rows {
		name := strings.TrimPrefix(row["routename"].String(), "runtime_plugin_")
		managed := strings.HasPrefix(row["routename"].String(), "runtime_plugin_")
		if row["routename"].String() == middleware.BizSuiteIMRoute {
			name, managed = "biz-suite", true
		}
		if managed && !r.Enabled(name) {
			hidden[row["id"].Int64()] = true
		}
	}
	for changed := true; changed; {
		changed = false
		for _, row := range rows {
			id := row["id"].Int64()
			if hidden[row["pid"].Int64()] && !hidden[id] {
				hidden[id] = true
				changed = true
			}
		}
	}
	for _, row := range rows {
		if !hidden[row["id"].Int64()] {
			result = append(result, row)
		}
	}
	return result
}

// runtimeAdminPageRedirect is shared by the admin shell and admin API groups.
// It establishes the host authentication context before the normal host token
// and RBAC middleware runs. The implementation lives in auth.go so the same
// cookie rules apply to both groups.
func runtimeAdminPageRedirect(r *ghttp.Request) {
	runtimePluginAuth(r)
}

func (r *Registry) ServeHTTP(w http.ResponseWriter, q *http.Request) {
	parts := strings.Split(strings.Trim(q.URL.Path, "/"), "/")
	if len(parts) < 2 || parts[0] != "plugins" || !pluginNamePattern.MatchString(parts[1]) {
		http.NotFound(w, q)
		return
	}
	r.mu.RLock()
	m := r.managers[parts[1]]
	r.mu.RUnlock()
	if m == nil {
		http.NotFound(w, q)
		return
	}
	m.ServeHTTP(w, q)
}
