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

func attWorkScheduleFixture(mode string) AttRule {
	a := attAutoFixtureRule()
	a.Config.Category = attWorkScheduleCategory(mode)
	a.Config.Cycle = AttCycle{} // The administrator does not configure a cycle.
	a.Config.WorkSchedule = &AttWorkSchedule{Mode: mode, WeekdayShiftIDs: []string{"day"}, RestdayShiftIDs: []string{"day"}}
	a.Config.Rule.Overtime = map[eng.Category]eng.OvertimePolicy{
		eng.Workday: {Enabled: true, RoundingMinutes: 30},
		eng.Restday: {Enabled: true, RoundingMinutes: 30, CompleteWorkdayMinutes: 480},
	}
	if mode != "production_employee" {
		sat := a.Config.Shifts[0]
		sat.ID, sat.Name = "saturday", "周六独立正班"
		a.Config.Shifts = append(a.Config.Shifts, sat)
		a.Config.WorkSchedule.SaturdayShiftIDs = []string{"saturday"}
	}
	return a
}

func TestWorkScheduleResolvesWeekWithoutCycleAndKeepsOverrides(t *testing.T) {
	for _, mode := range []string{"production_employee", "production_staff"} {
		t.Run(mode, func(t *testing.T) {
			a := attWorkScheduleFixture(mode)
			if err := validateAttConfig(a); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				date string
				day  int
			}{{"2026-12-28", 0}, {"2026-12-29", 1}, {"2026-12-30", 2}, {"2026-12-31", 3}, {"2027-01-01", 4}, {"2027-01-02", 5}, {"2027-01-03", 6}, {"2028-02-29", 1}} {
				day, err := attCycleDay(a.Config, tc.date)
				rest := tc.day == 6 || tc.day == 5 && mode == "production_employee"
				wantShift := "day"
				if tc.day == 5 && mode != "production_employee" {
					wantShift = "saturday"
				}
				if err != nil || day.ShiftID != wantShift || day.ScheduledWork == rest || (day.Category == eng.Restday) != rest || day.CandidateShiftIDs == nil || !reflect.DeepEqual(*day.CandidateShiftIDs, []string{wantShift}) {
					t.Fatalf("%s: %+v %v", tc.date, day, err)
				}
			}
			override := AttCalendarDay{Category: eng.Holiday, ShiftID: "day"}
			a.Config.Calendar = map[string]AttCalendarDay{"2027-01-02": override}
			if got, err := attCycleDay(a.Config, "2027-01-02"); err != nil || !reflect.DeepEqual(got, override) {
				t.Fatalf("explicit calendar lost: %+v %v", got, err)
			}
			if a.Config.Cycle.Mode != "" || len(a.Config.Cycle.Weekdays) != 0 {
				t.Fatal("date resolution mutated the original policy")
			}
		})
	}
}

func TestWorkScheduleRejectsMissingReferencesAndConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*AttRule)
		want string
	}{
		{"weekday", func(a *AttRule) { a.Config.WorkSchedule.WeekdayShiftIDs = nil }, "周一至周五"},
		{"saturday", func(a *AttRule) { a.Config.WorkSchedule.SaturdayShiftIDs = nil }, "周六正班"},
		{"restday", func(a *AttRule) { a.Config.WorkSchedule.RestdayShiftIDs = nil }, "休息日实际出勤"},
		{"unknown_shift", func(a *AttRule) { a.Config.WorkSchedule.SaturdayShiftIDs = []string{"missing"} }, "不存在"},
		{"duplicate_shift", func(a *AttRule) { a.Config.WorkSchedule.WeekdayShiftIDs = []string{"day", "day"} }, "重复"},
		{"wrong_category", func(a *AttRule) { a.Config.Category = "员工" }, "分类不一致"},
		{"legacy_evidence", func(a *AttRule) { a.Config.Rule.EvidenceMode = "" }, "本地打卡"},
		{"window", func(a *AttRule) { a.Config.Shifts[0].Segments[0].LocalWindows = nil }, "待配置"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := attWorkScheduleFixture("production_staff")
			tc.edit(&a)
			if err := validateAttConfig(a); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted %q, got %v", tc.want, err)
			}
		})
	}
}

func TestWorkScheduleSourcePreservesSaturdayAndDutyVariants(t *testing.T) {
	for _, source := range []ImportRuleDraft{
		{Category: "员工", ScheduleText: "8:00-12:00 13:30-17:30"},
		{Category: "生产职员", ScheduleText: "8:00-12:00 13:30-17:30 18:00-20:00", SaturdaySchedule: "8:00-12:00 13:30-17:30", Remarks: "计算加班2小时"},
		{Category: "生产职员", Department: "售后", ScheduleText: "8:30-12:00 13:30-18:00/20:00", SaturdaySchedule: "8:30-12:00 13:30-18:00/20:00", Remarks: "每天留守一人值班到20：00，其余人员18:00下班"},
	} {
		c, _ := localSourceConfig(source)
		p := c.WorkSchedule
		if p == nil || len(p.WeekdayShiftIDs) == 0 || len(p.RestdayShiftIDs) == 0 || c.Cycle.Mode != "weekly" || c.Cycle.Anchor != "" {
			t.Fatalf("source did not derive week: %+v", c)
		}
		sat, err := attCycleDay(c, "2026-10-10")
		if err != nil || (sat.Category == eng.Restday) != (source.Category == "员工") {
			t.Fatalf("wrong source Saturday: %+v %v", sat, err)
		}
		if source.Department == "售后" {
			if p.Mode != "after_sales" || len(p.WeekdayShiftIDs) != 2 || len(p.SaturdayShiftIDs) != 2 || c.Shifts[0].Segments[1].EndMinute != 1080 || c.Shifts[1].Segments[1].EndMinute != 1200 {
				t.Fatalf("duty candidates collapsed: %+v", c)
			}
		}
	}
	c, _ := localSourceConfig(ImportRuleDraft{Category: "生产职员", ScheduleText: "8:00-12:00 13:30-18:00"})
	sat, _ := attCycleDay(c, "2026-10-10")
	if sat.Category != eng.Workday || !sat.ScheduledWork || sat.ShiftID != "" || len(c.WorkSchedule.SaturdayShiftIDs) != 0 {
		t.Fatalf("missing staff Saturday must stay required and unconfigured: %+v", sat)
	}
}

