package hrm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func attWeeklyTestRule() AttRule {
	a := attWorkflowRule()
	a.Name = "按星期排班制度"
	a.Config.Cycle = AttCycle{Mode: "weekly"}
	for i := 0; i < 7; i++ {
		day := AttCalendarDay{ShiftID: "day", Category: eng.Workday, ScheduledWork: true}
		if i >= 5 {
			day.Category, day.ScheduledWork = eng.Restday, false
		}
		a.Config.Cycle.Weekdays = append(a.Config.Cycle.Weekdays, day)
	}
	// Both weekdays and optional weekend overtime use this same conditional shift.
	a.Config.Shifts[0].OvertimeVariant = &eng.ShiftVariant{TriggerMinute: 1080, Shift: eng.Shift{
		ID: "evening", Name: "有晚间加班", Segments: []eng.Segment{
			{ID: "work", Kind: "work", StartMinute: 540, EndMinute: 1080, PunchMode: "both"},
			{ID: "ot", Kind: "overtime", StartMinute: 1080, EndMinute: 1800, PunchMode: "both", OpenEnded: true},
		},
	}}
	return a
}

func TestAttendanceWeeklyDateResolutionAndLegacyCycles(t *testing.T) {
	weekly := attWeeklyTestRule().Config
	for i := range weekly.Cycle.Weekdays {
		weekly.Cycle.Weekdays[i].ShiftID = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}[i]
	}
	for _, tc := range []struct{ date, shift string }{
		{"2026-12-28", "mon"}, {"2026-12-29", "tue"}, {"2026-12-30", "wed"},
		{"2026-12-31", "thu"}, {"2027-01-01", "fri"}, {"2027-01-02", "sat"},
		{"2027-01-03", "sun"}, {"2027-01-04", "mon"},
		{"2028-02-28", "mon"}, {"2028-02-29", "tue"}, {"2028-03-01", "wed"},
	} {
		day, e := attCycleDay(weekly, tc.date)
		if e != nil || day.ShiftID != tc.shift {
			t.Fatalf("weekly %s: %+v %v, want %s", tc.date, day, e, tc.shift)
		}
		weekend := tc.shift == "sat" || tc.shift == "sun"
		if weekend != (day.Category == eng.Restday) || day.ScheduledWork == weekend {
			t.Fatalf("weekly category/required attendance changed: %s %+v", tc.date, day)
		}
	}
	override := AttCalendarDay{ShiftID: "override", Category: eng.Holiday, ScheduledWork: true}
	weekly.Calendar = map[string]AttCalendarDay{"2027-01-03": override}
	if day, e := attCycleDay(weekly, "2027-01-03"); e != nil || day != override {
		t.Fatalf("explicit calendar must precede weekdays: %+v %v", day, e)
	}
	if _, e := attCycleDay(weekly, "2027-02-30"); e == nil {
		t.Fatal("invalid weekly date accepted")
	}
	weekly.Cycle.Weekdays = weekly.Cycle.Weekdays[:6]
	if _, e := attCycleDay(weekly, "2027-01-04"); e == nil {
		t.Fatal("incomplete week accepted")
	}
	// Legacy mode remains anchored, including a 14-day rotation and dates
	// before its anchor. It does not become a weekday schedule implicitly.
	for _, mode := range []string{"", "cycle"} {
		legacy := AttConfig{Cycle: AttCycle{Mode: mode, Anchor: "2027-01-04", Days: []string{
			"day", "day", "day", "day", "day", "", "", "night", "night", "night", "night", "night", "", "",
		}}}
		for _, tc := range []struct{ date, shift string }{
			{"2027-01-03", ""}, {"2027-01-04", "day"}, {"2027-01-11", "night"}, {"2027-01-18", "day"},
		} {
			day, e := attCycleDay(legacy, tc.date)
			if e != nil || day.ShiftID != tc.shift || day.ScheduledWork {
				t.Fatalf("legacy %q %s changed: %+v %v", mode, tc.date, day, e)
			}
		}
	}
}

