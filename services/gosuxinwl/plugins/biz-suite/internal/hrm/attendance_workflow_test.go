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
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/xuri/excelize/v2"
)

// Exercise the real HTTP middleware and handlers with an isolated SQLite DB.
// No worker goroutine is started: tests explicitly advance queued calculations.
type attWorkflowClient struct {
	t    *testing.T
	base string
}

func newAttWorkflowClient(t *testing.T, ctx context.Context) attWorkflowClient {
	t.Helper()
	s := g.Server(guid.S())
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) {
			r.SetCtx(context.WithValue(r.Context(), "uid", gconv.Int64(r.Header.Get("X-Test-UID"))))
			r.Middleware.Next()
		}, core.Middleware)
		Bind(ctx, group)
	})
	if e := s.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = s.Shutdown() })
	return attWorkflowClient{t: t, base: fmt.Sprintf("http://127.0.0.1:%d/api/admin/hrm/attendance/v2/", s.GetListenedPort())}
}
func (c attWorkflowClient) request(uid int, method, path string, in any) (int, http.Header, []byte) {
	c.t.Helper()
	var body io.Reader
	if in != nil {
		b, e := json.Marshal(in)
		if e != nil {
			c.t.Fatal(e)
		}
		body = bytes.NewReader(b)
	}
	req, e := http.NewRequest(method, c.base+path, body)
	if e != nil {
		c.t.Fatal(e)
	}
	req.Header.Set("X-Test-UID", fmt.Sprint(uid))
	req.Header.Set("Content-Type", "application/json")
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		c.t.Fatal(e)
	}
	defer res.Body.Close()
	b, e := io.ReadAll(res.Body)
	if e != nil {
		c.t.Fatal(e)
	}
	return res.StatusCode, res.Header, b
}
func (c attWorkflowClient) json(uid int, method, path string, in any) (int, map[string]any) {
	c.t.Helper()
	status, _, raw := c.request(uid, method, path, in)
	body := map[string]any{}
	if status == 200 {
		if e := json.Unmarshal(raw, &body); e != nil {
			c.t.Fatalf("%s: %s", path, raw)
		}
	}
	return status, body
}
func (c attWorkflowClient) ok(method, path string, in any) map[string]any {
	c.t.Helper()
	status, body := c.json(1, method, path, in)
	if status != 200 || body["code"] != float64(0) {
		c.t.Fatalf("%s %s: status=%d body=%v", method, path, status, body)
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		c.t.Fatalf("%s non-object response: %v", path, body)
	}
	return data
}
func (c attWorkflowClient) rejected(method, path string, in any) {
	c.t.Helper()
	status, body := c.json(1, method, path, in)
	if status == 200 && body["code"] == float64(0) {
		c.t.Fatalf("%s %s unexpectedly accepted: %v", method, path, body)
	}
}

func attWorkflowRule() AttRule {
	return AttRule{Name: "测试制度", EffectiveFrom: "2026-08-01", Scope: AttScope{IsDefault: true}, Config: AttConfig{
		Category: "职员",
		Shifts:   []eng.Shift{{ID: "day", Name: "白班", Segments: []eng.Segment{{ID: "work", Kind: "work", StartMinute: 540, EndMinute: 1080, PunchMode: "both"}}}},
		Cycle:    AttCycle{Anchor: "2026-08-01", Days: []string{""}},
	}}
}
func attWorkflowMonth(t *testing.T, ctx context.Context, c attWorkflowClient) (employee, rule, batch string) {
	t.Helper()
	employee, e := UpsertEmployee(ctx, Employee{Name: "月报员工", EmployeeNo: "WF001", DeptID: 10})
	if e != nil {
		t.Fatal(e)
	}
	rule = c.ok("POST", "rules", attWorkflowRule())["id"].(string)
	c.ok("POST", "rules/"+rule+"/publish", g.Map{})
	generated := c.ok("POST", "rosters/generate", g.Map{"start_date": "2026-08-01", "end_date": "2026-08-31", "employee_ids": []string{employee}})
	if generated["saved"] != true || len(generated["issues"].([]any)) != 0 {
		t.Fatalf("rosters not generated: %v", generated)
	}
	batch = c.ok("POST", "batches", AttBatchRequest{StartDate: "2026-08-01", EndDate: "2026-08-31", EmployeeIDs: []string{employee}})["id"].(string)
	attProcessQueue(ctx)
	b, e := core.Model("biz_att_batch").Ctx(ctx).Where("id", batch).One()
	if e != nil || b["status"].String() != "completed" || b["completed_count"].Int() != 31 {
		t.Fatalf("calculation not complete: %v %v", b, e)
	}
	rows, e := core.Model("biz_att_result").Ctx(ctx).Where("batch_id", batch).WhereNotIn("status", []string{"rest", "calculated"}).All()
	if e != nil || len(rows) != 0 {
		t.Fatalf("unexpected review results: %v %v", rows, e)
	}
	return employee, rule, batch
}

