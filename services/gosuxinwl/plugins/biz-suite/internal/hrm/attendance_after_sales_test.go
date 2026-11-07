package hrm

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func attAfterSalesFixture() AttRule {
	a := attWorkScheduleFixture("after_sales")
	a.Scope = AttScope{EmployeeIDs: []string{"member", "member2", "member3"}}
	a.Config.WorkSchedule = &AttWorkSchedule{Mode: "after_sales", DutyGroup: "qa-after-sales", DutyExpectedMembers: 3, WeekdayShiftIDs: []string{"regular", "duty"}, SaturdayShiftIDs: []string{"sat-regular", "sat-duty"}, RestdayShiftIDs: []string{"regular", "duty"}, DutyShiftIDs: []string{"duty", "sat-duty"}}
	a.Config.Shifts = nil
	for _, id := range []string{"regular", "duty", "sat-regular", "sat-duty"} {
		end, outStart, outEnd := 18*60, 17*3600+30*60, 18*3600+30*60
		if strings.Contains(id, "duty") {
			end, outStart, outEnd = 20*60, 18*3600+30*60, 22*3600
		}
		a.Config.Shifts = append(a.Config.Shifts, eng.Shift{ID: id, Name: id, Segments: []eng.Segment{
			{ID: "morning", Kind: "work", StartMinute: 8*60 + 30, EndMinute: 12 * 60, PunchMode: "both", LocalWindows: attAutoWindows(7*3600, 10*3600, 11*3600, 12*3600+30*60)},
			{ID: "afternoon", Kind: "work", StartMinute: 13*60 + 30, EndMinute: end, PunchMode: "both", LocalWindows: attAutoWindows(13*3600, 15*3600, outStart, outEnd)},
		}})
	}
	return a
}

func attAfterSalesPunches(date, end string) []eng.Punch {
	out := []eng.Punch{}
	for i, clock := range []string{"08:30:00", "12:00:00", "13:30:00", end} {
		if clock == "" {
			continue
		}
		out = append(out, eng.Punch{ID: date + "-" + attJSON(i), At: tm(date + " " + clock).Unix(), Type: "out", Status: "early"})
	}
	return out
}

func TestAfterSalesValidatesExplicitGroupRolesAndSeparateExitWindows(t *testing.T) {
	a := attAfterSalesFixture()
	if err := validateAttConfig(a); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(*AttRule)
		want string
	}{
		{"group", func(a *AttRule) { a.Config.WorkSchedule.DutyGroup = "" }, "留守组编号"},
		{"roles", func(a *AttRule) { a.Config.WorkSchedule.DutyShiftIDs = nil }, "标记留守"},
		{"saturday_roles", func(a *AttRule) { a.Config.WorkSchedule.DutyShiftIDs = []string{"duty"} }, "标记留守"},
		{"overlap", func(a *AttRule) { a.Config.Shifts[0].Segments[1].LocalWindows.OutEndSecond = localSecond(21 * 3600) }, "窗口重叠"},
		{"scope", func(a *AttRule) { a.Scope = AttScope{DepartmentIDs: []int64{10}} }, "明确选择"},
		{"partial_group", func(a *AttRule) { a.Scope.EmployeeIDs = []string{"member"} }, "应有3"},
		{"duplicate_group", func(a *AttRule) { a.Scope.EmployeeIDs = []string{"member", "member", "member"} }, "应有3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := attAfterSalesFixture()
			tc.edit(&a)
			if err := validateAttConfig(a); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted %q, got %v", tc.want, err)
			}
		})
	}
}

func TestAfterSalesActualEvidenceSelectsRegularDutyOrReview(t *testing.T) {
	ctx := setup(t)
	a := attAfterSalesFixture()
	for _, tc := range []struct{ date, end, shift string }{
		{"2026-10-09", "18:00:00", "regular"},
		{"2026-10-09", "18:15:00", "regular"},
		{"2026-10-09", "20:00:00", "duty"},
		{"2026-10-10", "18:00:00", "sat-regular"},
		{"2026-10-10", "20:00:00", "sat-duty"},
		{"2026-10-09", "19:00:00", ""},
		{"2026-10-09", "17:45:00", ""},
		{"2026-10-09", "", ""},
	} {
		day, err := attCycleDay(a.Config, tc.date)
		if err != nil {
			t.Fatal(err)
		}
		got, issue, err := attSelectCompleteShift(ctx, AttEmployee{ID: "member"}, tc.date, day, a, attAfterSalesPunches(tc.date, tc.end), nil)
		if err != nil || got != tc.shift || (issue == "") != (tc.shift != "") {
			t.Fatalf("%s %s: shift=%s issue=%s error=%v", tc.date, tc.end, got, issue, err)
		}
	}
}

