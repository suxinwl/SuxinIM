package wecom

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func TestNameFilterSettingsNormalizationAndIsolation(t *testing.T) {
	ctx := setupDB(t)
	c := &Client{}
	if err := c.LoadSettings(ctx); err != nil {
		t.Fatal(err)
	}
	in := Settings{
		IncludeUserNames: []string{" 张三 ", "张三", "王 小明"}, ExcludeUserNames: []string{" 李四 "},
		IncludeDepartmentNames: []string{" 分子公司 ", "分子公司"}, ExcludeDepartmentNames: []string{" 新入职员工 "},
		IncludeUsers: []string{"legacy"}, IncludeDepartments: []int64{12}, IncludeSubdepartments: true,
	}
	if err := c.SaveSettings(ctx, in); err != nil {
		t.Fatal(err)
	}
	if in.IncludeUserNames[0] != " 张三 " || in.IncludeDepartmentNames[0] != " 分子公司 " {
		t.Fatal("validation mutated caller slices")
	}
	want := c.Settings()
	if !reflect.DeepEqual(want.IncludeUserNames, []string{"张三", "王 小明"}) || !reflect.DeepEqual(want.IncludeDepartmentNames, []string{"分子公司"}) || want.ExcludeUserNames[0] != "李四" || want.ExcludeDepartmentNames[0] != "新入职员工" {
		t.Fatal("name normalization lost exact Chinese names or internal whitespace", want)
	}
	copy := c.Settings()
	copy.IncludeUserNames[0], copy.ExcludeUserNames[0] = "changed", "changed"
	copy.IncludeDepartmentNames[0], copy.ExcludeDepartmentNames[0] = "changed", "changed"
	in.IncludeUserNames[0], in.IncludeDepartmentNames[0] = "changed", "changed"
	if !reflect.DeepEqual(c.Settings(), want) {
		t.Fatal("caller mutated active filter without a save")
	}
	reloaded := &Client{}
	if err := reloaded.LoadSettings(ctx); err != nil || !reflect.DeepEqual(reloaded.Settings(), want) {
		t.Fatal("name filters did not survive reload", err)
	}
	for name, bad := range map[string]Settings{
		"blank person":         {IncludeUserNames: []string{"  "}},
		"blank department":     {ExcludeDepartmentNames: []string{" "}},
		"person too long":      {ExcludeUserNames: []string{strings.Repeat("张", 129)}},
		"department too long":  {IncludeDepartmentNames: []string{strings.Repeat("部", 161)}},
		"internal control":     {IncludeUserNames: []string{"张\n三"}},
		"invalid UTF8":         {ExcludeUserNames: []string{string([]byte{255})}},
		"too many people":      {IncludeUserNames: make([]string, 10001)},
		"too many departments": {ExcludeDepartmentNames: make([]string, 10001)},
	} {
		t.Run(name, func(t *testing.T) {
			if err := c.SaveSettings(ctx, bad); err == nil {
				t.Fatal("invalid name list accepted")
			}
			if !reflect.DeepEqual(c.Settings(), want) {
				t.Fatal("invalid save changed active filters")
			}
		})
	}
	if err := (&Settings{IncludeUserNames: []string{strings.Repeat("张", 128)}, IncludeDepartmentNames: []string{strings.Repeat("部", 160)}}).validate(); err != nil {
		t.Fatal("valid Unicode names were limited by bytes", err)
	}
}