func TestAttendanceWeeklyHTTPPublishValidation(t *testing.T) {
	ctx := setup(t)
	attDirtyEmployee(t, ctx, "按星期制度员工")
	c := newAttWorkflowClient(t, ctx)
	for _, tc := range []struct {
		name string
		edit func(*AttRule)
		want string
	}{
		{"unknown_mode", func(a *AttRule) { a.Config.Cycle.Mode = "other" }, "排班模式无效"},
		{"missing_week", func(a *AttRule) { a.Config.Cycle.Weekdays = nil }, "周一至周日7天"},
		{"extra_day", func(a *AttRule) { a.Config.Cycle.Weekdays = append(a.Config.Cycle.Weekdays, AttCalendarDay{}) }, "周一至周日7天"},
		{"unknown_category", func(a *AttRule) { a.Config.Cycle.Weekdays[0].Category = "other" }, "周一的日期类型无效"},
		{"missing_category", func(a *AttRule) { a.Config.Cycle.Weekdays[1].Category = "" }, "周二的日期类型无效"},
		{"missing_shift", func(a *AttRule) { a.Config.Cycle.Weekdays[0].ShiftID = "absent" }, "周一引用了不存在的班次"},
		{"workday_without_shift", func(a *AttRule) { a.Config.Cycle.Weekdays[0].ShiftID = "" }, "周一要求出勤"},
		{"required_restday_without_shift", func(a *AttRule) {
			a.Config.Cycle.Weekdays[5].ShiftID = ""
			a.Config.Cycle.Weekdays[5].ScheduledWork = true
		}, "周六要求出勤"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := attWeeklyTestRule()
			tc.edit(&a)
			id := c.ok("POST", "rules", a)["id"].(string)
			status, body := c.json(1, "POST", "rules/"+id+"/publish", attWeeklyTestRule())
			message, _ := body["message"].(string)
			if status != 200 || body["code"] == float64(0) || !strings.Contains(message, tc.want) {
				t.Fatalf("expected %q: status=%d response=%v", tc.want, status, body)
			}
			stored, e := attLoadRule(ctx, id)
			if e != nil || stored.Status != "draft" {
				t.Fatalf("invalid weekly draft published: %+v %v", stored, e)
			}
		})
	}
	// A rest day may have no template, or retain a conditional shift as an
	// optional template; neither needs an anchor or legacy cycle days.
	a := attWeeklyTestRule()
	a.Config.Cycle.Weekdays[6].ShiftID = ""
	id := c.ok("POST", "rules", a)["id"].(string)
	c.ok("POST", "rules/"+id+"/publish", g.Map{})
}