func TestAfterSalesGroupCalculationExpandsAndRejectsNonUniqueDuty(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		ends       []string
	}{
		{"unique", "after_sales_duty_verified", []string{"20:00:00", "18:00:00", "18:00:00"}},
		{"multiple", "after_sales_duty_multiple", []string{"20:00:00", "20:00:00", "18:00:00"}},
		{"missing_peer", "after_sales_duty_incomplete", []string{"20:00:00", "18:00:00", ""}},
		{"no_duty", "after_sales_duty_missing", []string{"18:00:00", "18:00:00", "18:00:00"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.WithValue(setup(t), "uid", int64(1))
			people := []string{}
			for i, end := range tc.ends {
				id := attDirtyEmployee(t, ctx, "留守测试"+attJSON(i))
				people = append(people, id)
				for _, punch := range attAfterSalesPunches("2026-10-09", end) {
					_, err := UpsertClock(ctx, Clock{ID: id + punch.ID, EmployeeID: id, ClockTime: timeString(punch.At), ClockType: punch.Type, Status: punch.Status, Source: "manual"})
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			a := attAfterSalesFixture()
			a.Status, a.Scope = "published", AttScope{EmployeeIDs: people}
			attAutoStoreRule(t, ctx, a)
			// A request for a single employee still verifies the entire group.
			id, err := attQueueBatch(ctx, AttBatchRequest{StartDate: "2026-10-09", EndDate: "2026-10-09", EmployeeIDs: people[:1]})
			if err != nil {
				t.Fatal(err)
			}
			attProcessQueue(ctx)
			batch, _ := core.Model("biz_att_batch").Ctx(ctx).Where("id", id).One()
			if batch["status"].String() != "completed" || batch["employee_count"].Int() != 3 {
				t.Fatalf("group was not fully calculated: %v", batch)
			}
			rows, _ := core.Model("biz_att_result").Ctx(ctx).Where("batch_id", id).All()
			if len(rows) != 3 {
				t.Fatalf("expected all members, got %d", len(rows))
			}
			for _, row := range rows {
				var result eng.Result
				_ = json.Unmarshal([]byte(row["result_json"].String()), &result)
				last := result.Trace[len(result.Trace)-1]
				if last.Code != tc.code || len(last.EvidenceIDs) != 3 || (result.State == "calculated") != (tc.name == "unique") {
					t.Fatalf("unverified uniqueness or wrong group outcome: %+v", result)
				}
				var input map[string]json.RawMessage
				_ = json.Unmarshal([]byte(row["input_json"].String()), &input)
				if len(input["after_sales_group"]) == 0 {
					t.Fatal("group evidence was not retained")
				}
			}
			if tc.name == "unique" {
				// A subsequent changed clock expands the same group and creates
				// new reviewed results, without rewriting the earlier evidence.
				before := attJSON(rows)
				_, err = core.Model("biz_hrm_attendance_clock").Ctx(ctx).Where("employee_id", people[1]).Where("clock_time", "2026-10-09 18:00:00").Update(g.Map{"clock_time": "2026-10-09 20:00:00"})
				if err != nil {
					t.Fatal(err)
				}
				next, err := attQueueBatch(ctx, AttBatchRequest{StartDate: "2026-10-09", EndDate: "2026-10-09", EmployeeIDs: people[1:2], Automatic: true})
				if err != nil {
					t.Fatal(err)
				}
				attProcessQueue(ctx)
				newRows, _ := core.Model("biz_att_result").Ctx(ctx).Where("batch_id", next).All()
				for _, row := range newRows {
					if row["status"].String() != "review" {
						t.Fatal("incremental second duty did not invalidate whole group")
					}
				}
				oldRows, _ := core.Model("biz_att_result").Ctx(ctx).Where("batch_id", id).All()
				if !reflect.DeepEqual(before, attJSON(oldRows)) {
					t.Fatal("completed historical results were overwritten")
				}
			}
		})
	}
}

func TestAfterSalesSundayUsesPartialActualAttendanceWithoutDutyRequirement(t *testing.T) {
	ctx := setup(t)
	a := attAfterSalesFixture()
	const date = "2026-10-11"
	day, err := attCycleDay(a.Config, date)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                           string
		clocks                         []string
		wantShift, wantState           string
		actualMinutes, overtimeMinutes int64
	}{
		{"morning_only_equivalent_templates", []string{"08:30:00", "12:00:00"}, "regular", "calculated", 210, 210},
		{"afternoon_partial", []string{"13:30:00", "17:45:00"}, "regular", "calculated", 255, 255},
		{"partial_extended_shift", []string{"13:30:00", "19:00:00"}, "duty", "calculated", 330, 330},
		{"no_attendance", nil, "", "rest", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			punches := []eng.Punch{}
			for _, clock := range tc.clocks {
				punches = append(punches, eng.Punch{ID: clock, At: tm(date + " " + clock).Unix(), Type: "out", Status: "early"})
			}
			id, issue, err := attSelectCompleteShift(ctx, AttEmployee{ID: "member"}, date, day, a, punches, nil)
			if err != nil || issue != "" || id != tc.wantShift {
				t.Fatalf("shift=%q issue=%q err=%v", id, issue, err)
			}
			in := eng.Input{EmployeeID: "member", Date: date, Category: day.Category, ScheduledWork: day.ScheduledWork, Rule: a.Config.Rule, Punches: punches, AsOf: tm("2026-10-12 10:00:00").Unix()}
			for _, shift := range a.Config.Shifts {
				if shift.ID == id {
					in.Shift = shift
				}
			}
			result := eng.Calculate(in)
			if result.State != tc.wantState || result.ActualSeconds != tc.actualMinutes*60 || result.OvertimeSeconds != tc.overtimeMinutes*60 || result.MissingCount != 0 || result.LateCount != 0 || result.EarlyCount != 0 {
				t.Fatalf("rest-day attendance changed: %+v", result)
			}
		})
	}
	// A required Sunday is a workday exception, not voluntary overtime.
	day.ScheduledWork = true
	id, issue, err := attSelectCompleteShift(ctx, AttEmployee{ID: "member"}, date, day, a, attAfterSalesPunches(date, "19:00:00"), nil)
	if err != nil || id != "" || issue == "" {
		t.Fatalf("required work bypassed full evidence: %q %q %v", id, issue, err)
	}
	// Similar punch windows are not enough when templates produce different
	// actual work; only identical active evidence/accounting can be collapsed.
	day.ScheduledWork = false
	a.Config.Shifts[1].Segments[0].EndMinute = 11*60 + 30
	id, issue, err = attSelectCompleteShift(ctx, AttEmployee{ID: "member"}, date, day, a, attAfterSalesPunches(date, "")[:2], nil)
	if err != nil || id != "" || !strings.Contains(issue, "不同的班次核算结果") {
		t.Fatalf("different rest-day results silently merged: %q %q %v", id, issue, err)
	}
}

