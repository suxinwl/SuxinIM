package hrm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func TestClockFilterDatesAndValidation(t *testing.T) {
	for _, status := range []string{"", "normal", "late", "early", "missing", "abnormal"} {
		f, err := parseClockFilter(map[string]string{"status": " " + status + " "})
		if err != nil || f.Status != status {
			t.Fatalf("status %q: %+v %v", status, f, err)
		}
	}
	valid := []struct {
		query      map[string]string
		start, end string
		exclusive  bool
	}{
		{map[string]string{"date": "2026-12-31"}, "2026-12-31 00:00:00", "2027-01-01 00:00:00", true},
		{map[string]string{"month": "2026-12"}, "2026-12-01 00:00:00", "2027-01-01 00:00:00", true},
		{map[string]string{"month": "2024-02"}, "2024-02-01 00:00:00", "2024-03-01 00:00:00", true},
		{map[string]string{"start_time": "2026-12-31", "end_time": "2027-01-01"}, "2026-12-31 00:00:00", "2027-01-02 00:00:00", true},
		{map[string]string{"start_time": "2026-12-31T16:00:00Z", "end_time": "2027-01-01T17:30"}, "2027-01-01 00:00:00", "2027-01-01 17:30:00", false},
		{map[string]string{"end_time": "2026-12-31"}, "", "2027-01-01 00:00:00", true},
		{map[string]string{"start_time": "2026-12-31"}, "2026-12-31 00:00:00", "", false},
	}
	for _, tt := range valid {
		f, err := parseClockFilter(tt.query)
		if err != nil {
			t.Fatalf("%v: %v", tt.query, err)
		}
		start, end := "", ""
		if !f.Start.IsZero() {
			start = f.Start.Format("2006-01-02 15:04:05")
		}
		if !f.End.IsZero() {
			end = f.End.Format("2006-01-02 15:04:05")
		}
		if start != tt.start || end != tt.end || f.EndExclusive != tt.exclusive || !f.IncludeSubdepartments {
			t.Fatalf("wrong boundaries for %v: %+v", tt.query, f)
		}
	}
	invalid := []map[string]string{
		{"date": "2026-02-30"}, {"date": "2026-1-01"}, {"month": "2026-13"}, {"month": "2026-1"},
		{"date": "2026-01-01", "month": "2026-01"}, {"date": "2026-01-01", "end_time": "2026-01-02"},
		{"month": "2026-01", "start_time": "2026-01-01"}, {"start_time": "bad"}, {"end_time": "bad"},
		{"start_time": "2026-01-02", "end_time": "2026-01-01"},
		{"start_time": "2026-01-01 12:00:01", "end_time": "2026-01-01 12:00:00"},
		{"date": "0999-01-01"}, {"month": "9999-12"}, {"date": "9999-12-31"},
		{"department": "10"}, {"department": "wecom:-1"}, {"department": "host:0"}, {"department": "wecom:1 OR 1=1"},
		{"department": "wecom:01"}, {"department": "wecom:9223372036854775808"}, {"include_subdepartments": "sometimes"},
		{"page": "0"}, {"page": "-1"}, {"page": "1.5"}, {"page_size": "501"},
		{"status": "unknown"}, {"status": "迟到"}, {"status": "late,missing"}, {"status": "late' OR 1=1 --"},
	}
	for _, query := range invalid {
		if _, err := parseClockFilter(query); err == nil {
			t.Fatalf("invalid filter accepted: %v", query)
		}
	}
}

