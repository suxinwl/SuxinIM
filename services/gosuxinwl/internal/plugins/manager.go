package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
)

// State is separate from cached source manifests: uninstalling a compiled
// extension removes its running functions while keeping its data and package.
const statePath = "manifest/codeinstall/state.json"

type State struct {
	Version   int             `json:"version"`
	Installed map[string]bool `json:"installed"`
}

type Item struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Entry       string `json:"entry"`
	Installed   bool   `json:"installed"`
}

var operations sync.Mutex
var stateMutex sync.RWMutex
var current State
var hostContext context.Context
var hostServer *ghttp.Server
var running = map[string]func(){}

func readState(root string) (State, error) {
	state := State{Version: 1, Installed: map[string]bool{}}
	data, err := os.ReadFile(filepath.Join(root, statePath))
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err = json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("插件安装状态损坏: %w", err)
	}
	if state.Version != 1 || state.Installed == nil {
		return state, fmt.Errorf("插件安装状态格式无效")
	}
	return state, nil
}

func writeState(root string, state State) error {
	name := filepath.Join(root, statePath)
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(name), ".state-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), name)
}

func loadState() error {
	state, err := readState(".")
	if err != nil {
		return err
	}
	for _, p := range registered {
		if worker := os.Getenv("SUXIN_PLUGIN_NAME"); worker != "" {
			state.Installed[p.Name] = p.Name == worker
			continue
		}
		if _, explicit := state.Installed[p.Name]; !explicit {
			marker := "config.yml"
			if p.Name == "ebook" {
				marker = "plugin.json"
			}
			_, markerErr := os.Stat(filepath.Join("manifest", "codeinstall", p.Name, marker))
			_, sourceErr := os.Stat(filepath.Join("manifest", "codeinstall", p.Name, "plugin.json"))
			state.Installed[p.Name] = markerErr == nil || sourceErr == nil || (p.Name == "ebook" && os.Getenv("SUXIN_PLUGIN_PACKAGE") != "")
		}
	}
	stateMutex.Lock()
	current = state
	stateMutex.Unlock()
	return nil
}

func Enabled(name string) bool {
	if Managed(name) {
		return RuntimeEnabled(name)
	}
	stateMutex.RLock()
	defer stateMutex.RUnlock()
	return current.Installed[name]
}

func Catalog() []Item {
	items := make([]Item, 0, len(registered))
	for _, p := range registered {
		items = append(items, Item{p.Name, p.Title, p.Version, p.Description, p.Entry, Enabled(p.Name)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func SetInstalled(name string, installed bool) error {
	if Managed(name) {
		return RuntimeSetInstalled(name, installed)
	}
	operations.Lock()
	defer operations.Unlock()
	var plugin *Plugin
	for i := range registered {
		if strings.EqualFold(registered[i].Name, name) {
			plugin = &registered[i]
			break
		}
	}
	if plugin == nil {
		return fmt.Errorf("插件未集成到当前 GoSuxin 程序")
	}
	name = plugin.Name
	if hostContext == nil || hostServer == nil {
		return fmt.Errorf("插件服务尚未启动")
	}
	if Enabled(name) == installed {
		return nil
	}
	var stop func()
	if installed && plugin.Start != nil {
		var err error
		// Request cancellation must not terminate newly installed background work.
		stop, err = plugin.Start(hostContext, hostServer)
		if err != nil {
			return err
		}
	}
	stateMutex.RLock()
	next := State{Version: 1, Installed: make(map[string]bool, len(current.Installed))}
	for key, value := range current.Installed {
		next.Installed[key] = value
	}
	stateMutex.RUnlock()
	next.Installed[name] = installed
	if err := writeState(".", next); err != nil {
		if stop != nil {
			stop()
		}
		return fmt.Errorf("保存插件安装状态失败: %w", err)
	}
	stateMutex.Lock()
	current = next
	stateMutex.Unlock()
	if installed {
		if stop != nil {
			running[name] = stop
		}
	} else if stop = running[name]; stop != nil {
		stop()
		delete(running, name)
	}
	return nil
}

func stopAll() {
	operations.Lock()
	defer operations.Unlock()
	for name, stop := range running {
		stop()
		delete(running, name)
	}
}

func Gate(name string) ghttp.HandlerFunc {
	return func(r *ghttp.Request) {
		if !Enabled(name) {
			r.Response.WriteStatus(404, "插件未安装")
			r.ExitAll()
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/admin/") || r.Context().Value("uid") != nil {
			if serveRuntime(name, r) {
				return
			}
		}
		r.Middleware.Next()
	}
}

func GuardHandler(name string, handler ghttp.HandlerFunc) ghttp.HandlerFunc {
	return func(r *ghttp.Request) {
		if serveRuntime(name, r) {
			return
		}
		if !Enabled(name) {
			r.Response.WriteStatus(404, "插件未安装")
			r.ExitAll()
			return
		}
		handler(r)
	}
}

// Filtering preserves menu IDs, custom settings and role grants for reinstall.
func FilterMenus(rows gdb.Result) gdb.Result {
	hidden := map[int64]bool{}
	for _, p := range registered {
		if Enabled(p.Name) {
			continue
		}
		for _, root := range p.MenuRoots {
			for _, row := range rows {
				if row["routename"].String() == root {
					hidden[row["id"].Int64()] = true
				}
			}
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
	result := make(gdb.Result, 0, len(rows))
	for _, row := range rows {
		if !hidden[row["id"].Int64()] {
			result = append(result, row)
		}
	}
	return result
}

func EntryEnabled(route string) bool {
	for _, p := range registered {
		for _, root := range p.MenuRoots {
			if route == root {
				return Enabled(p.Name)
			}
		}
	}
	return true
}
