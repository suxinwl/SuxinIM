package hrm

import (
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"testing"
)

func TestAttendanceProjectionRemovesDeselectedPunches(t *testing.T) {
	ctx := setup(t)
	employee, e := UpsertEmployee(ctx, Employee{Name: "投影测试", EmployeeNo: "PR001", DeptID: 10})
	if e != nil {
		t.Fatal(e)
	}
	clock, e := UpsertClock(ctx, Clock{EmployeeID: employee, ClockTime: "2026-08-03 08:00:00", ClockType: "in"})
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"old", "new"} {
		result := eng.Result{}
		if id == "old" {
			result.Segments = []eng.SegmentResult{{InPunchID: clock}}
		}
		_, e = core.Model("biz_att_result").Ctx(ctx).Data(g.Map{"id": id, "batch_id": id, "employee_id": employee, "work_date": "2026-08-03", "rule_id": "rule", "status": "review", "employee_json": "{}", "result_json": attJSON(result), "input_json": "{}", "created_at": core.Now()}).Insert()
		if e != nil {
			t.Fatal(e)
		}
	}
	if e = attProjectClockStates(ctx, "old", "trial"); e != nil {
		t.Fatal(e)
	}
	if n, _ := core.Model("biz_att_clock_state").Count(); n != 1 {
		t.Fatal("old projection missing")
	}
	if e = attProjectClockStates(ctx, "new", "trial"); e != nil {
		t.Fatal(e)
	}
	if n, _ := core.Model("biz_att_clock_state").Count(); n != 0 {
		t.Fatal("deselected punch retained stale local status")
	}
}