func TestClockStatusFiltersRespectPaginationAndScope(t *testing.T) {
	ctx := clockFixtures(t)
	for id, status := range map[string]string{
		"day-start": "late", "day-end-z": "late", "day-end-a": "early",
		"after": "late", "same-name-clock": "late", "local-clock": "missing", "fallback-clock": "abnormal",
	} {
		if _, err := core.Model("biz_hrm_attendance_clock").Ctx(ctx).Where("id", id).Data(g.Map{"status": status}).Update(); err != nil {
			t.Fatal(err)
		}
	}
	check := func(ctx context.Context, query map[string]string, wantIDs []string, wantTotal int) {
		t.Helper()
		directory, err := loadClockDirectory(ctx)
		if err != nil {
			t.Fatal(err)
		}
		filter, err := parseClockFilter(query)
		if err != nil {
			t.Fatal(err)
		}
		rows, total, err := queryClocks(ctx, filter, directory)
		if err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for _, row := range rows {
			ids = append(ids, row["id"].(string))
			if filter.Status != "" && row["status"] != filter.Status {
				t.Fatalf("status filter returned another status: %v", row)
			}
		}
		if total != wantTotal || !reflect.DeepEqual(ids, wantIDs) {
			t.Fatalf("%v: got %v total %d, want %v total %d", query, ids, total, wantIDs, wantTotal)
		}
	}
	for _, tt := range []struct {
		query map[string]string
		ids   []string
		total int
	}{
		{map[string]string{"status": "", "date": "2026-12-31", "employee_id": "first"}, []string{"day-end-z", "day-end-a", "day-start"}, 3},
		{map[string]string{"status": "late", "date": "2026-12-31", "page": "2", "page_size": "1"}, []string{"same-name-clock"}, 3},
		{map[string]string{"status": "late", "date": "2026-12-31", "department": "wecom:2", "employee_search": "段详政", "page": "2", "page_size": "1"}, []string{"day-start"}, 2},
		{map[string]string{"status": "late", "month": "2026-12", "employee_id": "first"}, []string{"day-end-z", "day-start"}, 2},
		{map[string]string{"status": "late", "start_time": "2026-12-31 08:00:00", "end_time": "2026-12-31 18:00:00", "search": "E002"}, []string{"same-name-clock"}, 1},
		{map[string]string{"status": "late", "department": "wecom:2", "include_subdepartments": "false"}, []string{}, 0},
		{map[string]string{"status": "normal"}, []string{"before"}, 1},
		{map[string]string{"status": "early"}, []string{"day-end-a"}, 1},
		{map[string]string{"status": "missing", "department": "host:10"}, []string{"local-clock"}, 1},
		{map[string]string{"status": "abnormal"}, []string{"fallback-clock"}, 1},
		{map[string]string{"status": "late", "date": "2026-12-31", "page": "4", "page_size": "1"}, []string{}, 3},
	} {
		check(ctx, tt.query, tt.ids, tt.total)
	}
	// Adding a status predicate must not broaden HRM's employee visibility.
	self := context.WithValue(ctx, hrmScopeKey{}, &hrmScope{EmployeeIDs: []string{"first"}, employees: map[string]bool{"first": true}})
	check(self, map[string]string{"status": "late", "date": "2026-12-31", "page": "2", "page_size": "1"}, []string{"day-start"}, 2)
	check(self, map[string]string{"status": "late", "employee_id": "same-name"}, []string{}, 0)
	check(context.WithValue(ctx, hrmScopeKey{}, &hrmScope{}), map[string]string{"status": "late"}, []string{}, 0)
}

