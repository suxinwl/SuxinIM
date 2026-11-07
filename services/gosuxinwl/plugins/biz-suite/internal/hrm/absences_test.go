package hrm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func absenceFixture(t *testing.T) (context.Context, string, string, string) {
	t.Helper()
	ctx := clockFixtures(t)
	raw := `{"sp_no":"leave-100","sp_name":"请假申请","sp_status":2,"apply_time":1798678800,"applyer":{"userid":"wx-one"},"arbitrary_secret":"must-not-leak","apply_data":{"contents":[{"id":"vacation","control":"Vacation","title":[{"text":"请假明细","lang":"zh_CN"}],"value":{"vacation":{"selector":{"options":[{"key":"opaque-leave-key","value":[{"text":"Annual leave","lang":"en"},{"text":"年假","lang":"zh_CN"}]}]},"attendance":{"date_range":{"new_begin":1798678800,"new_end":1798686000,"new_duration":7200}}}}},{"id":"reason","control":"Textarea","title":[{"text":"请假事由","lang":"zh_CN"}],"value":{"text":"原始表单事由"}},{"id":"private","control":"Unrecognized","value":{"secret":"must-not-leak"}},{"id":"files","control":"File","value":{"files":[{"media_id":"private-media-id"}]}}]},"sp_record":[{"sp_status":2,"details":[{"approver":{"userid":"wx-three"},"speech":"已核对","sp_status":4,"sptime":1798678800,"media_id":["private-media-id"]},{"approver":{"userid":"unknown-private-id"},"speech":"","sp_status":99,"sptime":0}]}]}`
	id, err := UpsertLeave(ctx, Leave{EmployeeID: "first", Source: "wecom", ExternalRef: "leave-100", ApprovalStatus: "canceled", LeaveType: "opaque-leave-key", StartTime: "2026-12-31 09:00:00", EndTime: "2026-12-31 11:00:00", Duration: 2, Reason: "当前业务事由", RawJSON: raw})
	if err != nil {
		t.Fatal(err)
	}
	other, err := UpsertLeave(ctx, Leave{EmployeeID: "same-name", Source: "manual", ApprovalStatus: "approved", LeaveType: "病假", StartTime: "2026-12-31 10:00:00", EndTime: "2026-12-31 11:00:00", Duration: 1, Reason: "手工同名员工", RawJSON: raw})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"out", "trip"} {
		save := UpsertOut
		if kind == "trip" {
			save = UpsertTrip
		}
		if _, err := save(ctx, Out{ID: kind + "-manual", EmployeeID: "first", Source: "manual", ApprovalStatus: "approved", StartTime: "2026-12-31 09:00:00", EndTime: "2026-12-31 11:00:00", Duration: 2, Reason: "拜访客户", Destination: "深圳", RawJSON: raw}); err != nil {
			t.Fatal(err)
		}
	}
	return ctx, id, other, raw
}

func TestAbsenceListsProjectNamesTypesReasonsAndPagination(t *testing.T) {
	ctx, id, _, _ := absenceFixture(t)
	d, err := loadClockDirectory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f, err := parseClockFilter(map[string]string{"search": "段详政", "page_size": "1", "page": "2", "date": "2026-12-31"})
	if err != nil {
		t.Fatal(err)
	}
	rows, total, err := queryAbsences(ctx, "leave", f, d)
	if err != nil || total != 2 || len(rows) != 1 || rows[0]["id"] != id {
		t.Fatalf("combined pagination %v total %d error %v", rows, total, err)
	}
	row := rows[0]
	if row["employee_name"] != "段详政" || row["employee_no"] != "E001" || row["leave_type_name"] != "年假" || row["leave_type"] != "opaque-leave-key" || row["reason"] != "当前业务事由" || row["approval_status"] != "canceled" {
		t.Fatalf("projection incorrect: %v", row)
	}
	if _, ok := row["raw_json"]; ok {
		t.Fatal("raw JSON escaped list")
	}
	f, _ = parseClockFilter(map[string]string{"employee_id": "same-name", "search": "段详政"})
	rows, total, err = queryAbsences(ctx, "leave", f, d)
	if err != nil || total != 1 || rows[0]["employee_no"] != "E002" || rows[0]["leave_type_name"] != "病假" {
		t.Fatal("same-name filter mixed employees")
	}
	for _, kind := range []string{"out", "trip"} {
		f, _ = parseClockFilter(map[string]string{"search": "段详政", "department": "wecom:2"})
		rows, total, err = queryAbsences(ctx, kind, f, d)
		if err != nil || total != 1 || rows[0]["employee_name"] != "段详政" || rows[0]["reason"] != "拜访客户" {
			t.Fatalf("%s enrichment missing: %v %v", kind, rows, err)
		}
	}
	if got := absenceLeaveLabel(`{}`, "123", "wecom"); got != "未提供假期名称" {
		t.Fatalf("unknown leave key guessed: %s", got)
	}
}

