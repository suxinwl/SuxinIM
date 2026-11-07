package hrm

import (
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"testing"
)

func TestTripMonthlyHoursAndRevocation(t *testing.T) {
	ctx := setup(t)
	id, e := UpsertEmployee(ctx, Employee{Name: "Travel", EntryDate: "2026-01-05", QuitDate: "2026-01-05"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = core.Model("biz_hrm_attendance_group").Data(g.Map{"id": "trip-group", "name": "trip", "dept_ids": "[]", "employee_ids": "[\"" + id + "\"]", "shifts": "[{\"start\":\"09:00\",\"end\":\"18:00\",\"break_start\":\"12:00\",\"break_end\":\"13:00\",\"weekdays\":[1]}]", "rule_json": "{}", "created_at": core.Now(), "updated_at": core.Now()}).Insert()
	if e != nil {
		t.Fatal(e)
	}
	in := Out{EmployeeID: id, Source: "wecom", ExternalRef: "trip-1", ApprovalStatus: "approved", StartTime: "2026-01-05 09:00:00", EndTime: "2026-01-05 18:00:00", Duration: 8}
	if _, e = UpsertTrip(ctx, in); e != nil {
		t.Fatal(e)
	}
	month, e := Month(ctx, "2026-01", id)
	if e != nil {
		t.Fatal(e)
	}
	row := month.Items[0]
	if row.TripHours != 8 || row.OutHours != 0 || row.MissingCount != 0 || row.AbsentDays != 0 || row.WorkedDays != 1 {
		t.Fatal(row)
	}
	if _, e = UpsertLeave(ctx, Leave{EmployeeID: id, Source: "manual", ApprovalStatus: "approved", StartTime: "2026-01-05 09:00:00", EndTime: "2026-01-05 11:00:00", Duration: 2}); e != nil {
		t.Fatal(e)
	}
	month, e = Month(ctx, "2026-01", id)
	if e != nil {
		t.Fatal(e)
	}
	if month.Items[0].TripHours != 6 || month.Items[0].LeaveHours != 2 {
		t.Fatal(month.Items[0])
	}
	in.ApprovalStatus = "canceled"
	if _, e = UpsertTrip(ctx, in); e != nil {
		t.Fatal(e)
	}
	month, e = Month(ctx, "2026-01", id)
	if e != nil {
		t.Fatal(e)
	}
	if month.Items[0].TripHours != 0 || month.Items[0].MissingCount != 2 || month.Items[0].AbsentDays != 1 {
		t.Fatal(month.Items[0])
	}
}
