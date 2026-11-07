package hrm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func attAutoPointer(n int) *int { return &n }
func attAutoWindows(a, b, c, d int) *eng.LocalPunchWindows {
	return &eng.LocalPunchWindows{InStartSecond: attAutoPointer(a), InEndSecond: attAutoPointer(b), OutStartSecond: attAutoPointer(c), OutEndSecond: attAutoPointer(d)}
}

func TestLocalNoPublishedRuleInvalidatesOnlyDerivedRosters(t *testing.T) {
	ctx := setup(t)
	person := attDirtyEmployee(t, ctx, "归档制度")
	a := attAutoFixtureRule()
	a.Status = "published"
	attAutoStoreRule(t, ctx, a)
	for i, source := range []string{"automatic", "weekly", "cycle", "manual"} {
		if err := attPutRosters(ctx, []AttRoster{{EmployeeID: person, WorkDate: time.Date(2026, 10, 12+i, 0, 0, 0, 0, core.Location).Format("2006-01-02"), RuleID: a.ID, ShiftID: "day", Category: eng.Workday, Source: source}}, "原有排班"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := core.Model("biz_att_rule").Ctx(ctx).Where("id", a.ID).Update(g.Map{"status": "archived"}); err != nil {
		t.Fatal(err)
	}
	if err := attEnsureDerivedRosters(ctx, AttBatchRequest{StartDate: "2026-10-12", EndDate: "2026-10-15", EmployeeIDs: []string{person}}); err != nil {
		t.Fatal(err)
	}
	rows, err := core.Model("biz_att_roster").Ctx(ctx).Where("employee_id", person).All()
	if err != nil || len(rows) != 1 || rows[0]["source"].String() != "manual" {
		t.Fatalf("obsolete automatic roster retained or manual removed: %v %v", rows, err)
	}
}

func TestLocalApplyQueuesExactBoundaryDatesAndProtectsLockedNeighbors(t *testing.T) {
	for _, locked := range []bool{false, true} {
		name := "unlocked"
		if locked {
			name = "locked"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.WithValue(setup(t), "uid", int64(1))
			person := attDirtyEmployee(t, ctx, "边界核算")
			old := attAutoFixtureRule()
			old.Status, old.EffectiveFrom, old.Scope = "published", "2026-09-01", AttScope{EmployeeIDs: []string{person}}
			attAutoStoreRule(t, ctx, old)
			next := old
			next.ID, next.Version, next.Status, next.EffectiveFrom, next.EffectiveTo = "boundary-version", 2, "draft", "2026-10-01", "2026-10-31"
			attAutoStoreRule(t, ctx, next)
			if locked {
				for _, date := range []string{"2026-09-30", "2026-11-01"} {
					month := date[:7]
					if _, err := core.Model("biz_att_month").Ctx(ctx).Data(g.Map{"month": month, "status": "locked", "revision": 3, "batch_id": "frozen-" + month, "updated_by": 1, "updated_at": core.Now()}).Insert(); err != nil {
						t.Fatal(err)
					}
					if _, err := core.Model("biz_att_result").Ctx(ctx).Data(g.Map{"id": "frozen-" + date, "batch_id": "frozen-" + month, "employee_id": person, "work_date": date, "rule_id": old.ID, "status": "calculated", "employee_json": "{}", "result_json": `{"state":"calculated","credited_seconds":28800}`, "input_json": "{}", "created_at": core.Now()}).Insert(); err != nil {
						t.Fatal(err)
					}
				}
			}
			_, _, ids, err := ApplyAttendanceRule(ctx, next.ID, "2026-10-01", "2026-10-31", "十月制度修订覆盖跨月关联日期")
			if err != nil || len(ids) != 1 {
				t.Fatalf("main month batch changed: %v %v", ids, err)
			}
			main, err := core.Model("biz_att_batch").Ctx(ctx).Where("id", ids[0]).One()
			if err != nil || main["start_date"].String() != "2026-10-01" || main["end_date"].String() != "2026-10-31" {
				t.Fatalf("monthly closing batch lost full range: %v %v", main, err)
			}
			dirty, err := core.Model("biz_att_dirty").Ctx(ctx).OrderAsc("start_date").All()
			if err != nil || len(dirty) != 2 {
				t.Fatalf("expected two exact incremental boundaries: %v %v", dirty, err)
			}
			for i, want := range []string{"2026-09-30", "2026-11-01"} {
				var members []string
				_ = json.Unmarshal([]byte(dirty[i]["employee_ids"].String()), &members)
				if dirty[i]["start_date"].String() != want || dirty[i]["end_date"].String() != want || len(members) != 1 || members[0] != person {
					t.Fatalf("boundary range/scope expanded: %v", dirty[i])
				}
				if err = attQueueDirty(ctx, dirty[i]); err != nil {
					t.Fatal(err)
				}
			}
			batches, err := core.Model("biz_att_batch").Ctx(ctx).Where("id != ?", ids[0]).OrderAsc("start_date").All()
			if err != nil {
				t.Fatal(err)
			}
			if locked {
				if len(batches) != 0 {
					t.Fatalf("locked boundary was queued for calculation: %v", batches)
				}
				for _, date := range []string{"2026-09-30", "2026-11-01"} {
					month, err := core.Model("biz_att_month").Ctx(ctx).Where("month", date[:7]).One()
					if err != nil || month["status"].String() != "locked" || month["revision"].Int() != 3 || month["change_pending"].Int() != 1 {
						t.Fatalf("locked month was changed instead of flagged: %v %v", month, err)
					}
					result, err := core.Model("biz_att_result").Ctx(ctx).Where("id", "frozen-"+date).One()
					if err != nil || result["result_json"].String() != `{"state":"calculated","credited_seconds":28800}` {
						t.Fatalf("frozen result was overwritten: %v %v", result, err)
					}
				}
			} else {
				if len(batches) != 2 {
					t.Fatalf("unlocked boundaries were not recalculated: %v", batches)
				}
				for i, want := range []string{"2026-09-30", "2026-11-01"} {
					if batches[i]["start_date"].String() != want || batches[i]["end_date"].String() != want {
						t.Fatalf("incremental boundary range changed: %v", batches[i])
					}
				}
			}
		})
	}
}

func TestLocalHistoricalDepartmentSnapshotSurvivesTransfer(t *testing.T) {
	ctx := context.WithValue(setup(t), "uid", int64(1))
	person := attDirtyEmployee(t, ctx, "历史部门")
	a := attAutoFixtureRule()
	a.Status = "published"
	a.Scope = AttScope{DepartmentIDs: []int64{10}}
	attAutoStoreRule(t, ctx, a)
	if err := attPutRosters(ctx, []AttRoster{{EmployeeID: person, WorkDate: "2026-10-12", RuleID: a.ID, ShiftID: "day", Category: eng.Workday, Source: "automatic"}}, "调岗前归属"); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Model("biz_hrm_employee").Ctx(ctx).Where("id", person).Update(g.Map{"dept_id": 20}); err != nil {
		t.Fatal(err)
	}
	next := a
	next.ID = "historical-version"
	next.Version = 2
	next.Status = "draft"
	attAutoStoreRule(t, ctx, next)
	p, err := PreviewAttendanceRule(ctx, next.ID, "2026-10-12", "2026-10-12")
	if err != nil || !p.CanApply || p.EmployeeCount != 1 {
		t.Fatalf("historical scope lost after transfer: %+v %v", p, err)
	}
	if _, _, _, err = ApplyAttendanceRule(ctx, next.ID, "2026-10-12", "2026-10-12", "历史制度修订"); err != nil {
		t.Fatal(err)
	}
	if err = attEnsureDerivedRosters(ctx, AttBatchRequest{StartDate: "2026-10-12", EndDate: "2026-10-12", EmployeeIDs: []string{person}}); err != nil {
		t.Fatal(err)
	}
	row, err := core.Model("biz_att_roster").Ctx(ctx).Where("employee_id", person).Where("work_date", "2026-10-12").One()
	r := attDecodeRoster(row)
	if err != nil || r.Employee.DeptID != 10 || r.RuleID != next.ID {
		t.Fatalf("current department overwrote historic snapshot: %+v %v", r, err)
	}
}

func TestLocalLateDepartureAutoCandidateUsesVerifiedPreviousOut(t *testing.T) {
	ctx := setup(t)
	person := attDirtyEmployee(t, ctx, "晚归自动识别")
	a := attAutoFixtureRule()
	a.Status = "published"
	a.Config.ShiftSelection = "auto"
	a.Config.Rule.LateDepartureAfterSecond = attAutoPointer(21*3600 + 30*60)
	a.Config.Rule.LateDepartureDelayMinutes = 60
	a.Config.Shifts = []eng.Shift{{ID: "day", Segments: []eng.Segment{{ID: "day", Kind: "work", StartMinute: 480, EndMinute: 1320, PunchMode: "both", LocalWindows: attAutoWindows(7*3600, 12*3600, 17*3600, 23*3600)}}}}
	attAutoStoreRule(t, ctx, a)
	for _, value := range []string{"2026-10-11 08:00:00", "2026-10-11 22:00:00", "2026-10-12 09:00:00", "2026-10-12 22:00:00"} {
		if _, err := UpsertClock(ctx, Clock{ID: value, EmployeeID: person, ClockTime: value, ClockType: "outside", Source: "manual", Status: "normal"}); err != nil {
			t.Fatal(err)
		}
	}
	id, err := attQueueBatch(ctx, AttBatchRequest{StartDate: "2026-10-12", EndDate: "2026-10-12", EmployeeIDs: []string{person}})
	if err != nil {
		t.Fatal(err)
	}
	attProcessQueue(ctx)
	row, err := core.Model("biz_att_result").Ctx(ctx).Where("batch_id", id).One()
	if err != nil {
		t.Fatal(err)
	}
	var result eng.Result
	_ = json.Unmarshal([]byte(row["result_json"].String()), &result)
	if result.State != "calculated" || result.LateCount != 0 || result.LateArrivalMinutes != 60 {
		t.Fatalf("verified prior out failed automatic candidate/bootstrap: %+v", result)
	}
}

func TestLocalPublicationRequiresMatchedPeopleAndRejectsRetiredSources(t *testing.T) {
	ctx := context.WithValue(setup(t), "uid", int64(1))
	c := newAttWorkflowClient(t, ctx)
	a := attAutoFixtureRule()
	a.Status = "draft"
	a.Scope = AttScope{EmployeeIDs: []string{"unknown"}}
	attAutoStoreRule(t, ctx, a)
	for _, start := range []string{"2026-10-12", "2099-01-01"} {
		p, err := PreviewAttendanceRule(ctx, a.ID, start, "2026-10-12")
		if err != nil || p.CanApply || p.EmployeeCount != 0 || len(p.Issues) == 0 {
			t.Fatalf("unmatched scope accepted: %+v %v", p, err)
		}
		if _, _, _, err = ApplyAttendanceRule(ctx, a.ID, start, "2026-10-12", "范围验证"); err == nil || !strings.Contains(err.Error(), "未匹配任何有效人员") {
			t.Fatalf("lost actionable zero-scope error: %v", err)
		}
	}
	attDirtyEmployee(t, ctx, "有效来源人员")
	a.Scope = AttScope{IsDefault: true}
	for _, status := range []string{"archived", "superseded", "draft"} {
		a.ID = "retired-" + status
		a.FamilyID = a.ID
		a.Status = status
		attAutoStoreRule(t, ctx, a)
		if status == "draft" {
			if _, err := core.Model("biz_att_rule").Ctx(ctx).Where("id", a.ID).Update(g.Map{"source_json": `{"snapshot_id":"old-wecom"}`}); err != nil {
				t.Fatal(err)
			}
		}
		c.rejected("POST", "rules", a)
		c.rejected("POST", "rules/"+a.ID+"/publish", g.Map{})
		c.rejected("POST", "rules/"+a.ID+"/apply", g.Map{})
		copy := a
		copy.ID, copy.Status, copy.Source = "", "draft", json.RawMessage(`{}`)
		c.rejected("POST", "rules", copy)
	}
}

func TestLocalClockFallbackShowsOnlyPendingStatesAndFiltersThem(t *testing.T) {
	ctx := setup(t)
	person := attDirtyEmployee(t, ctx, "待配置原始卡")
	if _, err := UpsertClock(ctx, Clock{ID: "unassigned", EmployeeID: person, ClockTime: "2026-10-12 08:00:00", ClockType: "in", Status: "normal", Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	directory, err := loadClockDirectory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i, tc := range []struct{ state, want string }{{"configuration_pending", "configuration_pending"}, {"pending", "in_progress"}, {"calculated", "uncomputed"}} {
		id := tc.state
		if _, err := core.Model("biz_att_batch").Ctx(ctx).Data(g.Map{"id": id, "status": "completed", "start_date": "2026-10-12", "end_date": "2026-10-12", "request_json": attJSON(AttBatchRequest{QueuedAtUnixNano: int64(i + 1)}), "created_by": 1, "created_at": core.Now()}).Insert(); err != nil {
			t.Fatal(err)
		}
		if _, err := core.Model("biz_att_result").Ctx(ctx).Data(g.Map{"id": id, "batch_id": id, "employee_id": person, "work_date": "2026-10-12", "rule_id": "", "status": tc.state, "employee_json": "{}", "result_json": "{}", "input_json": "{}", "created_at": core.Now()}).Insert(); err != nil {
			t.Fatal(err)
		}
		filter, err := parseClockFilter(map[string]string{"employee_id": person, "date": "2026-10-12", "basis": "local", "status": tc.want})
		if err != nil {
			t.Fatal(err)
		}
		rows, total, err := queryClocks(ctx, filter, directory)
		if err != nil || total != 1 || len(rows) != 1 || rows[0]["local_status"] != tc.want {
			t.Fatalf("pending fallback %s: %v %d %v", tc.state, rows, total, err)
		}
		if tc.want != "uncomputed" && rows[0]["local_basis"] != "unassigned" {
			t.Fatalf("pending fallback falsely claims selected evidence: %v", rows)
		}
	}
}

func TestLocalLegacyMonthReturnsTrialOrUncomputedWithoutOldAlgorithm(t *testing.T) {
	ctx := setup(t)
	person := attDirtyEmployee(t, ctx, "旧月报安全")
	if _, err := UpsertClock(ctx, Clock{EmployeeID: person, ClockTime: "2026-10-12 08:00:00", ClockType: "in", Status: "normal", Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	got, err := attLocalTrialMonth(ctx, "2026-10", person)
	if err != nil {
		t.Fatal(err)
	}
	items := got["items"].([]MonthEmployee)
	if got["basis"] != "local_trial" || got["status"] != "uncomputed" || len(items) != 1 || items[0].WorkedDays != 0 || len(items[0].Days) != 31 {
		t.Fatalf("legacy raw card inferred attendance: %v", got)
	}
	for _, day := range items[0].Days {
		if day.State != "uncomputed" {
			t.Fatal("uncomputed day presented as attendance")
		}
	}
}
func attAutoFixtureRule() AttRule {
	a := attWorkflowRule()
	a.ID, a.FamilyID, a.Version = "local-rule", "local-family", 1
	a.EffectiveFrom = "2026-10-01"
	a.Config.Rule.EvidenceMode = "local"
	a.Config.Rule.StandardMinutes = 480
	a.Config.Shifts = []eng.Shift{{ID: "day", Name: "白班", Segments: []eng.Segment{
		{ID: "morning", Kind: "work", StartMinute: 480, EndMinute: 720, PunchMode: "both", LocalWindows: attAutoWindows(7*3600, 10*3600, 11*3600, 12*3600+40*60)},
		{ID: "afternoon", Kind: "work", StartMinute: 810, EndMinute: 1050, PunchMode: "both", LocalWindows: attAutoWindows(12*3600+40*60, 16*3600, 17*3600, 20*3600)},
	}}}
	a.Config.Cycle = AttCycle{Mode: "weekly", Weekdays: make([]AttCalendarDay, 7)}
	for i := range a.Config.Cycle.Weekdays {
		a.Config.Cycle.Weekdays[i] = AttCalendarDay{ShiftID: "day", Category: eng.Workday, ScheduledWork: true}
	}
	return a
}
func attAutoStoreRule(t *testing.T, ctx context.Context, a AttRule) {
	t.Helper()
	_, err := core.Model("biz_att_rule").Ctx(ctx).Data(g.Map{"id": a.ID, "family_id": a.FamilyID, "version": a.Version, "name": a.Name, "status": a.Status, "effective_from": a.EffectiveFrom, "effective_to": a.EffectiveTo, "config_json": attJSON(a.Config), "scope_json": attJSON(a.Scope), "conflicts_json": attJSON(a.Conflicts), "source_json": "{}", "created_by": 1, "created_at": core.Now(), "updated_at": core.Now()}).Insert()
	if err != nil {
		t.Fatal(err)
	}
}
func attAutoPunches(date string) []eng.Punch {
	return []eng.Punch{{ID: "morning-in", At: tm(date + " 08:00:00").Unix(), Type: "out", Status: "late"}, {ID: "morning-out", At: tm(date + " 12:00:00").Unix(), Type: "in", Status: "early"}, {ID: "afternoon-in", At: tm(date + " 12:40:00").Unix(), Type: "out", Status: "late"}, {ID: "afternoon-out", At: tm(date + " 17:30:00").Unix(), Type: "in", Status: "early"}}
}

func TestLocalApplyPreviewQueuesHistoryAndRetainsPublishedVersion(t *testing.T) {
	ctx := context.WithValue(setup(t), "uid", int64(1))
	person := attDirtyEmployee(t, ctx, "自动核算")
	a := attAutoFixtureRule()
	a.Status = "published"
	a.Scope = AttScope{EmployeeIDs: []string{person}}
	attAutoStoreRule(t, ctx, a)
	newRule := a
	newRule.ID = "local-new-version"
	newRule.Version = 2
	newRule.Status = "draft"
	newRule.Config.Rule.LateGraceMinutes = 5
	attAutoStoreRule(t, ctx, newRule)
	p, err := PreviewAttendanceRule(ctx, newRule.ID, "2026-10-01", "2026-10-31")
	if err != nil || !p.CanApply || p.EmployeeCount != 1 || p.DayCount != 31 {
		t.Fatalf("preview: %+v %v", p, err)
	}
	count, _ := core.Model("biz_att_batch").Ctx(ctx).Count()
	if count != 0 {
		t.Fatal("preview wrote a batch")
	}
	_, _, ids, err := ApplyAttendanceRule(ctx, newRule.ID, "2026-10-01", "2026-10-31", "已确认10月追溯")
	if err != nil || len(ids) != 1 {
		t.Fatalf("apply: %v %v", ids, err)
	}
	old, err := attLoadRule(ctx, a.ID)
	if err != nil || old.Version != 1 || old.Config.Rule.LateGraceMinutes != 0 || old.Status != "published" {
		t.Fatalf("historical version mutated: %+v %v", old, err)
	}
	request, _ := core.Model("biz_att_batch").Ctx(ctx).Where("id", ids[0]).One()
	if request["start_date"].String() != "2026-10-01" || request["end_date"].String() != "2026-10-31" {
		t.Fatal("lost requested historical range")
	}
}

func TestLocalApplyMissingWindowsCannotPublishAndLockedMonthCannotChange(t *testing.T) {
	ctx := context.WithValue(setup(t), "uid", int64(1))
	attDirtyEmployee(t, ctx, "窗口测试")
	a := attAutoFixtureRule()
	a.Status = "draft"
	a.Config.Shifts[0].Segments[0].LocalWindows.InStartSecond = nil
	attAutoStoreRule(t, ctx, a)
	if _, _, _, err := ApplyAttendanceRule(ctx, a.ID, "2026-10-01", "2026-10-31", ""); err == nil {
		t.Fatal("unconfigured window was published")
	}
	b := attAutoFixtureRule()
	b.ID, b.FamilyID, b.Status = "valid-rule", "valid-family", "draft"
	attAutoStoreRule(t, ctx, b)
	_, err := core.Model("biz_att_month").Ctx(ctx).Data(g.Map{"month": "2026-10", "status": "locked", "batch_id": "previous", "revision": 1, "updated_by": 1, "updated_at": core.Now()}).Insert()
	if err != nil {
		t.Fatal(err)
	}
	p, err := PreviewAttendanceRule(ctx, b.ID, "2026-10-01", "2026-10-31")
	if err != nil || p.CanApply || len(p.LockedMonths) != 1 {
		t.Fatalf("locked preview %+v %v", p, err)
	}
	if _, _, _, err = ApplyAttendanceRule(ctx, b.ID, "2026-10-01", "2026-10-31", ""); err == nil {
		t.Fatal("locked month changed")
	}
}

func TestLocalBatchDerivesRosterAndIgnoresWeComStatusAndType(t *testing.T) {
	ctx := setup(t)
	person := attDirtyEmployee(t, ctx, "自动白班")
	a := attAutoFixtureRule()
	a.Status = "published"
	attAutoStoreRule(t, ctx, a)
	for i, p := range attAutoPunches("2026-10-12") {
		_, err := UpsertClock(ctx, Clock{ID: p.ID, EmployeeID: person, ClockType: p.Type, ClockTime: timeString(p.At), Status: p.Status, Source: "manual", ExternalRef: attJSON(i)})
		if err != nil {
			t.Fatal(err)
		}
	}
	id, err := attQueueBatch(ctx, AttBatchRequest{StartDate: "2026-10-12", EndDate: "2026-10-12", EmployeeIDs: []string{person}})
	if err != nil {
		t.Fatal(err)
	}
	attProcessQueue(ctx)
	b, _ := core.Model("biz_att_batch").Ctx(ctx).Where("id", id).One()
	if b["status"].String() != "completed" {
		t.Fatalf("batch failed: %v", b)
	}
	row, _ := core.Model("biz_att_result").Ctx(ctx).Where("batch_id", id).One()
	var v eng.Result
	_ = json.Unmarshal([]byte(row["result_json"].String()), &v)
	if v.State != "calculated" || v.ActualSeconds != 8*3600 || v.LateCount != 0 || v.EarlyCount != 0 {
		t.Fatalf("local result %+v", v)
	}
	roster, _ := core.Model("biz_att_roster").Ctx(ctx).Where("employee_id", person).Where("work_date", "2026-10-12").One()
	if roster["source"].String() != "automatic" {
		t.Fatal("calculation required manual calendar action")
	}
}

func timeString(at int64) string {
	return time.Unix(at, 0).In(core.Location).Format("2006-01-02 15:04:05")
}

func TestLocalDerivedRosterPreservesManualAssignment(t *testing.T) {
	ctx := setup(t)
	person := attDirtyEmployee(t, ctx, "人工调班")
	a := attAutoFixtureRule()
	a.Status = "published"
	attAutoStoreRule(t, ctx, a)
	err := attPutRosters(ctx, []AttRoster{{EmployeeID: person, WorkDate: "2026-10-12", RuleID: a.ID, ShiftID: "", Category: eng.Restday, Source: "manual", Status: "confirmed"}}, "已确认调休")
	if err != nil {
		t.Fatal(err)
	}
	if err = attEnsureDerivedRosters(ctx, AttBatchRequest{StartDate: "2026-10-12", EndDate: "2026-10-12", EmployeeIDs: []string{person}}); err != nil {
		t.Fatal(err)
	}
	row, _ := core.Model("biz_att_roster").Ctx(ctx).Where("employee_id", person).Where("work_date", "2026-10-12").One()
	roster := attDecodeRoster(row)
	if roster.Source != "manual" || roster.Category != eng.Restday || roster.ShiftID != "" {
		t.Fatalf("manual override lost: %+v", roster)
	}
}

func TestLocalCandidateRequiresUniqueCompleteEvidence(t *testing.T) {
	ctx := setup(t)
	person := AttEmployee{ID: attDirtyEmployee(t, ctx, "候选识别")}
	a := attAutoFixtureRule()
	day := a.Config.Cycle.Weekdays[0]
	id, issue, err := attSelectCompleteShift(ctx, person, "2026-10-12", day, a, attAutoPunches("2026-10-12"), nil)
	if err != nil || issue != "" || id != "day" {
		t.Fatalf("unique %s %s %v", id, issue, err)
	}
	a.Config.Shifts = append(a.Config.Shifts, a.Config.Shifts[0])
	a.Config.Shifts[1].ID = "same-evidence"
	if id, issue, err = attSelectCompleteShift(ctx, person, "2026-10-12", day, a, attAutoPunches("2026-10-12"), nil); err != nil || id != "" || issue == "" {
		t.Fatalf("ambiguous %s %s %v", id, issue, err)
	}
	if id, issue, err = attSelectCompleteShift(ctx, person, "2026-10-12", day, a, nil, nil); err != nil || id != "" || issue == "" {
		t.Fatalf("no evidence %s %s %v", id, issue, err)
	}
}

func TestLocalMissingRuleIsConfigurationPending(t *testing.T) {
	ctx := setup(t)
	person := attDirtyEmployee(t, ctx, "未配制度")
	id, err := attQueueBatch(ctx, AttBatchRequest{StartDate: "2026-10-12", EndDate: "2026-10-12", EmployeeIDs: []string{person}})
	if err != nil {
		t.Fatal(err)
	}
	attProcessQueue(ctx)
	row, _ := core.Model("biz_att_result").Ctx(ctx).Where("batch_id", id).One()
	if row["status"].String() != "configuration_pending" {
		t.Fatalf("missing configuration presented as %s", row["status"])
	}
}

func TestLocalAdjacentWorkDatesCannotShareOneRealPunch(t *testing.T) {
	ctx := setup(t)
	person := attDirtyEmployee(t, ctx, "跨日归属")
	a := attAutoFixtureRule()
	a.Status = "published"
	a.Config.Shifts = []eng.Shift{
		{ID: "long", Name: "跨日下班窗", Segments: []eng.Segment{{ID: "work", Kind: "work", StartMinute: 480, EndMinute: 1200, PunchMode: "both", LocalWindows: attAutoWindows(7*3600, 10*3600, 19*3600, 33*3600)}}},
		{ID: "day", Name: "次日白班", Segments: []eng.Segment{{ID: "work", Kind: "work", StartMinute: 480, EndMinute: 960, PunchMode: "both", LocalWindows: attAutoWindows(7*3600, 10*3600, 15*3600, 18*3600)}}},
	}
	attAutoStoreRule(t, ctx, a)
	rosters := map[string]AttRoster{
		"2026-10-12": {EmployeeID: person, WorkDate: "2026-10-12", RuleID: a.ID, ShiftID: "long", Status: "confirmed", Source: "manual", Category: eng.Workday, ScheduledWork: true},
		"2026-10-13": {EmployeeID: person, WorkDate: "2026-10-13", RuleID: a.ID, ShiftID: "day", Status: "confirmed", Source: "manual", Category: eng.Workday, ScheduledWork: true},
	}
	punches := []eng.Punch{{ID: "first", At: tm("2026-10-12 08:00:00").Unix()}, {ID: "shared", At: tm("2026-10-13 08:00:00").Unix()}, {ID: "last", At: tm("2026-10-13 16:00:00").Unix()}}
	conflicts, err := attSharedPunchDates(ctx, person, rosters, punches, nil, tm("2026-10-15").Unix())
	if err != nil || len(conflicts["2026-10-12"]) != 1 || len(conflicts["2026-10-13"]) != 1 || conflicts["2026-10-12"][0] != "shared" {
		t.Fatalf("cross-date duplication was accepted: %v %v", conflicts, err)
	}
	for _, r := range rosters {
		if err := attPutRosters(ctx, []AttRoster{r}, "跨日验证"); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range punches {
		if _, err := UpsertClock(ctx, Clock{ID: p.ID, EmployeeID: person, ClockTime: timeString(p.At), ClockType: "in", Status: "normal", Source: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
	id, err := attQueueBatch(ctx, AttBatchRequest{StartDate: "2026-10-12", EndDate: "2026-10-13", EmployeeIDs: []string{person}})
	if err != nil {
		t.Fatal(err)
	}
	attProcessQueue(ctx)
	rows, err := core.Model("biz_att_result").Ctx(ctx).Where("batch_id", id).All()
	if err != nil || len(rows) != 2 {
		t.Fatalf("results %v %v", rows, err)
	}
	for _, r := range rows {
		if r["status"].String() != "review" {
			t.Fatalf("cross-date day counted as normal %v", r)
		}
	}
	states, _ := core.Model("biz_att_clock_state").Ctx(ctx).Where("clock_id", "shared").One()
	if states["local_status"].String() != "pending_review" {
		t.Fatalf("source clock exposed as normal %v", states)
	}
}

func TestAttendanceOneClickMonthCloseAndPublicationFreshness(t *testing.T) {
	ctx := setup(t)
	c := newAttWorkflowClient(t, ctx)
	_, ruleID, batch := attWorkflowMonth(t, ctx, c)
	a, err := attLoadRule(ctx, ruleID)
	if err != nil {
		t.Fatal(err)
	}
	newRule := a
	newRule.ID = "new-effective-policy"
	newRule.Version++
	newRule.Config.Rule.LateGraceMinutes = 3
	attAutoStoreRule(t, ctx, newRule)
	if err := attRequireFreshBatch(ctx, batch); err == nil {
		t.Fatal("published policy change did not invalidate old result")
	}
	c.rejected("POST", "months/2026-08/close", g.Map{"batch_id": batch, "reason": "旧证据不能月结"})
	if count, _ := core.Model("biz_att_publication").Ctx(ctx).Count(); count != 0 {
		t.Fatal("rejected close left partial publication records")
	}
	// A complete recalculation can be reviewed, published and locked together.
	personRows, _ := core.Model("biz_hrm_employee").Ctx(ctx).Fields("id").All()
	ids := []string{}
	for _, p := range personRows {
		ids = append(ids, p["id"].String())
	}
	newBatch, err := attQueueBatch(ctx, AttBatchRequest{StartDate: "2026-08-01", EndDate: "2026-08-31", EmployeeIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	attProcessQueue(ctx)
	closed := c.ok("POST", "months/2026-08/close", g.Map{"batch_id": newBatch, "reason": "全部核对完成"})
	if closed["status"] != "locked" || closed["revision"] != float64(3) {
		t.Fatalf("one click close did not preserve audit steps: %v", closed)
	}
	c.rejected("POST", "months/2026-08/close", g.Map{"batch_id": newBatch, "reason": "不可重复月结"})
}

func TestAttendanceLatestResultsFilterAfterVersionSelection(t *testing.T) {
	ctx := setup(t)
	c := newAttWorkflowClient(t, ctx)
	person := attDirtyEmployee(t, ctx, "中文姓名")
	for _, fixture := range []struct {
		id, status, result string
		seq                int64
	}{{"old", "completed", "calculated", 100}, {"latest", "completed", "review", 200}, {"failed", "failed", "calculated", 300}} {
		_, err := core.Model("biz_att_batch").Ctx(ctx).Data(g.Map{"id": fixture.id, "status": fixture.status, "start_date": "2026-10-12", "end_date": "2026-10-12", "request_json": attJSON(AttBatchRequest{QueuedAtUnixNano: fixture.seq}), "created_by": 1, "created_at": core.Now()}).Insert()
		if err != nil {
			t.Fatal(err)
		}
		_, err = core.Model("biz_att_result").Ctx(ctx).Data(g.Map{"id": "result-" + fixture.id, "batch_id": fixture.id, "employee_id": person, "work_date": "2026-10-12", "rule_id": "r", "status": fixture.result, "employee_json": attJSON(AttEmployee{ID: person, Name: "中文姓名", EmployeeNo: "07319", DeptID: 10, Department: "Dev"}), "result_json": attJSON(eng.Result{EmployeeID: person, Date: "2026-10-12", State: fixture.result}), "input_json": "{}", "created_at": core.Now()}).Insert()
		if err != nil {
			t.Fatal(err)
		}
	}
	got := c.ok("GET", "results?status=calculated", nil)
	if len(got["items"].([]any)) != 0 {
		t.Fatalf("stale calculated result leaked: %v", got)
	}
	got = c.ok("GET", "results?status=review&q=7319&dept_id=10", nil)
	items := got["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["batch_id"] != "latest" {
		t.Fatalf("latest result filtering failed %v", got)
	}
	got = c.ok("GET", "results?q="+person, nil)
	if len(got["items"].([]any)) != 0 {
		t.Fatal("hidden internal ID matched name/number search")
	}
	got = c.ok("GET", "results?dept_id=9999", nil)
	if len(got["items"].([]any)) != 0 {
		t.Fatal("department filter ignored")
	}
}
