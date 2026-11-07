package hrm

import (
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func TestAttendanceReviewLocksAdjacentManualEvidenceButRetainsWeComEvidence(t *testing.T) {
	for _, date := range []string{"2026-09-01", "2026-07-31"} {
		t.Run(date, func(t *testing.T) {
			ctx := setup(t)
			employee := ledgerEmployee(t, ctx)
			in := Clock{EmployeeID: employee, ClockTime: date + " 06:00:00", ClockType: "out", Source: "manual"}
			id, e := UpsertClock(ctx, in)
			if e != nil {
				t.Fatal(e)
			}
			// August closes a night shift ending Sep 1 and can depend on prior
			// July 31 overtime for an Aug 1 late-arrival entitlement.
			ledgerMonth(t, ctx, "closed-batch", "locked")
			in.ID, in.ClockTime = id, date+" 06:10:00"
			if _, e = UpsertClock(ctx, in); e == nil {
				t.Fatal("manual edit crossed the locked work-month boundary")
			}
			old, e := core.Model("biz_hrm_attendance_clock").Ctx(ctx).Where("id", id).One()
			if e != nil || !strings.Contains(old["clock_time"].String(), "06:00:00") {
				t.Fatal("rejected edit changed the original clock", old, e)
			}
			in.ID = ""
			if _, e = UpsertClock(ctx, in); e == nil {
				t.Fatal("new manual evidence changed a locked adjacent work date")
			}
			in.Source, in.ExternalRef = "wecom", "source-after-close"
			if _, e = UpsertClock(ctx, in); e != nil {
				t.Fatal("upstream evidence must still be retainable after month close", e)
			}
		})
	}
}

func TestAttendanceReviewRejectsUnknownPublishedCategoryAndExportOmission(t *testing.T) {
	ctx := setup(t)
	c := newAttWorkflowClient(t, ctx)
	a := attWorkflowRule()
	a.Config.Category = "未知分类"
	id := c.ok("POST", "rules", a)["id"].(string)
	c.rejected("POST", "rules/"+id+"/publish", g.Map{})
	employee := ledgerEmployee(t, ctx)
	person := AttEmployee{ID: employee, Name: "不得遗漏的员工", EmployeeNo: "CAT001", Category: "未知分类"}
	_, e := core.Model("biz_att_result").Ctx(ctx).Data(g.Map{"id": "category-result", "batch_id": "category-batch", "employee_id": employee, "work_date": "2026-08-03", "rule_id": "rule", "status": "rest", "employee_json": attJSON(person), "result_json": attJSON(eng.Result{EmployeeID: employee, Date: "2026-08-03", State: "rest"}), "input_json": "{}", "created_at": core.Now()}).Insert()
	if e != nil {
		t.Fatal(e)
	}
	rows, e := core.Model("biz_att_result").Ctx(ctx).Where("id", "category-result").All()
	if e != nil {
		t.Fatal(e)
	}
	if f, e := ExportAttendanceResults(rows, "2026-08-03", "2026-08-03"); e == nil {
		_ = f.Close()
		t.Fatal("unknown category silently omitted employee from a successful workbook")
	}
	// Old rows with no category keep their documented default and remain visible.
	person.Category = ""
	if _, e = core.Model("biz_att_result").Ctx(ctx).Where("id", "category-result").Update(g.Map{"employee_json": attJSON(person)}); e != nil {
		t.Fatal(e)
	}
	rows, e = core.Model("biz_att_result").Ctx(ctx).Where("id", "category-result").All()
	if e != nil {
		t.Fatal(e)
	}
	f, e := ExportAttendanceResults(rows, "2026-08-03", "2026-08-03")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	name, e := f.GetCellValue("职员", "A3")
	if e != nil || name != person.Name {
		t.Fatal("legacy blank category disappeared", name, e)
	}
}
