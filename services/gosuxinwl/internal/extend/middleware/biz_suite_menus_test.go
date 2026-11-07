package middleware

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
)

func bizMenuDB(t *testing.T) (context.Context, gdb.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := gdb.New(gdb.ConfigNode{Type: "sqlite", Name: filepath.Join(t.TempDir(), "menus.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(ctx) })
	for _, q := range []string{
		`CREATE TABLE auth_rule(id INTEGER PRIMARY KEY,uid INTEGER,pid INTEGER,type INTEGER,title TEXT NOT NULL,des TEXT NOT NULL,locale TEXT NOT NULL,icon TEXT NOT NULL,routepath TEXT NOT NULL,component TEXT NOT NULL,redirect TEXT NOT NULL,routename TEXT NOT NULL,path TEXT NOT NULL,permission TEXT,status INTEGER,requiresauth INTEGER,hideinmenu INTEGER,hidechildreninmenu INTEGER,activemenu INTEGER,noaffix INTEGER,onlypage INTEGER,isext INTEGER,keepalive INTEGER)`,
		`CREATE TABLE auth_role(id INTEGER PRIMARY KEY,rules TEXT,btns TEXT,status INTEGER)`,
	} {
		if _, err := db.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	return ctx, db
}

func TestBizSuiteMenuMigrationPreservesExistingGrants(t *testing.T) {
	ctx, db := bizMenuDB(t)
	rules, roles := db.Model("auth_rule"), db.Model("auth_role")
	seed := func(id, kind int, route, path, permission string, status int) {
		t.Helper()
		_, err := rules.Clone().Data(map[string]any{"id": id, "uid": 0, "pid": 10, "type": kind, "title": "original", "des": "", "locale": "", "icon": "", "routepath": "legacy-route", "component": "systool/plugin/runtime", "redirect": "", "routename": route, "path": path, "permission": permission, "status": status, "onlypage": 0}).Insert()
		if err != nil {
			t.Fatal(err)
		}
	}
	seed(10, 1, BizSuiteHRMRoute, "/runtime-plugins/biz-suite", "runtime.plugin.biz-suite", 0)
	seed(11, 2, BizSuiteHRMRoute+"_admin", "/plugins/biz-suite/admin", "runtime.plugin.biz-suite.admin", 0)
	seed(20, 2, "hrm-read", "", "biz-suite.hrm.read", 0)
	seed(21, 2, "im-read", "", "biz-suite.im.read", 0)
	seed(22, 2, "im-manage", "", "biz-suite.im.manage", 1)
	seed(23, 2, "wecom-read", "", "biz-suite.wecom.read", 0)
	seed(25, 2, "im-write", "", "biz-suite.im.write", 0)
	seed(90, 1, "runtime_plugin_inventory", "/runtime-plugins/inventory", "runtime.plugin.inventory", 0)
	for _, q := range []string{
		`INSERT INTO auth_role VALUES(1,'10,90','11,21',0)`,
		`INSERT INTO auth_role VALUES(2,'10','21',0)`,
		`INSERT INTO auth_role VALUES(3,'10','11',0)`,
		`INSERT INTO auth_role VALUES(4,'10','11,21',1)`,
		`INSERT INTO auth_role VALUES(5,'10','11,22',0)`,
		`INSERT INTO auth_role VALUES(6,'90','11,21',0)`,
		`INSERT INTO auth_role VALUES(7,'*','',0)`,
		`INSERT INTO auth_role VALUES(8,'10','11,25',0)`,
	} {
		if _, err := db.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := roles.Clone().OrderAsc("id").All()
	if err := EnsureBizSuiteEntrypoints(ctx, rules, roles); err != nil {
		t.Fatal(err)
	}
	im, _ := rules.Clone().Where("routename", BizSuiteIMRoute).One()
	imID := im["id"].Int64()
	if imID <= 90 || im["title"].String() != "企业即时通讯" || im["routepath"].String() != "/communication" || im["onlypage"].Int() != 0 || im["pid"].Int() != 0 || im["requiresauth"].Int() != 1 {
		t.Fatalf("bad IM entry: %v", im)
	}
	primary, _ := rules.Clone().Where("id", 10).One()
	if primary["title"].String() != "智慧HRM" || primary["routepath"].String() != "/runtime-plugins/biz-suite/hrm" || primary["onlypage"].Int() != 1 || primary["requiresauth"].Int() != 1 {
		t.Fatalf("bad primary entry: %v", primary)
	}
	after, _ := roles.Clone().OrderAsc("id").All()
	for i, role := range after {
		if role["btns"].String() != before[i]["btns"].String() || role["status"].Int() != before[i]["status"].Int() {
			t.Fatal("business grants or role status changed")
		}
		want := before[i]["rules"].String()
		if id := role["id"].Int(); id == 1 || id == 2 || id == 6 {
			want += "," + strconv.FormatInt(imID, 10)
		}
		if role["rules"].String() != want {
			t.Fatalf("role %d menu widened: %q want %q", role["id"].Int(), role["rules"].String(), want)
		}
	}
	for _, id := range []int{11, 20, 21, 22, 23, 25} {
		row, err := rules.Clone().Where("id", id).One()
		if err != nil || len(row) == 0 {
			t.Fatal("existing button ID lost")
		}
		want := int64(10)
		if id == 21 || id == 22 || id == 25 {
			want = imID
		}
		if row["pid"].Int64() != want {
			t.Fatalf("button %d moved to wrong parent", id)
		}
		if id == 22 && row["status"].Int() != 1 {
			t.Fatal("disabled module rule re-enabled")
		}
	}
	other, _ := rules.Clone().Where("id", 90).One()
	if other["routepath"].String() != "legacy-route" || other["title"].String() != "original" {
		t.Fatal("other plugin menu changed")
	}
	for _, tt := range []struct {
		roleIDs    []int
		superRules bool
		allowed    bool
	}{
		{[]int{1}, false, true},
		{[]int{2}, false, false},    // IM grant without the shared runtime gate.
		{[]int{3}, false, false},    // Shared gate without IM read/manage.
		{[]int{2, 3}, false, true},  // Existing cross-role authorization remains valid.
		{[]int{3, 4}, false, false}, // A disabled role cannot supply the IM grant.
		{[]int{5}, false, false},    // A disabled permission cannot supply it either.
		{[]int{7}, false, true},
		{[]int{7}, true, true},
	} {
		userRoles, err := roles.Clone().WhereIn("id", tt.roleIDs).Where("status", 0).All()
		if err != nil {
			t.Fatal(err)
		}
		allowed, err := canSeeBizSuiteIMMenu(rules, userRoles, tt.superRules)
		if err != nil || allowed != tt.allowed {
			t.Fatalf("roles %v menu visibility = %t, %v; want %t", tt.roleIDs, allowed, err, tt.allowed)
		}
		candidates := gdb.Result{primary, im, other}
		visible := filterBizSuiteIMRows(candidates, allowed)
		want := 2
		if allowed {
			want = 3
		}
		if len(visible) != want {
			t.Fatal("IM visibility filter changed unrelated menu")
		}
		if got := filterBizSuiteIMRows(gdb.Result{primary, other}, allowed); len(got) != 2 {
			t.Fatal("visibility filter re-added manually removed IM menu")
		}
	}
	// Explicit removal and disabled nodes survive subsequent host/worker starts.
	_, _ = roles.Clone().Where("id", 1).Data(map[string]any{"rules": "10,90"}).Update()
	_, _ = rules.Clone().WhereIn("id", []int64{10, imID, 11}).Data(map[string]any{"status": 1}).Update()
	for i := 0; i < 2; i++ {
		if err := EnsureBizSuiteEntrypoints(ctx, rules, roles); err != nil {
			t.Fatal(err)
		}
	}
	role, _ := roles.Clone().Where("id", 1).One()
	if role["rules"].String() != "10,90" {
		t.Fatal("removed IM menu re-granted on startup")
	}
	for _, id := range []int64{10, imID, 11} {
		row, _ := rules.Clone().Where("id", id).One()
		if row["status"].Int() != 1 {
			t.Fatal("disabled menu/gate re-enabled")
		}
	}
	upgradedIM, _ := rules.Clone().Where("routename", BizSuiteIMRoute).One()
	if upgradedIM["id"].Int64() != imID || upgradedIM["title"].String() != "企业即时通讯" || upgradedIM["routepath"].String() != "/communication" {
		t.Fatal("IM title upgrade changed stable entry identity")
	}
	superRoles, _ := roles.Clone().Where("id", 7).All()
	if allowed, err := canSeeBizSuiteIMMenu(rules, superRoles, true); err != nil || allowed {
		t.Fatal("superRoleAuth ignored disabled runtime gate")
	}
	if allowed, err := canSeeBizSuiteIMMenu(rules, superRoles, false); err != nil || !allowed {
		t.Fatal("unrestricted super-role semantics changed")
	}
	count, _ := rules.Clone().Count()
	if count != 9 {
		t.Fatalf("duplicate menu/button records: %d", count)
	}
}

func TestBizSuiteFreshInstallDoesNotGrantRoles(t *testing.T) {
	ctx, db := bizMenuDB(t)
	rules, roles := db.Model("auth_rule"), db.Model("auth_role")
	if _, err := db.Exec(ctx, `INSERT INTO auth_role VALUES(1,'','',0)`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := EnsureBizSuiteEntrypoints(ctx, rules, roles); err != nil {
			t.Fatal(err)
		}
	}
	row, _ := roles.Clone().Where("id", 1).One()
	if row["rules"].String() != "" || row["btns"].String() != "" {
		t.Fatal("fresh install granted role access")
	}
	count, _ := rules.Clone().Count()
	if count != 3 {
		t.Fatalf("fresh menu count %d", count)
	}
}

func TestBizSuiteMenuJSONGrantsPreserveGateAndMigrationBoundaries(t *testing.T) {
	ctx, db := bizMenuDB(t)
	rules, roles := db.Model("auth_rule"), db.Model("auth_role")
	for _, seed := range []struct {
		id, kind                int
		route, path, permission string
	}{
		{10, 1, BizSuiteHRMRoute, "/runtime-plugins/biz-suite", "runtime.plugin.biz-suite"},
		{11, 2, "runtime-gate", runtimePluginRulePath("biz-suite"), "runtime.plugin.biz-suite.admin"},
		{21, 2, "im-read", "", "biz-suite.im.read"},
	} {
		_, err := rules.Clone().Data(map[string]any{"id": seed.id, "pid": 10, "type": seed.kind, "title": "original", "des": "", "locale": "", "icon": "", "routepath": "legacy", "component": "", "redirect": "", "routename": seed.route, "path": seed.path, "permission": seed.permission, "status": 0}).Insert()
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, statement := range []string{
		`INSERT INTO auth_role VALUES(1,'[10,90]','[11,21]',0)`,
		`INSERT INTO auth_role VALUES(2,'10,bad','[11,21]',0)`,
		`INSERT INTO auth_role VALUES(3,'[10]','[11,21oops]',0)`,
		`INSERT INTO auth_role VALUES(4,'[10]','["11"]',0)`,
		`INSERT INTO auth_role VALUES(5,'[10]','[21]',0)`,
		`INSERT INTO auth_role VALUES(6,'["*"]','["*"]',0)`,
	} {
		if _, err := db.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := EnsureBizSuiteEntrypoints(ctx, rules, roles); err != nil {
		t.Fatal(err)
	}
	imID, _ := rules.Clone().Where("routename", BizSuiteIMRoute).Value("id")
	row, _ := roles.Clone().Where("id", 1).One()
	if row["rules"].String() != "10,90,"+imID.String() || row["btns"].String() != "[11,21]" {
		t.Fatalf("JSON role migration lost boundary IDs or changed API grants: %v", row)
	}
	for id, want := range map[int]string{2: "10,bad", 3: "[10]", 4: "[10]", 5: "10," + imID.String(), 6: `["*"]`} {
		row, _ := roles.Clone().Where("id", id).One()
		if row["rules"].String() != want {
			t.Fatalf("unexpected migration for role %d: %s", id, row["rules"].String())
		}
	}
	for _, test := range []struct {
		ids     []int
		allowed bool
	}{{[]int{1}, true}, {[]int{3}, false}, {[]int{4}, false}, {[]int{5}, false}, {[]int{4, 5}, true}, {[]int{6}, false}} {
		rows, _ := roles.Clone().WhereIn("id", test.ids).All()
		allowed, err := canSeeBizSuiteIMMenu(rules, rows, false)
		if err != nil || allowed != test.allowed {
			t.Fatalf("JSON roles %v allowed=%t want=%t error=%v", test.ids, allowed, test.allowed, err)
		}
	}
	if len(bizSuiteRuleIDs(`[11,"*"]`)) != 0 || len(bizSuiteRuleIDs("11,21oops")) != 0 {
		t.Fatal("malformed role row retained partial grants")
	}
	// Once created, independent entry initialization must not regrant its menu.
	if _, err := roles.Clone().Where("id", 1).Data(map[string]any{"rules": "[10,90]"}).Update(); err != nil {
		t.Fatal(err)
	}
	if err := EnsureBizSuiteEntrypoints(ctx, rules, roles); err != nil {
		t.Fatal(err)
	}
	row, _ = roles.Clone().Where("id", 1).One()
	if row["rules"].String() != "[10,90]" {
		t.Fatal("restart regranted manually removed JSON menu")
	}
}