func TestAfterSalesSundayBatchKeepsRestAndRejectsPartialPermissionScope(t *testing.T) {
	ctx := context.WithValue(setup(t), "uid", int64(1))
	const date = "2026-10-11"
	people := []string{}
	for i := 0; i < 3; i++ {
		people = append(people, attDirtyEmployee(t, ctx, "周日留守测试"+attJSON(i)))
	}
	for _, punch := range attAfterSalesPunches(date, "")[:2] {
		if _, err := UpsertClock(ctx, Clock{ID: people[0] + punch.ID, EmployeeID: people[0], ClockTime: timeString(punch.At), ClockType: punch.Type, Status: punch.Status, Source: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
	a := attAfterSalesFixture()
	a.Status, a.Scope = "published", AttScope{EmployeeIDs: people}
	attAutoStoreRule(t, ctx, a)
	limited := context.WithValue(ctx, hrmScopeKey{}, &hrmScope{EmployeeIDs: people[:1], employees: map[string]bool{people[0]: true}})
	if _, err := attQueueBatch(limited, AttBatchRequest{StartDate: date, EndDate: date, EmployeeIDs: people[:1]}); err == nil || !strings.Contains(err.Error(), "数据权限") {
		t.Fatalf("partial group scope was allowed: %v", err)
	}
	batchID, err := attQueueBatch(ctx, AttBatchRequest{StartDate: date, EndDate: date, EmployeeIDs: people[:1]})
	if err != nil {
		t.Fatal(err)
	}
	attProcessQueue(ctx)
	batch, _ := core.Model("biz_att_batch").Ctx(ctx).Where("id", batchID).One()
	if batch["status"].String() != "completed" {
		t.Fatalf("Sunday batch failed: %v", batch)
	}
	rows, err := core.Model("biz_att_result").Ctx(ctx).Where("batch_id", batchID).All()
	if err != nil || len(rows) != 3 {
		t.Fatalf("group rows=%d err=%v", len(rows), err)
	}
	for _, row := range rows {
		var result eng.Result
		if err := json.Unmarshal([]byte(row["result_json"].String()), &result); err != nil {
			t.Fatal(err)
		}
		wantState, wantSeconds := "rest", int64(0)
		if row["employee_id"].String() == people[0] {
			wantState, wantSeconds = "calculated", 210*60
		}
		if result.State != wantState || result.ActualSeconds != wantSeconds || result.OvertimeSeconds != wantSeconds || len(result.Issues) > 0 {
			t.Fatalf("duty review changed voluntary Sunday attendance: %+v", result)
		}
		for _, trace := range result.Trace {
			if strings.HasPrefix(trace.Code, "after_sales_duty_") {
				t.Fatal("rest day incorrectly required duty verification")
			}
		}
	}
}

func TestAfterSalesFinalReviewNeedsPhysicalDutyEndOrReviewedPunch(t *testing.T) {
	for _, reviewedPunch := range []bool{false, true} {
		name := "approved_leave_is_not_duty_presence"
		if reviewedPunch {
			name = "reviewed_punch_proves_duty_end"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.WithValue(setup(t), "uid", int64(1))
			const date = "2026-10-09"
			people := []string{}
			for i := 0; i < 3; i++ {
				id := attDirtyEmployee(t, ctx, "最终核验"+attJSON(i))
				people = append(people, id)
				punches := attAfterSalesPunches(date, "18:00:00")
				if i == 0 {
					punches = nil
					if reviewedPunch {
						punches = attAfterSalesPunches(date, "")
					}
				}
				for _, punch := range punches {
					if _, err := UpsertClock(ctx, Clock{ID: id + punch.ID, EmployeeID: id, ClockTime: timeString(punch.At), ClockType: punch.Type, Status: punch.Status, Source: "manual"}); err != nil {
						t.Fatal(err)
					}
				}
			}
			a := attAfterSalesFixture()
			a.Status, a.Scope = "published", AttScope{EmployeeIDs: people}
			attAutoStoreRule(t, ctx, a)
			if err := attPutRosters(ctx, []AttRoster{{EmployeeID: people[0], WorkDate: date, RuleID: a.ID, ShiftID: "duty", Category: eng.Workday, ScheduledWork: true, Source: "manual"}}, "已确认留守安排"); err != nil {
				t.Fatal(err)
			}
			if reviewedPunch {
				adj := AttAdjustmentRequest{ID: "reviewed-duty-end", EmployeeID: people[0], WorkDate: date, Kind: "punch", At: tm(date + " 20:00:00").Unix(), PunchType: "out", Reason: "已核实下班卡", Evidence: "主管签字记录"}
				_, err := core.Model("biz_att_adjustment").Ctx(ctx).Insert(g.Map{"id": adj.ID, "employee_id": people[0], "work_date": date, "adjustment_json": attJSON(adj), "reason": adj.Reason, "evidence": adj.Evidence, "created_by": 1, "created_at": core.Now()})
				if err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := UpsertLeave(ctx, Leave{ID: "full-duty-leave", EmployeeID: people[0], Source: "manual", ApprovalStatus: "approved", LeaveType: "年假", StartTime: date + " 08:30:00", EndTime: date + " 20:00:00", Duration: 11.5, Reason: "全日请假", RawJSON: "{}"}); err != nil {
					t.Fatal(err)
				}
			}
			batchID, err := attQueueBatch(ctx, AttBatchRequest{StartDate: date, EndDate: date, EmployeeIDs: people[:1]})
			if err != nil {
				t.Fatal(err)
			}
			attProcessQueue(ctx)
			batch, _ := core.Model("biz_att_batch").Ctx(ctx).Where("id", batchID).One()
			if batch["status"].String() != "completed" {
				t.Fatalf("batch failed: %v", batch)
			}
			rows, err := core.Model("biz_att_result").Ctx(ctx).Where("batch_id", batchID).All()
			if err != nil || len(rows) != 3 {
				t.Fatalf("group rows=%d error=%v", len(rows), err)
			}
			for _, row := range rows {
				var result eng.Result
				if err := json.Unmarshal([]byte(row["result_json"].String()), &result); err != nil {
					t.Fatal(err)
				}
				want := "after_sales_duty_incomplete"
				if reviewedPunch {
					want = "after_sales_duty_verified"
				}
				if result.Trace[len(result.Trace)-1].Code != want || (result.State == "calculated") != reviewedPunch {
					t.Fatalf("duty evidence incorrectly accepted/rejected: %+v", result)
				}
			}
		})
	}
}
