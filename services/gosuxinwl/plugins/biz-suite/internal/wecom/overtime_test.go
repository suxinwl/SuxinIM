package wecom

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func overtimeFixture(sp string, status, attendanceType int) g.Map {
	d := approvalFixture(sp, "overtime-template", status)
	d["apply_data"].(g.Map)["contents"].([]any)[0] = g.Map{"id": "vacation-control", "control": "Attendance", "value": g.Map{"attendance": g.Map{"type": attendanceType, "date_range": g.Map{"new_begin": 1700000000, "new_end": 1700007200, "new_duration": 7200}, "slice_info": g.Map{"duration": 7200, "future_field": "preserved"}}}}
	return d
}
func overtimeTemplate() Template {
	return Template{TemplateID: "overtime-template", ApprovalType: "overtime", Enabled: 1, Mapping: Mapping{AttendanceControlID: "vacation-control", ReasonControlID: "reason-control"}}
}
func TestOvertimeTypeFiveLifecycleAndUnknownTemplate(t *testing.T) {
	ctx := setupDB(t)
	employee := seedEmployee(t, ctx, "alice")
	fixture := overtimeFixture("OT-1", 2, 5)
	var d approvalDetail
	_ = json.Unmarshal([]byte(mustJSON(fixture)), &d)
	var partial *PartialError
	if err := saveApproval(ctx, d, mustJSON(fixture)); !errors.As(err, &partial) {
		t.Fatalf("unmapped overtime must remain raw: %v", err)
	}
	if countRows(t, ctx, "biz_hrm_attendance_overtime") != 0 || countRows(t, ctx, "wecom_approval_raw") != 1 {
		t.Fatal("unmapped overtime became effective")
	}
	if err := SaveTemplate(ctx, overtimeTemplate()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := saveApproval(ctx, d, mustJSON(fixture)); err != nil {
			t.Fatal(err)
		}
	}
	assertStatus := func(want string) {
		t.Helper()
		row, e := core.Model("biz_hrm_attendance_overtime").Ctx(ctx).One()
		if e != nil || row["approval_status"].String() != want || row["employee_id"].String() != employee || row["duration"].Float64() != 2 || row["reason"].String() != "family" || !strings.Contains(row["raw_json"].String(), "future_field") || countRows(t, ctx, "biz_hrm_attendance_overtime") != 1 {
			t.Fatalf("overtime mismatch: %v %v", row, e)
		}
	}
	assertStatus("approved")
	bad := overtimeFixture("OT-1", 2, 2)
	_ = json.Unmarshal([]byte(mustJSON(bad)), &d)
	if err := saveApproval(ctx, d, mustJSON(bad)); !errors.As(err, &partial) {
		t.Fatalf("punch correction type 2 accepted as overtime: %v", err)
	}
	assertStatus("pending")
	_ = json.Unmarshal([]byte(mustJSON(fixture)), &d)
	if err := saveApproval(ctx, d, mustJSON(fixture)); err != nil {
		t.Fatal(err)
	}
	assertStatus("approved")
	for _, state := range []int{3, 4, 7} {
		d.Status = state
		d.ApplyData.Contents = nil
		d.Applyer.UserID = ""
		if err := saveApproval(ctx, d, mustJSON(d)); err != nil {
			t.Fatal(err)
		}
		want := "canceled"
		if state == 3 {
			want = "rejected"
		}
		assertStatus(want)
	}
	for _, table := range []string{"biz_hrm_attendance_leave", "biz_hrm_attendance_trip", "biz_hrm_attendance_out"} {
		if countRows(t, ctx, table) != 0 {
			t.Fatal("overtime routed into another kind")
		}
	}
}

func TestOvertimeIndependentSyncAndHistoricalReversalHook(t *testing.T) {
	ctx := setupDB(t)
	employee := seedEmployee(t, ctx, "alice")
	if err := SaveTemplate(ctx, overtimeTemplate()); err != nil {
		t.Fatal(err)
	}
	phase := 0
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if r.URL.Path == "/oa/getapprovalinfo" {
			filters := req["filters"].([]any)
			if len(filters) != 1 || filters[0].(map[string]any)["value"] != "overtime-template" {
				t.Error("overtime queried other template")
			}
			ids := []string{}
			if phase == 0 {
				ids = []string{"OT-SYNC"}
			}
			respond(w, g.Map{"errcode": 0, "sp_no_list": ids})
			return
		}
		if r.URL.Path != "/oa/getapprovaldetail" {
			t.Error("unexpected endpoint", r.URL.Path)
		}
		d := overtimeFixture("OT-SYNC", 2, 5)
		if phase > 0 {
			d = g.Map{"sp_no": "OT-SYNC", "template_id": "overtime-template", "sp_status": 7}
		}
		respond(w, g.Map{"errcode": 0, "info": d})
	})
	calls := 0
	SetAttendanceChangeHook(func(hctx context.Context, start, end time.Time, ids []string) error {
		calls++
		if hctx.Err() != nil || start.Unix() != 1700000000 || end.Unix() != 1700007200 || len(ids) != 1 || ids[0] != employee {
			t.Errorf("incorrect dirty range: %v %v %v", start, end, ids)
		}
		return nil
	})
	t.Cleanup(func() { SetAttendanceChangeHook(nil) })
	start, end := time.Unix(1699990000, 0), time.Unix(1700010000, 0)
	if _, n, err := c.Run(ctx, "overtime", start, end); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	phase = 1
	if _, _, err := c.Run(ctx, "overtime", end.Add(100*24*time.Hour), end.Add(101*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	row, _ := core.Model("biz_hrm_attendance_overtime").Ctx(ctx).One()
	if row["approval_status"].String() != "canceled" || calls != 2 {
		t.Fatal("historical cancellation missing", row, calls)
	}
	if countRows(t, ctx, "wecom_sync_cursor") != 1 {
		t.Fatal("overtime cursor contaminated other types")
	}
}

func TestAttendanceHookSurvivesCancellationAndErrorIsRetriable(t *testing.T) {
	dbctx := setupDB(t)
	ctx, cancel := context.WithCancel(dbctx)
	ctx, changes := collectAttendanceChanges(ctx)
	start := time.Unix(1700000000, 0)
	recordAttendanceChange(ctx, "employee", start, start)
	if err := enqueueAttendanceChanges(ctx, changes); err != nil {
		t.Fatal(err)
	}
	cancel()
	called := false
	SetAttendanceChangeHook(func(hctx context.Context, a, b time.Time, ids []string) error {
		called = true
		if hctx.Err() != nil || !b.After(a) {
			t.Fatal("callback retained canceled context")
		}
		return errors.New("private internal engine failure")
	})
	t.Cleanup(func() { SetAttendanceChangeHook(nil) })
	var result error
	changes.notify(ctx, &result)
	if result == nil || strings.Contains(result.Error(), "private") || !called {
		t.Fatal("unsafe or missing retry error", result)
	}
}