func TestNameFilterExactUnionExclusionAndDepartmentSubtrees(t *testing.T) {
	f := syncFilter{
		settings:        Settings{IncludeDepartmentNames: []string{"分子公司"}, ExcludeDepartmentNames: []string{"新入职员工"}, IncludeUserNames: []string{"张三"}, IncludeUsers: []string{"legacy", "excluded-name"}, ExcludeUsers: []string{"excluded-id"}, ExcludeUserNames: []string{"李四"}, IncludeSubdepartments: true},
		parents:         map[int64]int64{1: 0, 2: 1, 3: 2, 4: 2, 5: 4, 6: 1, 7: 6, 8: 1},
		departmentNames: map[int64]string{1: "总公司", 2: "分子公司", 3: "研发", 4: "新入职员工", 5: "试用团队", 6: "分子公司", 7: "研发二部", 8: "其他"},
		userNames:       map[string]string{"first": "张三", "second": "张三", "prefix": "张三丰", "legacy": "王五", "excluded-name": "李四", "excluded-id": "张三"},
	}
	for _, tc := range []struct {
		name, uid string
		deps      []int64
		want      bool
	}{
		{"same name first", "first", []int64{3}, true},
		{"same name second and duplicate department name", "second", []int64{7}, true},
		{"exact name only", "prefix", []int64{3}, false},
		{"legacy ID union", "legacy", []int64{3}, true},
		{"name exclusion beats ID inclusion", "excluded-name", []int64{3}, false},
		{"ID exclusion beats name inclusion", "excluded-id", []int64{3}, false},
		{"unknown name", "unknown", []int64{3}, false},
		{"person and department intersection", "first", []int64{8}, false},
		{"excluded child", "first", []int64{5}, false},
		{"multi department exclusion wins", "first", []int64{3, 5}, false},
		{"unknown department membership", "first", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := f.allows(tc.uid, tc.deps); got != tc.want {
				t.Fatalf("allowed=%v want=%v", got, tc.want)
			}
		})
	}
	f.settings.IncludeSubdepartments = false
	if f.allows("first", []int64{3}) || !f.allows("first", []int64{2}) || !f.departmentAllowed(6) {
		t.Fatal("subdepartment switch or duplicate-name matching ignored")
	}
	f.settings.IncludeSubdepartments = true
	f.settings.IncludeDepartments = []int64{8}
	if !f.allows("first", []int64{8}) {
		t.Fatal("department ID/name union lost")
	}
	f.settings.IncludeDepartments = nil
	f.settings.IncludeDepartmentNames = []string{"尚未同步的部门"}
	if f.allows("first", []int64{3}) || f.departmentAllowed(2) {
		t.Fatal("unresolved department name incorrectly disabled the allowlist")
	}
	f.settings.IncludeDepartmentNames = []string{"分子公司"}
	f.departmentNames[2] = "已更名"
	if f.allows("first", []int64{3}) {
		t.Fatal("old resolved IDs survived a department rename")
	}
	f.parents[9], f.departmentNames[9] = 6, "新增分部"
	if !f.allows("first", []int64{9}) {
		t.Fatal("new descendant did not inherit named department selection")
	}
	f.settings.IncludeUserNames = []string{"unknown"}
	if f.allowsNamed("unknown", "unknown", []int64{9}) {
		t.Fatal("userid placeholder was accepted as a real name")
	}
}

