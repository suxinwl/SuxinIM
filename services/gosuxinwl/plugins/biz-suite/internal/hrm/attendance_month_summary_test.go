package hrm

import (
	"testing"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
)

func TestLocalMonthSummarySeparatesActualCreditAndOvertime(t *testing.T) {
	person := AttEmployee{ID: "monthly-summary", Name: "同名员工", EmployeeNo: "0032", Department: "生产一部"}
	values := []eng.Result{
		{Date: "2026-10-12", State: "calculated", ExpectedSeconds: 465 * 60, StandardSeconds: 480 * 60, ActualSeconds: 11 * 3600, CreditedSeconds: 8 * 3600, OvertimeSeconds: 3 * 3600, OvertimeBuckets: []eng.OvertimeBucket{{Category: eng.Workday, Seconds: 3 * 3600}}},
		{Date: "2026-10-17", State: "calculated", StandardSeconds: 480 * 60, ActualSeconds: 11 * 3600, CreditedSeconds: 8 * 3600, OvertimeSeconds: 11 * 3600, OvertimeBuckets: []eng.OvertimeBucket{{Category: eng.Restday, Seconds: 11 * 3600}}},
		{Date: "2026-10-01", State: "calculated", ActualSeconds: 2 * 3600, OvertimeSeconds: 2 * 3600, OvertimeBuckets: []eng.OvertimeBucket{{Category: eng.Holiday, Seconds: 2 * 3600}}},
		{Date: "2026-10-18", State: "configuration_pending"},
	}
	rows := gdb.Result{}
	for _, value := range values {
		rows = append(rows, attendanceExportRecord(g.Map{"employee_json": attJSON(person), "result_json": attJSON(value)}))
	}
	items := attMonthItems(rows)
	if len(items) != 1 {
		t.Fatalf("unexpected employee aggregation: %+v", items)
	}
	item := items[0]
	if item.EmployeeNo != "0032" || item.Department != "生产一部" || item.Name != person.Name {
		t.Fatalf("historical identity was not preserved: %+v", item)
	}
	if item.ExpectedDays != 1 || item.ExpectedHours != 7.75 || item.Days[1].ExpectedHours != 0 {
		t.Fatalf("optional weekend attendance became required work: %+v", item)
	}
	if item.ActualHours != 24 || item.CreditedHours != 16 || item.OvertimeHours != 16 {
		t.Fatalf("actual hours must not be credited attendance plus overtime: %+v", item)
	}
	if item.OvertimeWorkdayHours != 3 || item.OvertimeRestdayHours != 11 || item.OvertimeHolidayHours != 2 {
		t.Fatalf("overtime categories were lost: %+v", item)
	}
	if item.Days[3].State != "pending" || item.Days[3].Worked {
		t.Fatalf("unconfigured day was presented as attendance: %+v", item.Days[3])
	}
}
