package hrm

import (
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func adapterTime(value string) time.Time {
	t, e := time.ParseInLocation("2006-01-02 15:04:05", value, core.Location)
	if e != nil {
		panic(e)
	}
	return t
}

func TestAttendanceDailyEvidenceIsolatesOtherDatesAndRetainsNight(t *testing.T) {
	input := eng.Input{Date: "2026-10-29", Category: eng.Workday, Rule: eng.Rule{Window: eng.PunchWindow{InBeforeMinutes: 30, InAfterMinutes: 30, OutBeforeMinutes: 30, OutAfterMinutes: 30}}, Shift: eng.Shift{Segments: []eng.Segment{{ID: "night", Kind: "work", StartMinute: 1200, EndMinute: 1920}}}, Punches: []eng.Punch{{ID: "old", At: adapterTime("2026-10-28 09:00:00").Unix()}, {ID: "in", At: adapterTime("2026-10-29 19:45:00").Unix()}, {ID: "out", At: adapterTime("2026-10-30 08:20:00").Unix()}, {ID: "later", At: adapterTime("2026-10-31 09:00:00").Unix()}}, Absences: []eng.Absence{{ID: "old", StartAt: adapterTime("2026-10-01 09:00:00").Unix(), EndAt: adapterTime("2026-10-01 10:00:00").Unix()}, {ID: "night-leave", StartAt: adapterTime("2026-10-30 02:00:00").Unix(), EndAt: adapterTime("2026-10-30 04:00:00").Unix()}}}
	filtered := attDailyEvidence(input)
	if len(filtered.Punches) != 2 || filtered.Punches[0].ID != "in" || filtered.Punches[1].ID != "out" || len(filtered.Absences) != 1 {
		t.Fatal(filtered)
	}
	if len(input.Punches) != 4 || len(input.Absences) != 2 {
		t.Fatal("shared source slices mutated")
	}
}

func TestCompLeaveIssuesDoNotPoisonUnrelatedMonthOrDayShift(t *testing.T) {
	issues := []CompLeaveIssue{{LeaveID: "old", Start: adapterTime("2026-08-01 09:00:00"), End: adapterTime("2026-08-01 10:00:00"), Message: "历史余额不足"}, {LeaveID: "night", Start: adapterTime("2026-10-30 04:00:00"), End: adapterTime("2026-10-30 05:00:00"), Message: "夜班调休余额不足"}, {LeaveID: "unknown", Message: "历史源记录损坏"}}
	night := eng.Shift{Segments: []eng.Segment{{ID: "night", Kind: "work", StartMinute: 1200, EndMinute: 1920}}}
	day := eng.Shift{Segments: []eng.Segment{{ID: "day", Kind: "work", StartMinute: 540, EndMinute: 1080}}}
	if got := attCompIssuesForDate("2026-10-29", night, issues); len(got) != 1 || got[0] != "夜班调休余额不足" {
		t.Fatal(got)
	}
	if got := attCompIssuesForDate("2026-10-30", day, issues); len(got) != 0 {
		t.Fatal("unrelated day polluted", got)
	}
	if got := attCompIssuesForDate("2026-10-15", day, issues); len(got) != 0 {
		t.Fatal("whole month polluted", got)
	}
}

func TestAttendanceAdapterResolvesChineseLeaveAndPreservesDayUnit(t *testing.T) {
	ctx := setup(t)
	employee, e := UpsertEmployee(ctx, Employee{Name: "核算测试", DeptID: 10, EmployeeNo: "ATT1"})
	if e != nil {
		t.Fatal(e)
	}
	start, end := adapterTime("2026-10-29 09:00:00").Unix(), adapterTime("2026-10-29 18:00:00").Unix()
	raw := attJSON(g.Map{"apply_data": g.Map{"contents": []any{g.Map{"control": "Vacation", "value": g.Map{"vacation": g.Map{"selector": g.Map{"options": []any{g.Map{"key": "opaque-key", "value": []any{g.Map{"text": "年假", "lang": "zh_CN"}}}}}, "attendance": g.Map{"date_range": g.Map{"type": "day", "new_begin": start, "new_end": end}}}}}}}})
	_, e = UpsertLeave(ctx, Leave{EmployeeID: employee, Source: "wecom", ExternalRef: "unit-source", ApprovalStatus: "approved", LeaveType: "opaque-key", StartTime: "2026-10-29 09:00:00", EndTime: "2026-10-29 18:00:00", Duration: 8, RawJSON: raw})
	if e != nil {
		t.Fatal(e)
	}
	absences, e := attLoadAbsences(ctx, employee, "2026-10-29 00:00:00", "2026-10-30 00:00:00")
	if e != nil {
		t.Fatal(e)
	}
	if len(absences) != 1 || absences[0].LeaveType != "年假" || absences[0].Unit != "day" {
		t.Fatal(absences)
	}
	if got := attApprovalUnit(attJSON(g.Map{"info": g.Map{"apply_data": g.Map{"contents": []any{g.Map{"value": g.Map{"date_range": g.Map{"type": "hour", "new_begin": start, "new_end": end}}}}}}}), start, end); got != "hour" {
		t.Fatal(got)
	}
}

func TestAttendanceAdjustmentsRequireAuditableValidIntervals(t *testing.T) {
	base := AttAdjustmentRequest{WorkDate: "2026-10-29", Kind: "credit", Reason: "核查终端故障", Evidence: "主管确认记录", StartAt: adapterTime("2026-10-29 09:00:00").Unix(), EndAt: adapterTime("2026-10-29 18:00:00").Unix()}
	if e := attValidateAdjustment(base); e != nil {
		t.Fatal(e)
	}
	invalid := base
	invalid.Evidence = ""
	if attValidateAdjustment(invalid) == nil {
		t.Fatal("missing evidence accepted")
	}
	invalid = base
	invalid.EndAt = invalid.StartAt - 1
	if attValidateAdjustment(invalid) == nil {
		t.Fatal("reversed credit accepted")
	}
	invalid = base
	invalid.Kind = "punch"
	invalid.At = adapterTime("2026-11-12 09:00:00").Unix()
	invalid.PunchType = "in"
	if attValidateAdjustment(invalid) == nil {
		t.Fatal("wrong month punch accepted")
	}
	ctx := setup(t)
	employee, e := UpsertEmployee(ctx, Employee{Name: "补签测试", EmployeeNo: "ATT2", DeptID: 10})
	if e != nil {
		t.Fatal(e)
	}
	base.EmployeeID = employee
	base.Kind = "punch"
	base.At = adapterTime("2026-10-29 09:00:00").Unix()
	base.PunchType = "in"
	_, e = core.Model("biz_att_adjustment").Ctx(ctx).Data(g.Map{"id": "manual-in", "employee_id": employee, "work_date": base.WorkDate, "adjustment_json": attJSON(base), "reason": base.Reason, "evidence": base.Evidence, "created_by": 1, "created_at": core.Now()}).Insert()
	if e != nil {
		t.Fatal(e)
	}
	in := eng.Input{}
	rows, overrides, e := attAttachAdjustments(ctx, employee, base.WorkDate, &in)
	if e != nil {
		t.Fatal(e)
	}
	if len(rows) != 1 || len(overrides) != 0 || len(in.Punches) != 1 || in.Punches[0].ID != "adjustment:manual-in" {
		t.Fatal(in, rows, overrides)
	}
}
