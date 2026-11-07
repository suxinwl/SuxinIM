package hrm

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func attDirtyEmployee(t *testing.T, ctx context.Context, name string) string {
	t.Helper()
	id, e := UpsertEmployee(ctx, Employee{Name: name, EmployeeNo: name, DeptID: 10})
	if e != nil {
		t.Fatal(e)
	}
	return id
}

func attDirtyRoster(t *testing.T, ctx context.Context, employee, date, status string) {
	t.Helper()
	roster := AttRoster{ID: externalID("roster", employee+":"+date), EmployeeID: employee, WorkDate: date, RuleID: "fixture-rule", ShiftID: "day", Status: status, Source: "manual"}
	_, e := core.Model("biz_att_roster").Ctx(ctx).Data(g.Map{
		"id": roster.ID, "employee_id": employee, "work_date": date,
		"rule_id": roster.RuleID, "shift_id": roster.ShiftID, "status": status,
		"source": roster.Source, "roster_json": attJSON(roster), "updated_by": 1, "updated_at": core.Now(),
	}).Insert()
	if e != nil {
		t.Fatal(e)
	}
}

func attDirtyRun(t *testing.T, ctx context.Context, start, end string, employees []string) []AttBatchRequest {
	t.Helper()
	_, e := core.Model("biz_att_dirty").Ctx(ctx).Data(g.Map{
		"id": "dirty-fixture", "start_date": start, "end_date": end,
		"employee_ids": attJSON(employees), "status": "pending", "created_at": core.Now(),
	}).Insert()
	if e != nil {
		t.Fatal(e)
	}
	attProcessQueue(ctx)
	dirty, e := core.Model("biz_att_dirty").Ctx(ctx).Where("id", "dirty-fixture").One()
	if e != nil || dirty["status"].String() != "processed" {
		t.Fatalf("dirty task was not acknowledged: %v %v", dirty, e)
	}
	rows, e := core.Model("biz_att_batch").Ctx(ctx).Where("status", "queued").All()
	if e != nil {
		t.Fatal(e)
	}
	requests := []AttBatchRequest{}
	for _, row := range rows {
		var request AttBatchRequest
		if e := json.Unmarshal([]byte(row["request_json"].String()), &request); e != nil {
			t.Fatal(e)
		}
		if !request.Automatic || len(request.EmployeeIDs) == 0 {
			t.Fatalf("automatic task has an unsafe employee scope: %+v", request)
		}
		requests = append(requests, request)
	}
	return requests
}

func TestAttendanceDirtyUnrelatedRosterDoesNotQueueAffectedEmployees(t *testing.T) {
	for _, selection := range []string{"unassigned", "candidate", "filtered", "missing"} {
		t.Run(selection, func(t *testing.T) {
			ctx := setup(t)
			affected := attDirtyEmployee(t, ctx, "真实员工")
			qa := attDirtyEmployee(t, ctx, "独立测试员工")
			attDirtyRoster(t, ctx, qa, "2026-08-05", "confirmed")
			switch selection {
			case "candidate":
				attDirtyRoster(t, ctx, affected, "2026-08-05", "candidate")
			case "filtered":
				attDirtyRoster(t, ctx, affected, "2026-08-05", "confirmed")
				if _, e := core.Model("biz_hrm_employee").Ctx(ctx).Where("id", affected).Update(g.Map{"status": "filtered"}); e != nil {
					t.Fatal(e)
				}
			case "missing":
				affected = "unknown-employee"
			}
			requests := attDirtyRun(t, ctx, "2026-08-01", "2026-08-31", []string{affected})
			if len(requests) != 0 {
				t.Fatalf("unrelated QA roster triggered recalculation: %+v", requests)
			}
		})
	}
}

func TestAttendanceDirtyQueuesOnlyConfirmedAffectedEmployeeDates(t *testing.T) {
	ctx := setup(t)
	a := attDirtyEmployee(t, ctx, "受影响甲")
	b := attDirtyEmployee(t, ctx, "受影响乙")
	other := attDirtyEmployee(t, ctx, "其他人员")
	filtered := attDirtyEmployee(t, ctx, "已过滤人员")
	for _, day := range []string{"01", "02", "03", "05", "06"} {
		attDirtyRoster(t, ctx, a, "2026-08-"+day, "confirmed")
	}
	attDirtyRoster(t, ctx, a, "2026-08-04", "candidate")
	attDirtyRoster(t, ctx, b, "2026-08-03", "confirmed")
	attDirtyRoster(t, ctx, other, "2026-08-04", "confirmed")
	attDirtyRoster(t, ctx, filtered, "2026-08-04", "confirmed")
	if _, e := core.Model("biz_hrm_employee").Ctx(ctx).Where("id", filtered).Update(g.Map{"status": "filtered"}); e != nil {
		t.Fatal(e)
	}
	requests := attDirtyRun(t, ctx, "2026-08-01", "2026-08-07", []string{a, b, a, filtered, "unknown"})
	if len(requests) != 3 {
		t.Fatalf("expected three contiguous equal-scope batches, got %+v", requests)
	}
	actual := map[string]int{}
	for _, request := range requests {
		dates, e := attRange(request.StartDate, request.EndDate)
		if e != nil {
			t.Fatal(e)
		}
		for _, employee := range request.EmployeeIDs {
			for _, date := range dates {
				actual[employee+":"+date]++
			}
		}
	}
	want := map[string]int{b + ":2026-08-03": 1}
	for _, day := range []string{"01", "02", "03", "05", "06"} {
		want[a+":2026-08-"+day] = 1
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("automatic scope expanded across employees or unconfirmed dates: got %v want %v", actual, want)
	}
}

func TestAttendanceDirtyAllEmployeesStillRequiresConfirmedRostersAndSplitsLongRanges(t *testing.T) {
	ctx := setup(t)
	a := attDirtyEmployee(t, ctx, "已排班人员")
	_ = attDirtyEmployee(t, ctx, "未排班人员")
	dates, e := attRange("2026-07-01", "2026-09-05")
	if e != nil {
		t.Fatal(e)
	}
	for _, date := range dates {
		attDirtyRoster(t, ctx, a, date, "confirmed")
	}
	requests := attDirtyRun(t, ctx, dates[0], dates[len(dates)-1], nil)
	if len(requests) != 2 {
		t.Fatalf("expected 62-day split, got %+v", requests)
	}
	seen := map[string]bool{}
	for _, request := range requests {
		if !reflect.DeepEqual(request.EmployeeIDs, []string{a}) {
			t.Fatalf("unassigned employee leaked into all-employee invalidation: %+v", request)
		}
		part, e := attRange(request.StartDate, request.EndDate)
		if e != nil || len(part) > 62 {
			t.Fatalf("invalid batch size: %v %v", part, e)
		}
		for _, date := range part {
			if seen[date] {
				t.Fatal(fmt.Sprintf("date queued more than once: %s", date))
			}
			seen[date] = true
		}
	}
	if len(seen) != len(dates) {
		t.Fatalf("date coverage lost: got %d want %d", len(seen), len(dates))
	}
}