// Every fixture lives in setup's disposable SQLite database, never the tenant DB.
func clockFixtures(t *testing.T) context.Context {
	t.Helper()
	ctx := setup(t)
	for _, dept := range []struct {
		id, parent int
		name       string
	}{{1, 0, "总公司"}, {2, 1, "分子公司"}, {3, 2, "研发部"}, {4, 1, "新入职员工"}, {5, 1, "研发部"}, {6, 7, "循环甲"}, {7, 6, "循环乙"}} {
		_, err := core.Model("wecom_department").Data(g.Map{"id": fmt.Sprint(dept.id), "wecom_id": dept.id, "parent_id": dept.parent, "name": dept.name, "raw_json": "{}", "synced_at": core.Now()}).Insert()
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := core.HostModel("auth_dept").Data(g.Map{"id": 11, "pid": 10, "name": "本地研发部"}).Insert()
	if err != nil {
		t.Fatal(err)
	}
	for _, employee := range []Employee{
		{ID: "first", Name: "段详政", WeComUserID: "wx-one", EmployeeNo: "E001"},
		{ID: "same-name", Name: "段详政", WeComUserID: "wx-two", EmployeeNo: "E002"},
		{ID: "local", Name: "本地员工", DeptID: 11, EmployeeNo: "E003"},
		{ID: "fallback", Name: "wx-three", WeComUserID: "wx-three", EmployeeNo: "E004"},
		{ID: "cycle", Name: "循环员工", WeComUserID: "wx-cycle", EmployeeNo: "E005"},
	} {
		if _, err = UpsertEmployee(ctx, employee); err != nil {
			t.Fatal(err)
		}
	}
	for _, user := range []struct{ userID, employee, name, departments string }{
		{"wx-one", "first", "段详政", "[3,4]"}, {"wx-two", "same-name", "段详政", "[5]"},
		{"wx-three", "fallback", "真实姓名", "[4]"}, {"wx-cycle", "cycle", "循环员工", "[6]"},
	} {
		_, err = core.Model("wecom_user").Data(g.Map{"id": user.userID, "wecom_userid": user.userID, "employee_id": user.employee, "name": user.name, "department_ids": user.departments, "mobile": "private-phone", "email": "private-email", "raw_json": "private-raw", "synced_at": core.Now()}).Insert()
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, clock := range []Clock{
		{ID: "before", EmployeeID: "first", ClockTime: "2026-12-30 23:59:59", ClockType: "in"},
		{ID: "day-start", EmployeeID: "first", ClockTime: "2026-12-31 00:00:00", ClockType: "in"},
		{ID: "day-end-z", EmployeeID: "first", ClockTime: "2026-12-31 23:59:59", ClockType: "out"},
		{ID: "day-end-a", EmployeeID: "first", ClockTime: "2026-12-31 23:59:59", ClockType: "out"},
		{ID: "after", EmployeeID: "first", ClockTime: "2027-01-01 00:00:00", ClockType: "in"},
		{ID: "same-name-clock", EmployeeID: "same-name", ClockTime: "2026-12-31 09:00:00", ClockType: "in"},
		{ID: "local-clock", EmployeeID: "local", ClockTime: "2026-12-31 10:00:00", ClockType: "in"},
		{ID: "fallback-clock", EmployeeID: "fallback", ClockTime: "2026-12-31 10:00:00", ClockType: "in"},
	} {
		if _, err = UpsertClock(ctx, clock); err != nil {
			t.Fatal(err)
		}
	}
	return ctx
}

func TestClockDirectoryNamesDepartmentsAndCombinedFilters(t *testing.T) {
	ctx := clockFixtures(t)
	d, err := loadClockDirectory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := d.departments["wecom:3"].Path; got != "总公司 / 分子公司 / 研发部" {
		t.Fatalf("department path %q", got)
	}
	first := d.employees["first"]
	if first.Name != "段详政" || first.EmployeeNo != "E001" || !reflect.DeepEqual(first.DepartmentNames, []string{"研发部", "新入职员工"}) {
		t.Fatalf("unbound WeCom member: %+v", first)
	}
	if d.employees["fallback"].Name != "真实姓名" {
		t.Fatal("userid placeholder shown instead of mapped member name")
	}
	if !d.inDepartment(first, "wecom:2", true) || d.inDepartment(first, "wecom:2", false) || !d.inDepartment(first, "wecom:4", false) || d.inDepartment(d.employees["same-name"], "wecom:2", true) {
		t.Fatal("subtree/multiple membership/same-name department rules wrong")
	}
	if !d.inDepartment(d.employees["local"], "host:10", true) || d.inDepartment(first, "host:10", true) {
		t.Fatal("host and WeCom department namespaces mixed")
	}
	if d.inDepartment(d.employees["cycle"], "wecom:1", true) {
		t.Fatal("cycle expanded into unrelated department")
	}
	encoded, _ := json.Marshal(d)
	for _, secret := range []string{"private-phone", "private-email", "private-raw", "raw_json", "mobile", "wecom_userid"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("options leaked %s", secret)
		}
	}
	cases := []struct {
		query map[string]string
		ids   []string
		total int
	}{
		{map[string]string{"date": "2026-12-31", "employee_id": "first"}, []string{"day-end-z", "day-end-a", "day-start"}, 3},
		{map[string]string{"date": "2026-12-31", "search": "段详政"}, []string{"day-end-z", "day-end-a", "same-name-clock", "day-start"}, 4},
		{map[string]string{"date": "2026-12-31", "search": "段详政", "employee_id": "same-name"}, []string{"same-name-clock"}, 1},
		{map[string]string{"date": "2026-12-31", "employee_search": " 段详政 ", "page": "2", "page_size": "2"}, []string{"same-name-clock", "day-start"}, 4},
		{map[string]string{"date": "2026-12-31", "employee_search": "段详政", "department": "wecom:2", "page": "2", "page_size": "1"}, []string{"day-end-a"}, 3},
		{map[string]string{"date": "2026-12-31", "employee_search": "e002", "department": "wecom:5"}, []string{"same-name-clock"}, 1},
		{map[string]string{"employee_search": "E002", "department": "wecom:2"}, []string{}, 0},
		{map[string]string{"employee_search": "E001", "employee_id": "same-name"}, []string{}, 0},
		{map[string]string{"employee_search": "真实姓名"}, []string{"fallback-clock"}, 1},
		{map[string]string{"employee_search": "first"}, []string{}, 0},
		{map[string]string{"employee_search": "' OR 1=1 --"}, []string{}, 0},
		{map[string]string{"month": "2026-12", "search": "E001", "department": "wecom:2", "page": "2", "page_size": "2"}, []string{"day-start", "before"}, 4},
		{map[string]string{"month": "2026-12", "department": "wecom:2", "include_subdepartments": "false"}, []string{}, 0},
		{map[string]string{"month": "2026-12", "department": "wecom:5"}, []string{"same-name-clock"}, 1},
		{map[string]string{"date": "2026-12-31", "department": "host:10"}, []string{"local-clock"}, 1},
		{map[string]string{"start_time": "2026-12-31", "end_time": "2026-12-31", "employee_id": "first"}, []string{"day-end-z", "day-end-a", "day-start"}, 3},
		{map[string]string{"month": "2027-01", "employee_id": "first"}, []string{"after"}, 1},
		{map[string]string{"search": "真实姓名"}, []string{"fallback-clock"}, 1},
		{map[string]string{"search": "' OR 1=1 --"}, []string{}, 0},
		{map[string]string{"department": "wecom:2", "employee_id": "same-name"}, []string{}, 0},
	}
	for _, tt := range cases {
		f, err := parseClockFilter(tt.query)
		if err != nil {
			t.Fatal(err)
		}
		rows, total, err := queryClocks(ctx, f, d)
		if err != nil {
			t.Fatalf("%v: %v", tt.query, err)
		}
		ids := []string{}
		for _, row := range rows {
			ids = append(ids, row["id"].(string))
			if row["employee_name"] == "" || row["department_names"] == nil {
				t.Fatalf("mapping missing: %v", row)
			}
		}
		if total != tt.total || !reflect.DeepEqual(ids, tt.ids) {
			t.Fatalf("%v: got %v total %d want %v total %d", tt.query, ids, total, tt.ids, tt.total)
		}
	}
	f, _ := parseClockFilter(map[string]string{"department": "wecom:999"})
	if _, _, err = queryClocks(ctx, f, d); err == nil {
		t.Fatal("unknown department silently ignored")
	}
	// Broken historical mappings remain visible, with an explicit empty name.
	_, err = core.Model("biz_hrm_attendance_clock").Where("id", "local-clock").Data(g.Map{"employee_id": "missing"}).Update()
	if err != nil {
		t.Fatal(err)
	}
	f, _ = parseClockFilter(map[string]string{"employee_id": "missing"})
	rows, total, err := queryClocks(ctx, f, d)
	if err != nil || total != 1 || rows[0]["employee_name"] != "" || rows[0]["employee_no"] != "" {
		t.Fatalf("orphan mapping %v %d %v", rows, total, err)
	}
	for _, name := range []string{"local", "Alex Chen"} {
		if _, err = core.Model("biz_hrm_employee").Where("id", "local").Data(g.Map{"name": name}).Update(); err != nil {
			t.Fatal(err)
		}
		d, err = loadClockDirectory(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := name
		if name == "local" {
			want = ""
		}
		if d.employees["local"].Name != want {
			t.Fatalf("machine ID or non-Chinese name mishandled: %q", d.employees["local"].Name)
		}
	}
}

func TestClockHTTPFiltersOptionsAndPagination(t *testing.T) {
	ctx := clockFixtures(t)
	s := g.Server(guid.S())
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) {
			if r.Header.Get("X-Test-UID") == "1" {
				r.SetCtx(context.WithValue(r.Context(), "uid", int64(1)))
			}
			r.Middleware.Next()
		})
		Bind(ctx, group)
	})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	call := func(path string, authenticated bool) (int, http.Header, map[string]any) {
		t.Helper()
		req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/api/admin/hrm/attendance/clocks%s", s.GetListenedPort(), path), nil)
		if authenticated {
			req.Header.Set("X-Test-UID", "1")
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusBadRequest {
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatal(err)
			}
		}
		return res.StatusCode, res.Header, body
	}
	for _, path := range []string{"", "/options"} {
		if code, _, _ := call(path, false); code != 401 {
			t.Fatalf("anonymous %s status %d", path, code)
		}
	}
	_, _, options := call("/options", true)
	if options["code"] != float64(0) || len(options["data"].(map[string]any)["employees"].([]any)) != 5 {
		t.Fatalf("options route not bound: %v", options)
	}
	query := url.Values{"date": {"2026-12-31"}, "employee_search": {"段详政"}, "department": {"wecom:2"}, "page": {"2"}, "page_size": {"1"}}
	_, headers, body := call("?"+query.Encode(), true)
	if headers.Get("X-Total-Count") != "3" || body["code"] != float64(0) {
		t.Fatalf("pagination contract: %v %v", headers, body)
	}
	rows := body["data"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["id"] != "day-end-a" || rows[0].(map[string]any)["employee_name"] != "段详政" {
		t.Fatalf("filtered page: %v", body)
	}
	query = url.Values{"employee_search": {"E002"}, "department": {"wecom:2"}}
	_, headers, body = call("?"+query.Encode(), true)
	if headers.Get("X-Total-Count") != "0" || body["code"] != float64(0) || len(body["data"].([]any)) != 0 {
		t.Fatalf("person search was not intersected with department: %v %v", headers, body)
	}
	for _, query := range []string{"?date=2026-12-31&month=2026-12", "?date=invalid", "?start_time=2026-12-31&end_time=2026-12-30", "?department=wecom:999"} {
		_, headers, body = call(query, true)
		if body["code"] == float64(0) || headers.Get("X-Total-Count") != "" {
			t.Fatalf("invalid filter returned success: %s %v", query, body)
		}
	}
	if _, err := core.Model("biz_hrm_attendance_clock").Ctx(ctx).WhereIn("id", []string{"day-start", "day-end-z"}).Data(g.Map{"status": "late"}).Update(); err != nil {
		t.Fatal(err)
	}
	query = url.Values{"status": {"late"}, "date": {"2026-12-31"}, "department": {"wecom:2"}, "employee_search": {"段详政"}, "page": {"2"}, "page_size": {"1"}}
	code, headers, body := call("?"+query.Encode(), true)
	if code != http.StatusOK || headers.Get("X-Total-Count") != "2" || body["code"] != float64(0) {
		t.Fatalf("status pagination contract: %d %v %v", code, headers, body)
	}
	rows = body["data"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["id"] != "day-start" || rows[0].(map[string]any)["status"] != "late" {
		t.Fatalf("status was not filtered before pagination: %v", body)
	}
	for _, status := range []string{"unknown", "迟到", "late,missing", "late' OR 1=1 --"} {
		code, headers, body := call("?status="+url.QueryEscape(status), true)
		if code != http.StatusBadRequest || body["code"] == float64(0) || headers.Get("X-Total-Count") != "" || !strings.Contains(fmt.Sprint(body["message"]), "打卡状态无效") {
			t.Fatalf("invalid status was not rejected with a Chinese error: %d %v %v", code, headers, body)
		}
	}
}

func TestClockEmployeeNumberSearchIgnoresInternalIDsAndReadsLegacyProfile(t *testing.T) {
	ctx := clockFixtures(t)
	const huID, wangID = "employee-hu", "internal-7319-wang"
	const raw = `{"userid":"wx-hu","name":"测试员工甲","extattr":{"attrs":[{"name":"工号","type":0,"text":{"value":"7319"}},{"name":"私人备注","type":0,"text":{"value":"private-remark"}}]}}`
	for _, employee := range []Employee{
		{ID: huID, Name: "测试员工甲", WeComUserID: "wx-hu"}, // Old HRM record has no employee_no yet.
		{ID: wangID, Name: "王芳", WeComUserID: "wx-wang", EmployeeNo: "3240"},
	} {
		if _, err := UpsertEmployee(ctx, employee); err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range []struct{ user, employee, name, departments, raw string }{
		{"wx-hu", huID, "测试员工甲", "[3]", raw},
		// An existing, explicit HRM employee number remains authoritative.
		{"wx-wang", wangID, "王芳", "[5]", `{"extattr":{"attrs":[{"name":"工号","type":0,"text":{"value":"stale-profile-number"}}]}}`},
	} {
		if _, err := core.Model("wecom_user").Data(g.Map{"id": item.user, "wecom_userid": item.user, "employee_id": item.employee, "name": item.name, "department_ids": item.departments, "raw_json": item.raw, "synced_at": core.Now()}).Insert(); err != nil {
			t.Fatal(err)
		}
	}
	for _, clock := range []Clock{
		{ID: "hu-in", EmployeeID: huID, ClockTime: "2026-12-31 08:30:00", ClockType: "in", Address: "深圳办公楼"},
		{ID: "hu-out", EmployeeID: huID, ClockTime: "2026-12-31 18:00:00", ClockType: "out", Address: "深圳办公楼"},
		{ID: "wang-out", EmployeeID: wangID, ClockTime: "2026-12-31 18:15:00", ClockType: "out", Address: "深圳办公楼"},
	} {
		if _, err := UpsertClock(ctx, clock); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := func(table string) string {
		t.Helper()
		rows, err := core.Model(table).Ctx(ctx).OrderAsc("id").All()
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(rows.List())
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}
	beforeEmployees, beforeMembers, beforeClocks := snapshot("biz_hrm_employee"), snapshot("wecom_user"), snapshot("biz_hrm_attendance_clock")
	d, err := loadClockDirectory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if d.employees[huID].EmployeeNo != "7319" || d.employees[wangID].EmployeeNo != "3240" {
		t.Fatal("legacy raw employee number fallback or existing HRM number precedence is incorrect")
	}
	encoded, _ := json.Marshal(d)
	if strings.Contains(string(encoded), "private-remark") || strings.Contains(string(encoded), "extattr") {
		t.Fatal("raw contact profile escaped the directory projection")
	}
	for _, tt := range []struct {
		query map[string]string
		ids   []string
		total int
	}{
		{map[string]string{"search": "7319"}, []string{"hu-out", "hu-in"}, 2},
		{map[string]string{"employee_search": "7319"}, []string{"hu-out", "hu-in"}, 2},
		{map[string]string{"employee_search": " 测试员工甲 "}, []string{"hu-out", "hu-in"}, 2},
		{map[string]string{"employee_search": "员工甲", "search": "深圳", "date": "2026-12-31", "department": "wecom:2", "page": "2", "page_size": "1"}, []string{"hu-in"}, 2},
		{map[string]string{"employee_search": "7319", "department": "wecom:5"}, []string{}, 0},
		{map[string]string{"employee_search": "3240"}, []string{"wang-out"}, 1},
		{map[string]string{"search": "3240"}, []string{"wang-out"}, 1},
		{map[string]string{"search": wangID}, []string{}, 0},
		{map[string]string{"employee_search": wangID}, []string{}, 0},
		{map[string]string{"employee_search": "stale-profile-number"}, []string{}, 0},
		{map[string]string{"employee_id": wangID}, []string{"wang-out"}, 1},
	} {
		filter, err := parseClockFilter(tt.query)
		if err != nil {
			t.Fatal(err)
		}
		rows, total, err := queryClocks(ctx, filter, d)
		if err != nil {
			t.Fatalf("query %v: %v", tt.query, err)
		}
		ids := []string{}
		for _, row := range rows {
			ids = append(ids, row["id"].(string))
			if row["employee_id"] == huID && (row["employee_no"] != "7319" || row["employee_name"] != "测试员工甲") {
				t.Fatal("legacy employee number was not projected onto clock rows")
			}
		}
		if total != tt.total || !reflect.DeepEqual(ids, tt.ids) {
			t.Fatalf("query %v returned %v total %d, want %v total %d", tt.query, ids, total, tt.ids, tt.total)
		}
	}
	if beforeEmployees != snapshot("biz_hrm_employee") || beforeMembers != snapshot("wecom_user") || beforeClocks != snapshot("biz_hrm_attendance_clock") {
		t.Fatal("read-time legacy profile fallback changed stored employee, contact, or clock records")
	}
	// All attendance lists share the visible employee-number mapping. A raw
	// internal ID must not reintroduce the same false positive in approvals.
	for _, employeeID := range []string{huID, wangID} {
		if _, err := UpsertLeave(ctx, Leave{ID: "leave-" + employeeID, EmployeeID: employeeID, Source: "manual", ApprovalStatus: "approved", LeaveType: "事假", StartTime: "2026-12-31 09:00:00", EndTime: "2026-12-31 10:00:00", Duration: 1, Reason: "办理个人事务"}); err != nil {
			t.Fatal(err)
		}
		for _, kind := range []string{"out", "trip"} {
			save := UpsertOut
			if kind == "trip" {
				save = UpsertTrip
			}
			if _, err := save(ctx, Out{ID: kind + "-" + employeeID, EmployeeID: employeeID, Source: "manual", ApprovalStatus: "approved", StartTime: "2026-12-31 09:00:00", EndTime: "2026-12-31 10:00:00", Duration: 1, Reason: "拜访客户", Destination: "深圳"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, kind := range []string{"leave", "out", "trip"} {
		filter, _ := parseClockFilter(map[string]string{"search": "7319"})
		rows, total, err := queryAbsences(ctx, kind, filter, d)
		if err != nil || total != 1 || len(rows) != 1 || rows[0]["employee_id"] != huID || rows[0]["employee_no"] != "7319" {
			t.Fatalf("%s employee-number search returned unrelated internal ID: total %d error %v", kind, total, err)
		}
		filter, _ = parseClockFilter(map[string]string{"search": wangID})
		rows, total, err = queryAbsences(ctx, kind, filter, d)
		if err != nil || total != 0 || len(rows) != 0 {
			t.Fatalf("%s internal ID was searched as employee text: total %d error %v", kind, total, err)
		}
	}
}