func TestNameFilterFirstContactSyncAndCheckin(t *testing.T) {
	ctx := setupDB(t)
	var memberDepartments, requestedUsers []string
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/department/list":
			respond(w, g.Map{"errcode": 0, "department": []Department{{ID: 1, Name: "总公司"}, {ID: 2, ParentID: 1, Name: "分子公司"}, {ID: 3, ParentID: 2, Name: "研发"}, {ID: 4, ParentID: 2, Name: "新入职员工"}, {ID: 5, ParentID: 4, Name: "培训组"}, {ID: 6, ParentID: 1, Name: "分子公司"}, {ID: 7, ParentID: 1, Name: "其他"}}})
		case "/user/list":
			memberDepartments = append(memberDepartments, r.URL.Query().Get("department_id"))
			respond(w, g.Map{"errcode": 0, "userlist": []User{
				{UserID: "alice", Name: "张三", Department: []int64{3}, Status: 1},
				{UserID: "bob", Name: "张三", Department: []int64{6}, Status: 1},
				{UserID: "legacy", Name: "王五", Department: []int64{2}, Status: 1},
				{UserID: "prefix", Name: "张三丰", Department: []int64{2}, Status: 1},
				{UserID: "excluded", Name: "李四", Department: []int64{2}, Status: 1},
				{UserID: "newhire", Name: "张三", Department: []int64{3, 5}, Status: 1},
				{UserID: "outside", Name: "张三", Department: []int64{7}, Status: 1},
				{UserID: "unknown", Department: []int64{3}, Status: 1},
			}})
		case "/checkin/getcheckindata":
			var in struct {
				Users []string `json:"useridlist"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Error(err)
			}
			requestedUsers = append(requestedUsers, in.Users...)
			clocks := []Checkin{}
			for _, uid := range in.Users {
				clocks = append(clocks, Checkin{UserID: uid, Type: "上班打卡", Time: 1700000000})
			}
			respond(w, g.Map{"errcode": 0, "checkindata": clocks})
		default:
			t.Error(r.URL.Path)
		}
	})
	if err := c.LoadSettings(ctx); err != nil {
		t.Fatal(err)
	}
	s := Settings{IncludeDepartmentNames: []string{"分子公司"}, ExcludeDepartmentNames: []string{"新入职员工"}, IncludeUserNames: []string{"张三"}, IncludeUsers: []string{"legacy", "excluded"}, ExcludeUserNames: []string{"李四"}, IncludeSubdepartments: true}
	if err := c.SaveSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if n, err := c.SyncContact(ctx); err != nil || n != 3 {
			t.Fatal("first-sync names or idempotence", n, err)
		}
	}
	if strings.Join(memberDepartments, ",") != "2,3,6,2,3,6" || countRows(t, ctx, "wecom_user") != 3 || countRows(t, ctx, "biz_hrm_employee") != 3 {
		t.Fatal("fresh department tree or upstream member names not applied", memberDepartments)
	}
	// Existing blank raw profile can use one explicitly bound employee's real name.
	for _, uid := range []string{"fallback", "unknown"} {
		seedRawUser(t, ctx, uid)
		seedEmployee(t, ctx, uid)
		if _, err := core.Model("wecom_user").Ctx(ctx).Where("wecom_userid", uid).Update(g.Map{"department_ids": "[3]"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := core.Model("biz_hrm_employee").Ctx(ctx).Where("wecom_userid", "fallback").Update(g.Map{"name": "张三"}); err != nil {
		t.Fatal(err)
	}
	start, end := time.Unix(1699990000, 0), time.Unix(1700010000, 0)
	for i := 0; i < 2; i++ {
		if n, err := c.SyncCheckin(ctx, start, end); err != nil || n != 4 {
			t.Fatal("checkin name filter", n, err)
		}
	}
	if strings.Join(requestedUsers, ",") != "alice,bob,fallback,legacy,alice,bob,fallback,legacy" || countRows(t, ctx, "biz_hrm_attendance_clock") != 4 {
		t.Fatal("unknown names leaked into checkin or duplicate clocks created", requestedUsers)
	}
}

func TestNameFilterApprovalUnknownNamesAndHistoricalCancellation(t *testing.T) {
	ctx := setupDB(t)
	for _, uid := range []string{"alice", "unknown"} {
		seedEmployee(t, ctx, uid)
	}
	if _, err := core.Model("biz_hrm_employee").Ctx(ctx).Where("wecom_userid", "alice").Update(g.Map{"name": "张三"}); err != nil {
		t.Fatal(err)
	}
	installTemplate(t, ctx)
	phase := 0
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oa/getapprovalinfo":
			ids := []string{"known", "unknown"}
			if phase > 0 {
				ids = []string{}
			}
			respond(w, g.Map{"errcode": 0, "sp_no_list": ids})
		case "/oa/getapprovaldetail":
			var in map[string]string
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Error(err)
			}
			detail := approvalFixture(in["sp_no"], "leave-tpl", 2)
			if in["sp_no"] == "unknown" {
				detail["applyer"] = g.Map{"userid": "unknown"}
			}
			if phase == 2 {
				detail = g.Map{"sp_no": in["sp_no"], "template_id": "leave-tpl", "sp_status": 7}
			}
			respond(w, g.Map{"errcode": 0, "info": detail})
		default:
			t.Error(r.URL.Path)
		}
	})
	if err := c.LoadSettings(ctx); err != nil {
		t.Fatal(err)
	}
	s := Settings{IncludeUserNames: []string{"张三", "unknown"}}
	if err := c.SaveSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	start, end := time.Unix(1699990000, 0), time.Unix(1700010000, 0)
	if n, err := c.SyncApprovals(ctx, start, end); err != nil || n != 1 {
		t.Fatal("HRM mapping fallback or unknown-name filtering", n, err)
	}
	if countRows(t, ctx, "wecom_approval_raw") != 1 || countRows(t, ctx, "biz_hrm_attendance_leave") != 1 {
		t.Fatal("unnamed applicant imported")
	}
	s.ExcludeUserNames = []string{"张三"}
	if err := c.SaveSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	phase = 1
	if n, err := c.SyncApprovals(ctx, start, end); err != nil || n != 0 {
		t.Fatal("excluded approved history re-imported", n, err)
	}
	phase = 2
	if n, err := c.SyncApprovals(ctx, start, end); err != nil || n != 1 {
		t.Fatal("name filter blocked historical cancellation", n, err)
	}
	row, err := core.Model("biz_hrm_attendance_leave").Ctx(ctx).One()
	if err != nil || row["approval_status"].String() != "canceled" {
		t.Fatal("old leave remained effective", err)
	}
}

func TestNameFilterOptionsSafeNamesAndBoundEmployees(t *testing.T) {
	ctx := setupDB(t)
	for _, d := range []Department{{ID: 2, Name: "分子公司"}, {ID: 3, ParentID: 2, Name: "研发"}} {
		if err := upsert(ctx, "wecom_department", "wecom_id", d.ID, g.Map{"id": stableID(fmt.Sprint(d.ID)), "wecom_id": d.ID, "parent_id": d.ParentID, "name": d.Name, "raw_json": "{}", "synced_at": now()}); err != nil {
			t.Fatal(err)
		}
	}
	for _, uid := range []string{"alice", "same-name", "unknown"} {
		seedRawUser(t, ctx, uid)
		name := "张三"
		if uid == "unknown" {
			name = uid
		}
		if _, err := core.Model("wecom_user").Ctx(ctx).Where("wecom_userid", uid).Update(g.Map{"name": name, "department_ids": "[2,3]", "mobile": "PRIVATE-PHONE", "email": "PRIVATE-EMAIL"}); err != nil {
			t.Fatal(err)
		}
	}
	seedEmployee(t, ctx, "bound")
	if _, err := core.Model("biz_hrm_employee").Ctx(ctx).Where("wecom_userid", "bound").Update(g.Map{"name": "王小明", "mobile": "PRIVATE-HRM-PHONE", "email": "PRIVATE-HRM-EMAIL"}); err != nil {
		t.Fatal(err)
	}
	c := &Client{}
	s := g.Server(guid.S())
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) {
			r.SetCtx(context.WithValue(r.Context(), "uid", int64(1)))
			r.Middleware.Next()
		})
		Bind(ctx, group, c)
	})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	res, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/admin/wecom/filter-options", s.GetListenedPort()))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatal("filter options failed", err)
	}
	if strings.Contains(string(raw), "PRIVATE-") || strings.Contains(string(raw), "mobile") || strings.Contains(string(raw), "email") {
		t.Fatal("filter suggestions disclosed sensitive fields")
	}
	var response struct {
		Code int `json:"code"`
		Data struct {
			Users []filterUserOption `json:"users"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil || response.Code != 0 || len(response.Data.Users) != 4 {
		t.Fatal("filter options contract", err)
	}
	for _, u := range response.Data.Users {
		switch u.UserID {
		case "alice", "same-name":
			if u.Name != "张三" || !slices.Equal(u.DepartmentNames, []string{"分子公司", "研发"}) || u.DepartmentIDs != "[2,3]" {
				t.Fatal("same-name department labels lost", u)
			}
		case "bound":
			if u.Name != "王小明" || u.DepartmentIDs != "[]" || len(u.DepartmentNames) != 0 {
				t.Fatal("HRM bound member merge inferred department membership", u)
			}
		case "unknown":
			if u.Name != "" {
				t.Fatal("userid displayed as a real name")
			}
		default:
			t.Fatal("unexpected suggestion")
		}
	}
}

func TestNameFilterRefreshesExcludedExistingMemberScope(t *testing.T) {
	ctx := setupDB(t)
	phase, checkinCalls := 0, 0
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/department/list":
			respond(w, g.Map{"errcode": 0, "department": []Department{{ID: 2, Name: "分子公司"}, {ID: 3, ParentID: 2, Name: "研发"}, {ID: 4, ParentID: 2, Name: "新入职员工"}}})
		case "/user/list":
			name, departments := "张三", []int64{2}
			if phase == 1 {
				name = "李四"
			}
			if phase == 2 {
				departments = []int64{3, 4}
			}
			respond(w, g.Map{"errcode": 0, "userlist": []User{{UserID: "alice", Name: name, Department: departments, Status: 1}, {UserID: "never-import", Name: "李四", Department: []int64{2}, Status: 1}}})
		case "/checkin/getcheckindata":
			checkinCalls++
			respond(w, g.Map{"errcode": 0, "checkindata": []any{}})
		default:
			t.Error(r.URL.Path)
		}
	})
	if err := c.LoadSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveSettings(ctx, Settings{IncludeUserNames: []string{"张三"}, ExcludeUserNames: []string{"李四"}, IncludeDepartmentNames: []string{"分子公司"}, ExcludeDepartmentNames: []string{"新入职员工"}, IncludeSubdepartments: true}); err != nil {
		t.Fatal(err)
	}
	if n, err := c.SyncContact(ctx); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	for phase = 1; phase <= 2; phase++ {
		if n, err := c.SyncContact(ctx); err != nil || n != 0 {
			t.Fatal("excluded member recreated", n, err)
		}
		f, err := c.filter(ctx)
		if err != nil {
			t.Fatal(err)
		}
		allowed, err := f.applicantAllowed(ctx, "alice")
		if err != nil || allowed {
			t.Fatal("stale scope metadata allowed approval after rename/move", err)
		}
		if n, err := c.SyncCheckin(ctx, time.Unix(1699990000, 0), time.Unix(1700010000, 0)); err != nil || n != 0 {
			t.Fatal("excluded checkin", n, err)
		}
	}
	if checkinCalls != 0 || countRows(t, ctx, "wecom_user") != 1 || countRows(t, ctx, "biz_hrm_employee") != 1 {
		t.Fatal("excluded person inserted or checkin queried")
	}
	row, err := core.Model("biz_hrm_employee").Ctx(ctx).One()
	if err != nil || row["name"].String() != "张三" {
		t.Fatal("filter rewrote existing HRM profile/history", err)
	}
}