func TestAbsenceDetailWhitelistsCurrentBusinessAndMappedTimeline(t *testing.T) {
	ctx, id, manual, raw := absenceFixture(t)
	item, err := readAbsenceDetail(ctx, "leave", id)
	if err != nil {
		t.Fatal(err)
	}
	if item["approval_status"] != "canceled" || item["reason"] != "当前业务事由" || item["duration"] != float64(2) {
		t.Fatal("old raw approval overwrote current HRM business record")
	}
	form := item["approval_form"].(approvalForm)
	if form.Applicant["name"] != "段详政" || form.Title != "请假申请" || form.ApplyTime == "" || len(form.Timeline) != 2 {
		t.Fatalf("form missing: %+v", form)
	}
	if form.Timeline[0].Name != "真实姓名" || form.Timeline[0].StatusName != "已转审" || form.Timeline[0].Time == "" || form.Timeline[1].Name != "未匹配审批人" || form.Timeline[1].Time != "" || form.Timeline[1].Status != "unknown" {
		t.Fatalf("timeline mapping: %+v", form.Timeline)
	}
	encoded, _ := json.Marshal(item)
	for _, secret := range []string{"raw_json", "detail_json", "must-not-leak", "private-media-id", "unknown-private-id", "wx-one", "wx-three"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("detail leaked %s", secret)
		}
	}
	if !strings.Contains(string(encoded), "原始表单事由") || !strings.Contains(string(encoded), "此类控件暂不支持展示") {
		t.Fatal("readable source form missing")
	}
	manualItem, err := readAbsenceDetail(ctx, "leave", manual)
	if err != nil {
		t.Fatal(err)
	}
	manualForm := manualItem["approval_form"].(approvalForm)
	if len(manualForm.Timeline) != 0 || len(manualForm.Fields) != 0 || manualForm.ApplyTime != "" {
		t.Fatal("manual record parsed unrelated raw approval")
	}
	for _, kind := range []string{"out", "trip"} {
		item, err := readAbsenceDetail(ctx, kind, kind+"-manual")
		if err != nil || item["destination"] != "深圳" {
			t.Fatalf("%s detail: %v", kind, err)
		}
	}
	// Wrong reference/applicant documents must not expose somebody else's form.
	for _, invalid := range []string{strings.Replace(raw, `"leave-100"`, `"other-record"`, 1), strings.Replace(raw, `"wx-one"`, `"other-applicant"`, 1), `{broken`} {
		_, err = core.Model("biz_hrm_attendance_leave").Where("id", id).Data(g.Map{"raw_json": invalid}).Update()
		if err != nil {
			t.Fatal(err)
		}
		item, err = readAbsenceDetail(ctx, "leave", id)
		if err != nil {
			t.Fatal(err)
		}
		if form := item["approval_form"].(approvalForm); len(form.Timeline) != 0 || !strings.Contains(form.Notice, "未取得可核对") {
			t.Fatal("unverified source document displayed")
		}
	}
	if _, err := readAbsenceDetail(ctx, "leave", "not-found"); err == nil {
		t.Fatal("missing record succeeded")
	}
	if _, err := readAbsenceDetail(ctx, "other", id); err == nil {
		t.Fatal("unknown kind accepted")
	}
	if approvalTimestamp(json.RawMessage(`0`)) != "" || approvalTimestamp(json.RawMessage(`"nonsense"`)) != "" {
		t.Fatal("unknown time guessed")
	}
}

func TestAbsenceHTTPListAndDetailContracts(t *testing.T) {
	ctx, id, _, _ := absenceFixture(t)
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
	call := func(path string, auth bool) (int, http.Header, map[string]any) {
		t.Helper()
		req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/api/admin/hrm/attendance/%s", s.GetListenedPort(), path), nil)
		if auth {
			req.Header.Set("X-Test-UID", "1")
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var body map[string]any
		if response.StatusCode == 200 {
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
		}
		return response.StatusCode, response.Header, body
	}
	for _, path := range []string{"leaves", "leaves/" + id, "outs/out-manual", "trips/trip-manual"} {
		if status, _, _ := call(path, false); status != 401 {
			t.Fatalf("anonymous %s status %d", path, status)
		}
	}
	_, headers, body := call("leaves?search="+url.QueryEscape("段详政")+"&page_size=1", true)
	if headers.Get("X-Total-Count") != "2" || body["code"] != float64(0) {
		t.Fatal("list response contract failed")
	}
	_, _, body = call("leaves/"+id, true)
	if body["code"] != float64(0) || body["data"].(map[string]any)["employee_name"] != "段详政" {
		t.Fatal("detail route failed")
	}
	for _, path := range []string{"outs/out-manual", "trips/trip-manual"} {
		_, _, body = call(path, true)
		if body["code"] != float64(0) {
			t.Fatalf("detail %s failed", path)
		}
	}
	_, _, body = call("leaves/missing", true)
	if body["code"] == float64(0) {
		t.Fatal("missing detail accepted")
	}
}