func TestAttendanceWorkflowPublishedRulesAndLockedMonthAreImmutable(t *testing.T) {
	ctx := setup(t)
	c := newAttWorkflowClient(t, ctx)
	employee, rule, batch := attWorkflowMonth(t, ctx, c)
	original, e := attLoadRule(ctx, rule)
	if e != nil {
		t.Fatal(e)
	}
	edited := original
	edited.Name = "新草稿制度"
	edited.Config.Rule.LateGraceMinutes = 5
	newRule := c.ok("POST", "rules", edited)
	if newRule["id"] == rule || newRule["version"] != float64(2) || newRule["status"] != "draft" {
		t.Fatalf("published rule was overwritten: %v", newRule)
	}
	old, e := attLoadRule(ctx, rule)
	if e != nil || old.Status != "published" || old.Name != original.Name || old.Config.Rule.LateGraceMinutes != 0 {
		t.Fatalf("old rule changed: %+v %v", old, e)
	}
	for _, action := range []string{"review", "publish", "lock"} {
		c.ok("POST", "months/2026-08/action", g.Map{"action": action, "batch_id": batch, "reason": "月报闭环验证"})
	}
	result, e := core.Model("biz_att_result").Ctx(ctx).Where("batch_id", batch).Where("work_date", "2026-08-03").One()
	if e != nil {
		t.Fatal(e)
	}
	before := result["result_json"].String()
	c.rejected("POST", "rosters", g.Map{"items": []AttRoster{{EmployeeID: employee, WorkDate: "2026-08-03", RuleID: rule, ShiftID: "day", Category: eng.Workday, ScheduledWork: true}}, "reason": "锁定后不得改排班"})
	minutes := int64(480)
	c.rejected("POST", "adjustments", AttAdjustmentRequest{EmployeeID: employee, WorkDate: "2026-08-03", Kind: "override", CreditedMinutes: &minutes, Reason: "锁定后不得核定", Evidence: "签字单"})
	day, _ := attDate("2026-08-03")
	if e = AttendanceChanged(ctx, day, day, []string{employee}); e != nil {
		t.Fatal(e)
	}
	attProcessQueue(ctx)
	m, e := core.Model("biz_att_month").Ctx(ctx).Where("month", "2026-08").One()
	if e != nil || m["status"].String() != "locked" || m["change_pending"].Int() != 1 {
		t.Fatalf("dirty sync failed to flag locked month: %v %v", m, e)
	}
	after, _ := core.Model("biz_att_result").Ctx(ctx).Where("id", result["id"]).Value("result_json")
	if after.String() != before {
		t.Fatal("locked result rewritten by synchronization")
	}
	n, _ := core.Model("biz_att_publication").Ctx(ctx).Count()
	if n != 3 {
		t.Fatalf("expected 3 immutable publication records; got %d", n)
	}
	c.ok("POST", "months/2026-08/action", g.Map{"action": "reopen", "reason": "收到补充凭据"})
	n, _ = core.Model("biz_att_publication").Ctx(ctx).Count()
	if n != 4 {
		t.Fatal("reopen overwrote historical publications")
	}
	after, _ = core.Model("biz_att_result").Ctx(ctx).Where("id", result["id"]).Value("result_json")
	if after.String() != before {
		t.Fatal("reopen overwrote original result")
	}
}

func TestAttendanceWorkflowAdjustmentsRequireEvidenceAndAudit(t *testing.T) {
	ctx := setup(t)
	c := newAttWorkflowClient(t, ctx)
	employee, e := UpsertEmployee(ctx, Employee{Name: "核定员工", EmployeeNo: "WF002", DeptID: 10})
	if e != nil {
		t.Fatal(e)
	}
	minutes := int64(480)
	a := AttAdjustmentRequest{EmployeeID: employee, WorkDate: "2026-08-03", Kind: "override", CreditedMinutes: &minutes}
	c.rejected("POST", "adjustments", a)
	a.Reason = "人事依据纸质签到核定"
	c.rejected("POST", "adjustments", a)
	a.Evidence = "2026年8月3日签字签到单"
	id := c.ok("POST", "adjustments", a)["id"].(string)
	row, e := core.Model("biz_att_audit").Ctx(ctx).Where("entity", "adjustment").Where("entity_id", id).One()
	if e != nil || row["reason"].String() != a.Reason || row["created_by"].Int() != 1 || !strings.Contains(row["after_json"].String(), a.Evidence) {
		t.Fatalf("adjustment lost reason, evidence or actor: %v %v", row, e)
	}
	n, _ := core.Model("biz_att_adjustment").Ctx(ctx).Count()
	if n != 1 {
		t.Fatal("rejected adjustments left rows")
	}
}

