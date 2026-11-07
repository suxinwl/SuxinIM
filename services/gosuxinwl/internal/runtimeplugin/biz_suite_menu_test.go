package runtimeplugin

import (
	"testing"

	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
)

func TestBizSuiteIndependentMenusFollowPluginLifecycle(t *testing.T) {
	rows := gdb.Result{}
	for _, entry := range []struct {
		id, pid int
		route   string
	}{{1, 0, "runtime_plugin_biz-suite"}, {2, 0, "biz_suite_im"}, {3, 2, "im_child"}, {4, 0, "unrelated"}} {
		rows = append(rows, gdb.Record{"id": gvar.New(entry.id), "pid": gvar.New(entry.pid), "routename": gvar.New(entry.route)})
	}
	manager := &Manager{name: "biz-suite", dir: t.TempDir()}
	r := &Registry{managers: map[string]*Manager{"biz-suite": manager}}
	if got := r.FilterMenus(rows); len(got) != 4 {
		t.Fatalf("enabled entries hidden: %v", got)
	}
	manager.disabled = true
	got := r.FilterMenus(rows)
	if len(got) != 1 || got[0]["id"].Int() != 4 {
		t.Fatalf("uninstalled/disabled entry remains: %v", got)
	}
	delete(r.managers, "biz-suite")
	got = r.FilterMenus(rows)
	if len(got) != 1 || got[0]["id"].Int() != 4 {
		t.Fatalf("absent plugin entry remains: %v", got)
	}
}
