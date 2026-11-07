package plugins

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
)

func isolate(t *testing.T, plugins ...Plugin) {
	t.Helper()
	t.Chdir(t.TempDir())
	oldRegistered, oldState, oldContext, oldServer, oldRunning := registered, current, hostContext, hostServer, running
	registered, current, hostContext, hostServer, running = plugins, State{1, map[string]bool{}}, context.Background(), ghttp.GetServer(t.Name()), map[string]func(){}
	t.Cleanup(func() {
		stopAll()
		registered, current, hostContext, hostServer, running = oldRegistered, oldState, oldContext, oldServer, oldRunning
	})
}

func TestUninstallReinstallPersistsAndKeepsFiles(t *testing.T) {
	starts, stops := 0, 0
	isolate(t, Plugin{Name: "ebook", Start: func(context.Context, *ghttp.Server) (func(), error) {
		starts++
		return func() { stops++ }, nil
	}})
	if err := os.MkdirAll("manifest/codeinstall/ebook", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("storage", 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"manifest/codeinstall/ebook/plugin.json", "storage/retained.pdf"} {
		if err := os.WriteFile(name, []byte("retained content"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := loadState(); err != nil || !Enabled("ebook") {
		t.Fatalf("legacy installation not recognized: %v", err)
	}
	running["ebook"] = func() { stops++ }
	if err := SetInstalled("ebook", false); err != nil {
		t.Fatal(err)
	}
	if err := SetInstalled("ebook", false); err != nil {
		t.Fatal(err)
	}
	if stops != 1 || Enabled("ebook") {
		t.Fatalf("uninstall did not stop exactly once: %d", stops)
	}
	if err := loadState(); err != nil || Enabled("ebook") {
		t.Fatalf("cached source manifest overrode saved uninstall: %v", err)
	}
	for _, name := range []string{"manifest/codeinstall/ebook/plugin.json", "storage/retained.pdf"} {
		if data, err := os.ReadFile(name); err != nil || string(data) != "retained content" {
			t.Fatalf("uninstall changed %s: %v", name, err)
		}
	}
	if err := SetInstalled("EBOOK", true); err != nil {
		t.Fatal(err)
	}
	if err := SetInstalled("ebook", true); err != nil {
		t.Fatal(err)
	}
	if starts != 1 || !Enabled("ebook") {
		t.Fatalf("reinstall started workers more than once: %d", starts)
	}
	if err := loadState(); err != nil || !Enabled("ebook") {
		t.Fatalf("reinstall not persistent: %v", err)
	}
	if err := SetInstalled("../unsafe", false); err == nil {
		t.Fatal("unknown plugin accepted")
	}
}

func TestActivationFailureDoesNotPublishInstalled(t *testing.T) {
	isolate(t, Plugin{Name: "ebook", Start: func(context.Context, *ghttp.Server) (func(), error) { return nil, errors.New("schema unavailable") }})
	if err := SetInstalled("ebook", true); err == nil || Enabled("ebook") {
		t.Fatal("failed installation was published")
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatal("failed activation wrote a state file")
	}
}

func TestFailedStateWriteStopsNewWorker(t *testing.T) {
	stopped := false
	isolate(t, Plugin{Name: "ebook", Start: func(context.Context, *ghttp.Server) (func(), error) { return func() { stopped = true }, nil }})
	if err := os.MkdirAll(statePath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := SetInstalled("ebook", true); err == nil || Enabled("ebook") || !stopped {
		t.Fatal("failed persistence left an installed plugin or worker")
	}
}

func TestCorruptStateDoesNotSilentlyRestorePlugins(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(statePath)), 0700); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"{", `{"version":2,"installed":{}}`, `{"version":1,"installed":null}`} {
		if err := os.WriteFile(filepath.Join(root, statePath), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readState(root); err == nil {
			t.Fatalf("invalid state accepted: %s", data)
		}
	}
}

func TestMenuFilterKeepsOriginalPermissionsForReinstall(t *testing.T) {
	isolate(t, Plugin{Name: "ebook", MenuRoots: []string{"albumCenter"}}, Plugin{Name: "inventory", MenuRoots: []string{"inventory_admin"}})
	makeRow := func(id, pid int, name string) gdb.Record {
		return gdb.Record{"id": gvar.New(id), "pid": gvar.New(pid), "routename": gvar.New(name), "status": gvar.New(0)}
	}
	rows := gdb.Result{makeRow(13, 0, "codestore"), makeRow(200, 100, "albumManage"), makeRow(100, 0, "albumCenter"), makeRow(300, 200, "album:save"), makeRow(400, 0, "inventory_admin")}
	if got := FilterMenus(rows); len(got) != 1 || got[0]["id"].Int() != 13 {
		t.Fatalf("disabled menus leaked: %v", got)
	}
	if len(rows) != 5 || rows[2]["status"].Int() != 0 {
		t.Fatal("menu filter changed existing permissions")
	}
	current.Installed["ebook"] = true
	if got := FilterMenus(rows); len(got) != 4 {
		t.Fatalf("reinstall did not restore preserved menu tree: %v", got)
	}
	if EntryEnabled("inventory_admin") || !EntryEnabled("codestore") {
		t.Fatal("shortcut status incorrect")
	}
}