func TestAttendanceWorkflowReviewRejectsStaleInputs(t *testing.T) {
	for _, change := range []string{"source", "roster"} {
		t.Run(change, func(t *testing.T) {
			ctx := setup(t)
			c := newAttWorkflowClient(t, ctx)
			employee, rule, batch := attWorkflowMonth(t, ctx, c)
			if change == "source" {
				if _, e := UpsertClock(ctx, Clock{EmployeeID: employee, ClockTime: "2026-08-03 09:00:00", ClockType: "in", Source: "wecom", ExternalRef: "new-source"}); e != nil {
					t.Fatal(e)
				}
				day, _ := attDate("2026-08-03")
				if e := AttendanceChanged(ctx, day, day, []string{employee}); e != nil {
					t.Fatal(e)
				}
			} else {
				c.ok("POST", "rosters", g.Map{"items": []AttRoster{{EmployeeID: employee, WorkDate: "2026-08-03", RuleID: rule, ShiftID: "day", Category: eng.Workday, ScheduledWork: true}}, "reason": "新增补班"})
			}
			c.rejected("POST", "months/2026-08/action", g.Map{"action": "review", "batch_id": batch, "reason": "旧批次不得通过复核"})
		})
	}
}

func TestAttendanceWorkflowScopedResultsExportsAndImports(t *testing.T) {
	ctx := scopeFixtures(t)
	c := newAttWorkflowClient(t, ctx)
	_, e := core.Model("biz_att_batch").Ctx(ctx).Data(g.Map{"id": "private-batch", "start_date": "2026-08-01", "end_date": "2026-08-31", "status": "completed", "request_json": "{}", "employee_count": 1, "total_count": 1, "completed_count": 1, "created_by": 1, "created_at": core.Now()}).Insert()
	if e != nil {
		t.Fatal(e)
	}
	_, e = core.Model("biz_att_result").Ctx(ctx).Data(g.Map{"id": "private-result", "batch_id": "private-batch", "employee_id": "other", "work_date": "2026-08-03", "rule_id": "rule", "status": "rest", "employee_json": attJSON(AttEmployee{ID: "other", Name: "绝不能泄露的员工", EmployeeNo: "PRIV001", Category: "职员"}), "result_json": attJSON(eng.Result{Date: "2026-08-03", State: "rest"}), "input_json": "{}", "created_at": core.Now()}).Insert()
	if e != nil {
		t.Fatal(e)
	}
	for _, uid := range []int{2, 5} {
		status, body := c.json(uid, "GET", "results?batch_id=private-batch", nil)
		if status != 200 || body["code"] != float64(0) || len(body["data"].(map[string]any)["items"].([]any)) != 0 {
			t.Fatalf("scoped result leaked uid %d: %d %v", uid, status, body)
		}
		for _, path := range []string{"results/private-result", "imports", "imports/private-import"} {
			status, body = c.json(uid, "GET", path, nil)
			if status == 200 && body["code"] == float64(0) {
				t.Fatalf("uid %d accessed %s", uid, path)
			}
		}
		status, body = c.json(uid, "POST", "rules", attWorkflowRule())
		if status != 403 {
			t.Fatalf("reader uid %d wrote a rule: %d %v", uid, status, body)
		}
		status, headers, raw := c.request(uid, "GET", "export?batch_id=private-batch", nil)
		if status == 200 && strings.Contains(headers.Get("Content-Type"), "spreadsheetml") {
			f, e := excelize.OpenReader(bytes.NewReader(raw))
			if e != nil {
				t.Fatal(e)
			}
			for _, sheet := range f.GetSheetList() {
				rows, e := f.GetRows(sheet)
				if e != nil {
					t.Fatal(e)
				}
				for _, row := range rows {
					for _, v := range row {
						if strings.Contains(v, "绝不能泄露") || strings.Contains(v, "PRIV001") {
							t.Fatalf("uid %d exported another employee", uid)
						}
					}
				}
			}
			_ = f.Close()
		} else if status != 403 {
			t.Fatalf("unexpected export response: %d %s", status, raw)
		}
	}
	for _, uid := range []int{0, 8} {
		status, _ := c.json(uid, "GET", "results", nil)
		if status != 401 && status != 403 {
			t.Fatalf("unavailable account uid %d read results: %d", uid, status)
		}
	}
}

func TestAttendanceWorkflowEvidenceImportIsIdempotent(t *testing.T) {
	ctx := setup(t)
	f := attendanceSyntheticWorkbook(t, "2026年8月考勤签收表")
	defer f.Close()
	buf, e := f.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	first, _, _, e := ImportAttendanceEvidence(ctx, bytes.NewReader(buf.Bytes()), "original.xlsx")
	if e != nil {
		t.Fatal(e)
	}
	again, _, _, e := ImportAttendanceEvidence(ctx, bytes.NewReader(buf.Bytes()), "renamed.xlsx")
	if e != nil || again != first {
		t.Fatalf("same evidence not idempotent: %s %s %v", first, again, e)
	}
	n, _ := core.Model("biz_att_import").Ctx(ctx).Count()
	if n != 1 {
		t.Fatal("duplicate import row")
	}
	n, _ = core.Model("biz_hrm_employee").Ctx(ctx).Count()
	if n != 0 {
		t.Fatal("evidence import created employees")
	}
	n, _ = core.Model("biz_att_rule").Ctx(ctx).Where("status", "published").Count()
	if n != 0 {
		t.Fatal("evidence import published rules")
	}
}