func TestAttendanceWeeklyHTTPGenerateAcrossYearWithOverrides(t *testing.T) {
	ctx := setup(t)
	c := newAttWorkflowClient(t, ctx)
	employee, e := UpsertEmployee(ctx, Employee{Name: "星期排班员工", EmployeeNo: "WEEK001", DeptID: 10})
	if e != nil {
		t.Fatal(e)
	}
	legacy := attWorkflowRule()
	legacy.Config.Cycle = AttCycle{Mode: "cycle", Anchor: "2026-12-28", Days: []string{"day"}}
	legacyID := c.ok("POST", "rules", legacy)["id"].(string)
	c.ok("POST", "rules/"+legacyID+"/publish", g.Map{})
	c.ok("POST", "rosters/generate", g.Map{"start_date": "2026-12-30", "end_date": "2026-12-31", "employee_ids": []string{employee}})
	c.ok("POST", "rosters", g.Map{"items": []AttRoster{{EmployeeID: employee, WorkDate: "2026-12-31", RuleID: legacyID, Category: eng.Restday, Source: "manual"}}, "reason": "保留人工调休"})

	a := attWeeklyTestRule()
	a.ID = legacyID // Saves a new version; never mutates the published rule.
	a.Config.Calendar = map[string]AttCalendarDay{"2026-12-31": {ShiftID: "day", Category: eng.Holiday}}
	weeklyID := c.ok("POST", "rules", a)["id"].(string)
	if weeklyID == legacyID {
		t.Fatal("weekly edit overwrote published legacy rule")
	}
	c.ok("POST", "rules/"+weeklyID+"/publish", g.Map{})
	stored, e := attLoadRule(ctx, weeklyID)
	if e != nil || stored.Config.Cycle.Mode != "weekly" || stored.Config.Cycle.Anchor != "" || len(stored.Config.Cycle.Days) != 0 || len(stored.Config.Cycle.Weekdays) != 7 || stored.Config.Shifts[0].OvertimeVariant == nil {
		t.Fatalf("weekly/conditional payload did not persist: %+v %v", stored, e)
	}
	old, e := attLoadRule(ctx, legacyID)
	if e != nil || old.Config.Cycle.Mode != "cycle" || old.Status != "published" {
		t.Fatalf("legacy publication changed: %+v %v", old, e)
	}
	for _, h := range []struct {
		date string
		work int
	}{{"2026-12-31", 1}, {"2027-01-01", 0}, {"2027-01-02", 1}} {
		_, e = core.Model("biz_hrm_attendance_holiday").Ctx(ctx).Data(g.Map{
			"id": "weekly-" + h.date, "holiday_date": h.date, "name": "测试节假日", "is_workday": h.work,
			"remark": "", "created_at": core.Now(), "updated_at": core.Now(),
		}).Insert()
		if e != nil {
			t.Fatal(e)
		}
	}
	request := g.Map{"start_date": "2026-12-28", "end_date": "2027-01-04", "employee_ids": []string{employee}, "preview": true}
	preview := c.ok("POST", "rosters/generate", request)
	if preview["saved"] != false || len(preview["issues"].([]any)) != 0 || len(preview["items"].([]any)) != 8 {
		t.Fatalf("weekly preview failed: %v", preview)
	}
	n, e := core.Model("biz_att_roster").Ctx(ctx).Where("employee_id", employee).Count()
	if e != nil || n != 2 {
		t.Fatalf("preview wrote rosters: %d %v", n, e)
	}
	for _, raw := range preview["items"].([]any) {
		b, _ := json.Marshal(raw)
		var day AttRoster
		if e := json.Unmarshal(b, &day); e != nil {
			t.Fatal(e)
		}
		category, scheduled := eng.Workday, true
		switch day.WorkDate {
		case "2026-12-31", "2027-01-01":
			category, scheduled = eng.Holiday, false
		case "2027-01-03":
			category, scheduled = eng.Restday, false
		}
		if day.Category != category || day.ScheduledWork != scheduled || day.ShiftID != "day" || day.Source != "weekly" || day.RuleID != weeklyID {
			t.Fatalf("weekly/holiday/calendar result wrong: %+v", day)
		}
	}
	request["preview"] = false
	saved := c.ok("POST", "rosters/generate", request)
	if saved["saved"] != true || len(saved["issues"].([]any)) != 0 {
		t.Fatalf("weekly generate failed: %v", saved)
	}
	rows, e := core.Model("biz_att_roster").Ctx(ctx).Where("employee_id", employee).All()
	if e != nil || len(rows) != 8 {
		t.Fatalf("wrong saved weekly roster count: %d %v", len(rows), e)
	}
	for _, row := range rows {
		day := attDecodeRoster(row)
		if day.WorkDate == "2026-12-31" {
			if day.Source != "manual" || day.Category != eng.Restday || day.RuleID != legacyID || day.ShiftID != "" {
				t.Fatalf("weekly generation overwrote manual assignment: %+v", day)
			}
		} else if day.Source != "weekly" || day.RuleID != weeklyID {
			t.Fatalf("weekly did not update an automatically generated roster: %+v", day)
		}
	}
}