func TestNameFilterUnknownValuesCannotBypassNameExclusions(t *testing.T) {
	f := syncFilter{settings: Settings{ExcludeUserNames: []string{"李四"}, IncludeUsers: []string{"unknown"}}}
	for _, name := range []string{"", "unknown"} {
		if f.allowsNamed("unknown", name, nil) {
			t.Fatal("unknown name bypassed name exclusion using ID allowlist")
		}
	}
	f.settings.IncludeUsers = nil
	if !f.allowsNamed("known", "张三", nil) {
		t.Fatal("known non-excluded name rejected")
	}
	f.settings = Settings{ExcludeDepartmentNames: []string{"新入职员工"}, IncludeSubdepartments: true}
	f.parents = map[int64]int64{1: 0, 2: 1, 4: 1, 5: 6, 6: 5}
	f.departmentNames = map[int64]string{1: "总公司", 2: "研发", 3: "缺失上级", 4: "", 5: "循环一", 6: "循环二"}
	if !f.allowsNamed("known", "张三", []int64{2}) {
		t.Fatal("complete known non-excluded path rejected")
	}
	for _, id := range []int64{3, 4, 5, 99} {
		if f.allowsNamed("known", "张三", []int64{id}) {
			t.Fatalf("unknown/cyclic department path %d bypassed name exclusion", id)
		}
	}
	f.settings.IncludeSubdepartments = false
	if !f.allowsNamed("known", "张三", []int64{3}) || f.allowsNamed("known", "张三", []int64{4}) {
		t.Fatal("exact department exclusion required ancestors or accepted missing name")
	}
	f.settings = Settings{ExcludeDepartments: []int64{9}, IncludeSubdepartments: true}
	if !f.allowsNamed("known", "", []int64{99}) {
		t.Fatal("legacy ID-only filter behavior changed")
	}
}

