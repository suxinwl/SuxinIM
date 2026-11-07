package hrm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func TestExclusionsHideCurrentDirectoryButPreserveAttendanceHistory(t *testing.T) {
	ctx := clockFixtures(t)
	for _, sql := range []string{
		`INSERT INTO wecom_settings(id,settings_json,updated_at) VALUES(1,'{"exclude_department_names":["分子公司"],"exclude_userids":["wx-one"],"include_subdepartments":true}','2026-10-29')`,
		`UPDATE biz_hrm_employee SET status='filtered' WHERE id='local'`,
		`UPDATE gf_auth_dept SET deletetime='2026-10-29' WHERE id=11`,
	} {
		if _, err := core.DB().Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	directory, err := loadClockDirectory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, employee := range directory.Employees {
		if employee.ID == "first" || employee.ID == "local" {
			t.Fatalf("filtered employee remained a candidate: %+v", employee)
		}
	}
	for _, department := range directory.Departments {
		if department.Key == "wecom:2" || department.Key == "wecom:3" || department.Key == "host:11" {
			t.Fatalf("filtered/deleted department remained a candidate: %+v", department)
		}
	}
	if directory.employees["first"].Name != "段详政" || directory.employees["local"].Name != "本地员工" || directory.departments["host:11"].Name != "本地研发部" {
		t.Fatal("historical name/department mappings were removed")
	}
	filter, _ := parseClockFilter(map[string]string{})
	rows, total, err := queryClocks(ctx, filter, directory)
	if err != nil || total != 8 || len(rows) != 8 {
		t.Fatalf("archiving altered historical attendance: total=%d err=%v", total, err)
	}
	for _, row := range rows {
		if row["employee_id"] == "first" && row["employee_name"] != "段详政" {
			t.Fatalf("historical clock lost employee name: %v", row)
		}
	}
	filter, _ = parseClockFilter(map[string]string{"search": "E001"})
	rows, total, err = queryClocks(ctx, filter, directory)
	if err != nil || total != 5 || rows[0]["employee_name"] != "段详政" {
		t.Fatalf("filtered employee history cannot be searched: %v total=%d err=%v", rows, total, err)
	}
	month, err := Month(ctx, "2026-12", "")
	if err != nil || len(month.Items) != 3 {
		t.Fatalf("default month still includes archived employees: %+v %v", month, err)
	}
	history, err := Month(ctx, "2026-12", "local")
	if err != nil || len(history.Items) != 1 {
		t.Fatalf("explicit historical month no longer available: %+v %v", history, err)
	}
	// Source approval updates and historical ETL must still be able to cancel
	// or correct an existing record after its employee leaves the directory.
	leave := Leave{EmployeeID: "local", Source: "wecom", ExternalRef: "old-leave", ApprovalStatus: "approved", StartTime: "2026-12-31 09:00:00", EndTime: "2026-12-31 10:00:00", Duration: 1}
	id, err := UpsertLeave(ctx, leave)
	if err != nil {
		t.Fatal(err)
	}
	leave.ApprovalStatus = "canceled"
	if after, err := UpsertLeave(ctx, leave); err != nil || after != id {
		t.Fatalf("source cancellation blocked: %s %v", after, err)
	}
	filter, _ = parseClockFilter(map[string]string{"search": "本地员工"})
	absences, total, err := queryAbsences(ctx, "leave", filter, directory)
	if err != nil || total != 1 || absences[0]["employee_name"] != "本地员工" {
		t.Fatalf("filtered employee absence history cannot be searched: %v total=%d err=%v", absences, total, err)
	}
	if _, err := UpsertClock(ctx, Clock{EmployeeID: "local", Source: "ruoyi", ExternalRef: "old-clock", ClockType: "in", ClockTime: "2026-12-30 09:00:00"}); err != nil {
		t.Fatalf("historical clock import blocked: %v", err)
	}
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) {
			r.SetCtx(context.WithValue(r.Context(), "uid", int64(1)))
			r.Middleware.Next()
		}, core.Middleware)
		Bind(ctx, group)
	})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	request := func(method, path, body string) map[string]any {
		t.Helper()
		req, _ := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d/api/admin/hrm/%s", server.GetListenedPort(), path), bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		var result map[string]any
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatalf("invalid response: %s", raw)
		}
		return result
	}
	if rows := request("GET", "employees", "")["data"].([]any); len(rows) != 3 {
		t.Fatalf("employee list has filtered employees: %v", rows)
	}
	if count := request("GET", "summary", "")["data"].(map[string]any)["employees"]; count != float64(3) {
		t.Fatalf("employee count includes filtered employees: %v", count)
	}
	if options := request("GET", "options", "")["data"].(map[string]any)["employees"].([]any); len(options) != 3 {
		t.Fatalf("employee selector includes filtered employees: %v", options)
	}
	for _, test := range []struct{ path, body string }{
		{"employees", `{"id":"local","name":"本地员工","status":"active"}`},
		{"employees", `{"id":"first","name":"段详政","wecom_userid":"changed","status":"active"}`},
		{"attendance/clocks", `{"employee_id":"local","clock_type":"in","clock_time":"2026-12-31 09:00:00"}`},
		{"attendance/leaves", `{"employee_id":"local","start_time":"2026-12-31 09:00:00","end_time":"2026-12-31 10:00:00"}`},
		{"attendance/outs", `{"employee_id":"local","start_time":"2026-12-31 09:00:00","end_time":"2026-12-31 10:00:00"}`},
		{"attendance/trips", `{"employee_id":"first","start_time":"2026-12-31 09:00:00","end_time":"2026-12-31 10:00:00"}`},
	} {
		response := request("POST", test.path, test.body)
		encoded, _ := json.Marshal(response)
		if !strings.Contains(string(encoded), "过滤") {
			t.Fatalf("manual write bypassed exclusion: %s %s", test.path, encoded)
		}
	}
}

func TestFilteredEmployeeCannotGrantSelfOrDepartmentScope(t *testing.T) {
	ctx := scopeFixtures(t)
	if _, err := core.Model("biz_hrm_employee").WhereIn("id", []string{"self", "child"}).Update(g.Map{"status": "filtered"}); err != nil {
		t.Fatal(err)
	}
	self, err := loadHRMScope(context.WithValue(ctx, "uid", int64(2)))
	if err != nil || len(self.EmployeeIDs) != 0 {
		t.Fatalf("filtered employee retained self access: %+v %v", self, err)
	}
	manager, err := loadHRMScope(context.WithValue(ctx, "uid", int64(3)))
	if err != nil || manager.employees["self"] || manager.employees["child"] || !manager.employees["manager"] {
		t.Fatalf("department scope includes filtered employees: %+v %v", manager, err)
	}
}