func TestWorkScheduleHTTPSaveMaterializesDraftAndProtectsLegacyVersion(t *testing.T) {
	ctx := setup(t)
	c := newAttWorkflowClient(t, ctx)
	person := attDirtyEmployee(t, ctx, "固定工作周")
	old := attWeeklyTestRule()
	old.Scope = AttScope{EmployeeIDs: []string{person}}
	id := c.ok("POST", "rules", old)["id"].(string)
	c.ok("POST", "rules/"+id+"/publish", g.Map{})
	a := attWorkScheduleFixture("production_staff")
	a.ID, a.Scope = id, old.Scope
	a.Config.Rule.EvidenceMode = "" // New fixed-week drafts adopt local evidence.
	newID := c.ok("POST", "rules", a)["id"].(string)
	stored, err := attLoadRule(ctx, newID)
	if err != nil || newID == id || stored.Config.Cycle.Mode != "weekly" || len(stored.Config.Cycle.Weekdays) != 7 || stored.Config.Rule.EvidenceMode != "local" || stored.Status != "draft" {
		t.Fatalf("save did not persist the derived week: %+v %v", stored, err)
	}
	original, _ := attLoadRule(ctx, id)
	if original.Config.WorkSchedule != nil || original.Status != "published" {
		t.Fatal("saved strategy changed the previous published version")
	}
	// Incomplete drafts are editable; the server still blocks activation.
	stored.Config.WorkSchedule.SaturdayShiftIDs = nil
	c.ok("POST", "rules", stored)
	_, response := c.json(1, "POST", "rules/"+newID+"/publish", g.Map{})
	if response["code"] == float64(0) || !strings.Contains(response["message"].(string), "周六正班") {
		t.Fatalf("missing Saturday was activated: %v", response)
	}
}

func TestWorkScheduleAutomaticallyDerivesAndCalculatesWeekend(t *testing.T) {
	for _, mode := range []string{"production_employee", "production_staff"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.WithValue(setup(t), "uid", int64(1))
			person := attDirtyEmployee(t, ctx, "无需手动排班")
			a := attWorkScheduleFixture(mode)
			a.Status = "draft"
			a.Scope = AttScope{EmployeeIDs: []string{person}}
			attAutoStoreRule(t, ctx, a)
			for _, date := range []string{"2026-10-10", "2026-10-11"} {
				for _, p := range attAutoPunches(date) {
					_, err := UpsertClock(ctx, Clock{ID: date + p.ID, EmployeeID: person, ClockTime: timeString(p.At), ClockType: p.Type, Status: p.Status, Source: "manual"})
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			_, _, batches, err := ApplyAttendanceRule(ctx, a.ID, "2026-10-09", "2026-10-12", "确认固定工作周")
			if err != nil || len(batches) != 1 {
				t.Fatalf("activation did not queue calculation: %v %v", batches, err)
			}
			attProcessQueue(ctx)
			batch, _ := core.Model("biz_att_batch").Ctx(ctx).Where("id", batches[0]).One()
			if batch["status"].String() != "completed" {
				t.Fatalf("automatic batch failed: %v", batch)
			}
			for _, date := range []string{"2026-10-10", "2026-10-11"} {
				row, _ := core.Model("biz_att_result").Ctx(ctx).Where("batch_id", batches[0]).Where("work_date", date).One()
				var result eng.Result
				_ = json.Unmarshal([]byte(row["result_json"].String()), &result)
				wantOT := int64(480 * 60)
				if mode == "production_staff" && date == "2026-10-10" {
					wantOT = 0
				}
				if result.State != "calculated" || result.ActualSeconds != 480*60 || result.OvertimeSeconds != wantOT {
					t.Fatalf("%s %s: %+v", mode, date, result)
				}
				roster, _ := core.Model("biz_att_roster").Ctx(ctx).Where("employee_id", person).Where("work_date", date).One()
				if roster["source"].String() != "automatic" {
					t.Fatalf("no derived roster: %v", roster)
				}
			}
			// An ordinary required day without cards cannot be inferred as rest.
			missing, _ := core.Model("biz_att_result").Ctx(ctx).Where("batch_id", batches[0]).Where("work_date", "2026-10-09").One()
			var result eng.Result
			_ = json.Unmarshal([]byte(missing["result_json"].String()), &result)
			if result.State == "rest" || result.State == "calculated" {
				t.Fatalf("missing required attendance claimed normal: %+v", result)
			}
		})
	}
}