func TestNameFilterRefreshesMemberMovedWhollyIntoExcludedDepartment(t *testing.T) {
	ctx := setupDB(t)
	phase, checkinCalls := 0, 0
	var requestedDepartments []string
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/department/list":
			respond(w, g.Map{"errcode": 0, "department": []Department{{ID: 2, Name: "分子公司"}, {ID: 4, ParentID: 2, Name: "新入职员工"}}})
		case "/user/list":
			department := r.URL.Query().Get("department_id")
			requestedDepartments = append(requestedDepartments, department)
			users := []User{}
			if phase == 0 && department == "2" {
				users = append(users, User{UserID: "alice", Name: "张三", Department: []int64{2}, Status: 1})
			}
			if phase == 1 && department == "4" {
				users = append(users, User{UserID: "alice", Name: "张三", Department: []int64{4}, Status: 1}, User{UserID: "never-import", Name: "王五", Department: []int64{4}, Status: 1})
			}
			respond(w, g.Map{"errcode": 0, "userlist": users})
		case "/checkin/getcheckindata":
			checkinCalls++
			respond(w, g.Map{"errcode": 0, "checkindata": []any{}})
		default:
			t.Error(r.URL.Path)
		}
	})
	if err := c.LoadSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.SaveSettings(ctx, Settings{IncludeDepartmentNames: []string{"分子公司"}, ExcludeDepartmentNames: []string{"新入职员工"}, IncludeSubdepartments: true}); err != nil {
		t.Fatal(err)
	}
	if n, err := c.SyncContact(ctx); err != nil || n != 1 {
		t.Fatal("initial member import", n, err)
	}
	phase = 1
	if n, err := c.SyncContact(ctx); err != nil || n != 0 {
		t.Fatal("excluded department import", n, err)
	}
	f, err := c.filter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if allowed, err := f.applicantAllowed(ctx, "alice"); err != nil || allowed {
		t.Fatal("member moved wholly into an excluded department retained stale eligibility", err)
	}
	if n, err := c.SyncCheckin(ctx, time.Unix(1699990000, 0), time.Unix(1700010000, 0)); err != nil || n != 0 {
		t.Fatal("excluded member checkin", n, err)
	}
	if checkinCalls != 0 || countRows(t, ctx, "wecom_user") != 1 || countRows(t, ctx, "biz_hrm_employee") != 1 {
		t.Fatal("scope refresh imported excluded members or queried their checkins")
	}
	if strings.Join(requestedDepartments, ",") != "2,2,4" {
		t.Fatal("excluded departments should only be checked for missing known members", requestedDepartments)
	}
}

func TestNameFilterUnobservedMemberIsUnknownNotResigned(t *testing.T) {
	ctx := setupDB(t)
	visible := true
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/department/list":
			respond(w, g.Map{"errcode": 0, "department": []Department{{ID: 2, Name: "分子公司"}}})
		case "/user/list":
			users := []User{}
			if visible {
				users = append(users, User{UserID: "alice", Name: "张三", Department: []int64{2}, Status: 1})
			}
			respond(w, g.Map{"errcode": 0, "userlist": users})
		case "/checkin/getcheckindata":
			t.Error("unobserved member must not be queried for checkins")
			respond(w, g.Map{"errcode": 0, "checkindata": []any{}})
		default:
			t.Error(r.URL.Path)
		}
	})
	if err := c.LoadSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if n, err := c.SyncContact(ctx); err != nil || n != 1 {
		t.Fatal("initial import", n, err)
	}
	visible = false
	if n, err := c.SyncContact(ctx); err != nil || n != 0 {
		t.Fatal("reduced visibility", n, err)
	}
	row, err := core.Model("wecom_user").Ctx(ctx).Where("wecom_userid", "alice").One()
	if err != nil || row["name"].String() != "张三" || row["department_ids"].String() != "[2]" || row["processing_error"].String() != contactScopeUnobserved {
		t.Fatal("unobserved scope must preserve the last profile and report uncertainty", err)
	}
	employee, err := core.Model("biz_hrm_employee").Ctx(ctx).Where("wecom_userid", "alice").One()
	if err != nil || employee["status"].String() != "active" || employee["name"].String() != "张三" {
		t.Fatal("missing visibility altered HRM profile or status", err)
	}
	f, err := c.filter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if f.allows("alice", []int64{2}) {
		t.Fatal("no configured filters must not bypass an unknown scope")
	}
	if allowed, err := f.applicantAllowed(ctx, "alice"); err != nil || allowed {
		t.Fatal("unknown scope remained approval eligible", err)
	}
	if n, err := c.SyncCheckin(ctx, time.Unix(1699990000, 0), time.Unix(1700010000, 0)); err != nil || n != 0 {
		t.Fatal("unknown scope checkin", n, err)
	}
	visible = true
	if n, err := c.SyncContact(ctx); err != nil || n != 1 {
		t.Fatal("observed member did not recover", n, err)
	}
	f, err = c.filter(ctx)
	if err != nil || !f.allows("alice", []int64{2}) {
		t.Fatal("confirmed member did not regain eligibility", err)
	}
	row, err = core.Model("wecom_user").Ctx(ctx).Where("wecom_userid", "alice").One()
	if err != nil || row["processing_error"].String() != "" || countRows(t, ctx, "biz_hrm_employee") != 1 {
		t.Fatal("confirmation left an exception or duplicated HRM employee", err)
	}
}
