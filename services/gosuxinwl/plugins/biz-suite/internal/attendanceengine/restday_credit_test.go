package attendanceengine

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"
)

func restAt(clock string) int64 { return stamp("2026-10-31", clock) }

func restProductionInput(rule Rule, shift Shift, eveningEnd string) Input {
	in := productionInput()
	in.Date, in.Category = "2026-10-31", Restday
	in.Rule, in.Shift = rule, shift
	in.AsOf = stamp("2026-11-01", "12:00:00")
	for i := range in.Punches {
		in.Punches[i].At += 2 * 86400
	}
	if eveningEnd != "" {
		in.Punches[3].At = restAt("17:15:00")
		in.Punches = append(in.Punches,
			Punch{ID: "ot-in", Type: "in", Status: "normal", At: restAt("17:45:00")},
			Punch{ID: "ot-out", Type: "out", Status: "normal", At: restAt(eveningEnd)})
	}
	return in
}

func tracePresent(r Result, code string) bool {
	for _, trace := range r.Trace {
		if trace.Code == code {
			return true
		}
	}
	return false
}

func requireOvertimeTotals(t *testing.T, r Result) {
	t.Helper()
	var buckets, segments, rawBuckets, rawSegments int64
	for _, b := range r.OvertimeBuckets {
		buckets += b.Seconds
		rawBuckets += b.RawSeconds
		if b.CompleteWorkdayApplied || b.WorkdayActualSeconds > 0 || b.AdditionalSeconds > 0 {
			if b.WorkdayCreditedSeconds+b.AdditionalSeconds != b.Seconds {
				t.Fatalf("split totals do not reconcile: %+v", b)
			}
		}
	}
	for _, s := range r.Segments {
		segments += s.OvertimeSeconds
		rawSegments += s.RawOvertimeSeconds
	}
	if buckets != r.OvertimeSeconds || segments != r.OvertimeSeconds || rawBuckets != r.RawOvertimeSeconds || rawSegments != r.RawOvertimeSeconds {
		t.Fatalf("overtime totals disagree: %+v", r)
	}
}

func TestRestdayCompleteWorkdayWebPresetEightPlusEvening(t *testing.T) {
	rule, shift := productionWebPreset(t)
	if rule.Overtime[Restday].CompleteWorkdayMinutes != 480 || rule.Overtime[Workday].CompleteWorkdayMinutes != 0 {
		t.Fatal("web preset must explicitly enable only the restday complete-workday rule")
	}
	for _, tc := range []struct {
		end                          string
		actual, raw, overtime, extra int64
	}{
		{"", 480 * 60, 480 * 60, 480 * 60, 0},
		{"21:00:00", 660 * 60, 645 * 60, 660 * 60, 180 * 60},
		{"20:59:00", 659 * 60, 644 * 60, 630 * 60, 150 * 60},
		{"18:20:00", 500 * 60, 485 * 60, 480 * 60, 0},
		{"18:29:59", 509*60 + 59, 494*60 + 59, 480 * 60, 0},
		{"18:30:00", 510 * 60, 495 * 60, 510 * 60, 30 * 60},
		{"18:59:59", 539*60 + 59, 524*60 + 59, 510 * 60, 30 * 60},
		{"19:00:00", 540 * 60, 525 * 60, 540 * 60, 60 * 60},
	} {
		t.Run(tc.end, func(t *testing.T) {
			in := restProductionInput(rule, shift, tc.end)
			before, _ := json.Marshal(in)
			r := Calculate(in)
			requireClean(t, r)
			if r.ActualSeconds != tc.actual || r.RawOvertimeSeconds != tc.raw || r.OvertimeSeconds != tc.overtime || r.CreditedSeconds != 0 || len(r.OvertimeBuckets) != 1 || r.MissingCount != 0 {
				t.Fatalf("8+N rule mixed actual and credited metrics: %+v", r)
			}
			b := r.OvertimeBuckets[0]
			wantActual := int64(465 * 60)
			if tc.end == "" {
				wantActual = 480 * 60
			}
			if !b.CompleteWorkdayApplied || b.WorkdayActualSeconds != wantActual || b.WorkdayCreditedSeconds != 480*60 || b.AdditionalSeconds != tc.extra || !tracePresent(r, "complete_workday_overtime_credit") {
				t.Fatal(b, r.Trace)
			}
			requireOvertimeTotals(t, r)
			after, _ := json.Marshal(in)
			if string(before) != string(after) || !reflect.DeepEqual(r, Calculate(in)) {
				t.Fatal("credit mutated evidence or became nondeterministic")
			}
		})
	}
}

func TestRestdayCreditRequiresFullWorkdayButNotCompleteEvening(t *testing.T) {
	rule, shift := productionWebPreset(t)
	t.Run("no_punches_rest", func(t *testing.T) {
		in := restProductionInput(rule, shift, "")
		in.Punches = nil
		r := Calculate(in)
		if r.State != "rest" || r.OvertimeSeconds != 0 || r.ActualSeconds != 0 || r.MissingCount != 0 || tracePresent(r, "complete_workday_overtime_credit") {
			t.Fatal(r)
		}
	})
	t.Run("complete_morning_only_exact_3h40", func(t *testing.T) {
		in := restProductionInput(rule, shift, "")
		in.Punches = in.Punches[:2]
		r := Calculate(in)
		requireClean(t, r)
		if r.OvertimeSeconds != 220*60 || r.ActualSeconds != 220*60 || r.OvertimeBuckets[0].CompleteWorkdayApplied {
			t.Fatal(r)
		}
		if r.LateSeconds != 0 || r.EarlySeconds != 0 || r.LateCount != 0 || r.EarlyCount != 0 {
			t.Fatal("voluntary half day was penalized", r)
		}
		requireOvertimeTotals(t, r)
	})
	t.Run("partial_white_cannot_top_up_29min_evening", func(t *testing.T) {
		in := restProductionInput(rule, shift, "18:29:59")
		in.Punches = append(in.Punches[:2:2], in.Punches[4:]...)
		r := Calculate(in)
		requireClean(t, r)
		if r.OvertimeSeconds != 220*60 || r.OvertimeBuckets[0].AdditionalSeconds != 0 || r.OvertimeBuckets[0].CompleteWorkdayApplied {
			t.Fatal(r)
		}
		requireOvertimeTotals(t, r)
	})
	for _, lateSeconds := range []int64{1, 60} {
		for _, grace := range []int{0, 5} {
			t.Run("late_"+strconv.FormatInt(lateSeconds, 10)+"_seconds_grace_"+strconv.Itoa(grace), func(t *testing.T) {
				in := restProductionInput(rule, shift, "18:29:59")
				in.Punches[0].At += lateSeconds
				// Even an explicit grace period affects lateness classification only;
				// it does not fabricate complete physical attendance for fixed OT.
				in.Rule.LateGraceMinutes = grace
				r := Calculate(in)
				requireClean(t, r)
				if r.OvertimeSeconds != 465*60-lateSeconds || r.OvertimeBuckets[0].AdditionalSeconds != 0 || r.OvertimeBuckets[0].CompleteWorkdayApplied {
					t.Fatal("late/under-30-minute fractions combined or gifted full credit", r)
				}
				if r.RawLateSeconds != lateSeconds || r.LateSeconds != 0 || r.EarlySeconds != 0 || r.LateCount != 0 || r.EarlyCount != 0 {
					t.Fatal("voluntary restday boundary differences became attendance penalties", r)
				}
				requireOvertimeTotals(t, r)
			})
		}
	}
	t.Run("leave_during_morning_without_afternoon_is_actual_not_early_penalty", func(t *testing.T) {
		in := restProductionInput(rule, shift, "")
		in.Punches = in.Punches[:2]
		in.Punches[1].At = restAt("11:00:00")
		r := Calculate(in)
		requireClean(t, r)
		if r.ActualSeconds != 180*60 || r.OvertimeSeconds != 180*60 || r.RawEarlySeconds != 40*60 || r.EarlySeconds != 0 || r.EarlyCount != 0 || r.LateCount != 0 || r.OvertimeBuckets[0].CompleteWorkdayApplied {
			t.Fatal("voluntary early finish was penalized or credited a complete day", r)
		}
		requireOvertimeTotals(t, r)
	})
	t.Run("white_missing_out_review_no_eight_hours", func(t *testing.T) {
		in := restProductionInput(rule, shift, "21:00:00")
		in.Punches = append(in.Punches[:3:3], in.Punches[4:]...)
		r := Calculate(in)
		if r.State != "review" || !hasIssue(r, "overtime_punch_incomplete") || r.OvertimeSeconds != 400*60 || r.OvertimeBuckets[0].CompleteWorkdayApplied {
			t.Fatal(r)
		}
		requireOvertimeTotals(t, r)
	})
	t.Run("night_missing_in_keeps_evidenced_eight_hours_under_review", func(t *testing.T) {
		in := restProductionInput(rule, shift, "21:00:00")
		in.Punches = append(in.Punches[:4:4], in.Punches[5])
		r := Calculate(in)
		if r.State != "review" || !hasIssue(r, "overtime_punch_incomplete") || r.OvertimeSeconds != 480*60 || r.ActualSeconds != 465*60 || !r.OvertimeBuckets[0].CompleteWorkdayApplied || r.OvertimeBuckets[0].AdditionalSeconds != 0 {
			t.Fatal(r)
		}
		requireOvertimeTotals(t, r)
	})
}

func TestVoluntaryRestdayBoundaryPolicyIsScoped(t *testing.T) {
	rule, shift := productionWebPreset(t)
	for _, tc := range []struct {
		name            string
		category        Category
		required        bool
		completeMinutes int
		penalized       bool
	}{
		{"new_optional_restday", Restday, false, 480, false},
		{"legacy_optional_restday", Restday, false, 0, true},
		{"required_restday", Restday, true, 480, true},
		{"weekday", Workday, false, 480, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := restProductionInput(rule, shift, "")
			in.Category, in.ScheduledWork = tc.category, tc.required
			in.Rule.Overtime = map[Category]OvertimePolicy{Restday: {Enabled: true, CompleteWorkdayMinutes: tc.completeMinutes}, Workday: {Enabled: true}}
			in.Punches[0].At += 60
			in.Punches[3].At -= 60
			r := Calculate(in)
			requireClean(t, r)
			if r.RawLateSeconds != 60 || r.RawEarlySeconds != 60 || r.ActualSeconds != 478*60 {
				t.Fatal("raw evidence changed", r)
			}
			if tc.penalized {
				if r.LateSeconds != 60 || r.EarlySeconds != 60 || r.LateCount != 1 || r.EarlyCount != 1 {
					t.Fatal("existing attendance policy changed", r)
				}
			} else if r.LateSeconds != 0 || r.EarlySeconds != 0 || r.LateCount != 0 || r.EarlyCount != 0 || r.OvertimeSeconds != 478*60 || !tracePresent(r, "voluntary_restday_boundary") {
				t.Fatal("voluntary restday should retain actual OT without attendance penalty", r)
			}
		})
	}
}

func TestRestdayCreditHonorsApprovalIntersectionAndDailyCap(t *testing.T) {
	rule, shift := productionWebPreset(t)
	for _, tc := range []struct {
		name      string
		approvals []Absence
		want      int64
		complete  bool
	}{
		{"no_approval", nil, 0, false},
		{"full_approval", []Absence{{ID: "full", Kind: "overtime", Status: "approved", StartAt: restAt("08:00:00"), EndAt: restAt("21:00:00")}}, 660 * 60, true},
		{"partial_white_and_full_night", []Absence{{ID: "morning", Kind: "overtime", Status: "approved", StartAt: restAt("08:00:00"), EndAt: restAt("11:40:00")}, {ID: "night", Kind: "overtime", Status: "approved", StartAt: restAt("18:00:00"), EndAt: restAt("21:00:00")}}, 400 * 60, false},
		{"complete_white_only", []Absence{{ID: "white", Kind: "overtime", Status: "approved", StartAt: restAt("08:00:00"), EndAt: restAt("17:15:00")}}, 480 * 60, true},
		{"canceled", []Absence{{ID: "full", Kind: "overtime", Status: "canceled", StartAt: restAt("08:00:00"), EndAt: restAt("21:00:00")}}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := restProductionInput(rule, shift, "21:00:00")
			in.Rule.Overtime = map[Category]OvertimePolicy{Restday: {Enabled: true, RequireApproval: true, CompleteWorkdayMinutes: 480}}
			in.Absences = tc.approvals
			r := Calculate(in)
			requireClean(t, r)
			if r.OvertimeSeconds != tc.want || r.ActualSeconds != 660*60 || len(r.OvertimeBuckets) != 1 || r.OvertimeBuckets[0].CompleteWorkdayApplied != tc.complete {
				t.Fatal(r)
			}
			requireOvertimeTotals(t, r)
		})
	}
	for _, tc := range []struct{ cap, want int }{{600, 600}, {550, 540}, {450, 450}} {
		t.Run("cap_"+strconv.Itoa(tc.cap), func(t *testing.T) {
			in := restProductionInput(rule, shift, "21:00:00")
			in.Rule.Overtime = map[Category]OvertimePolicy{Restday: {Enabled: true, CompleteWorkdayMinutes: 480, MaximumMinutes: tc.cap}}
			r := Calculate(in)
			if r.OvertimeSeconds != int64(tc.want)*60 || r.ActualSeconds != 660*60 || !tracePresent(r, "complete_workday_overtime_cap") {
				t.Fatal(r)
			}
			requireOvertimeTotals(t, r)
		})
	}
}

func TestRestdayCreditCalendarSplitDoesNotDuplicateOrBypassCategories(t *testing.T) {
	rule, shift := productionWebPreset(t)
	in := restProductionInput(rule, shift, "21:00:00")
	in.Punches[5].At = stamp("2026-11-01", "01:00:00")
	in.Rule.OvertimeDateMode = "calendar"
	in.DayCategories = map[string]Category{"2026-11-01": Holiday}
	in.Rule.Overtime = map[Category]OvertimePolicy{Restday: {Enabled: true, CompleteWorkdayMinutes: 480}, Holiday: {Enabled: false}}
	r := Calculate(in)
	requireClean(t, r)
	if len(r.OvertimeBuckets) != 1 || r.OvertimeSeconds != 14*3600 || r.OvertimeBuckets[0].WorkdayCreditedSeconds != 8*3600 {
		t.Fatal(r)
	}
	requireOvertimeTotals(t, r)
	in.Rule.Overtime[Holiday] = OvertimePolicy{Enabled: true, RequireApproval: true}
	r = Calculate(in)
	if r.OvertimeSeconds != 14*3600 {
		t.Fatal("unapproved next-day category credited", r)
	}
	in.Absences = []Absence{{ID: "next-night", Kind: "overtime", Status: "approved", StartAt: stamp("2026-11-01", "00:00:00"), EndAt: stamp("2026-11-01", "01:00:00")}}
	r = Calculate(in)
	requireClean(t, r)
	if len(r.OvertimeBuckets) != 2 || r.OvertimeSeconds != 15*3600 || r.OvertimeBuckets[1].CompleteWorkdayApplied || r.OvertimeBuckets[1].WorkdayCreditedSeconds != 0 {
		t.Fatal("calendar split duplicated whole workday", r)
	}
	requireOvertimeTotals(t, r)
	// A work segment itself spanning dates has no defined allocation of an
	// entire fixed workday. Preserve actual categorized time and explain it.
	in = restProductionInput(rule, shift, "")
	in.Rule.OvertimeDateMode = "calendar"
	in.Rule.Overtime = map[Category]OvertimePolicy{Restday: {Enabled: true, CompleteWorkdayMinutes: 480}}
	in.DayCategories = map[string]Category{"2026-11-01": Restday}
	in.Shift = Shift{ID: "cross-date-work", Segments: []Segment{{ID: "work", Kind: "work", StartMinute: 22 * 60, EndMinute: 26 * 60}}}
	in.Punches = []Punch{{ID: "in", Type: "in", Status: "normal", At: restAt("22:00:00")}, {ID: "out", Type: "out", Status: "normal", At: stamp("2026-11-01", "02:00:00")}}
	r = Calculate(in)
	requireClean(t, r)
	if r.OvertimeSeconds != 4*3600 || !tracePresent(r, "complete_workday_calendar_unresolved") {
		t.Fatal(r)
	}
	for _, b := range r.OvertimeBuckets {
		if b.CompleteWorkdayApplied {
			t.Fatal("ambiguous calendar workday received fixed credit", b)
		}
	}
	requireOvertimeTotals(t, r)
}

func TestRestdayCreditDefaultZeroAndRequiredWorkRemainUnchanged(t *testing.T) {
	rule, shift := productionWebPreset(t)
	in := restProductionInput(rule, shift, "21:00:00")
	in.Rule.Overtime = map[Category]OvertimePolicy{Restday: {Enabled: true}}
	r := Calculate(in)
	if r.OvertimeSeconds != 630*60 || r.RawOvertimeSeconds != 645*60 || r.OvertimeBuckets[0].CompleteWorkdayApplied {
		t.Fatal("zero changed legacy rounding", r)
	}
	in.Rule.Overtime[Restday] = OvertimePolicy{Enabled: true, CompleteWorkdayMinutes: 480}
	in.ScheduledWork = true
	r = Calculate(in)
	if r.OvertimeSeconds != 180*60 || r.OvertimeBuckets[0].CompleteWorkdayApplied {
		t.Fatal("required normal work gained optional restday credit", r)
	}
	in.Category = Workday
	in.ScheduledWork = false
	in.Rule.Overtime[Workday] = OvertimePolicy{Enabled: true}
	r = Calculate(in)
	if r.OvertimeSeconds != 180*60 || r.ActualSeconds != 660*60 || r.OvertimeBuckets[0].CompleteWorkdayApplied {
		t.Fatal("weekday rule changed", r)
	}
}

func TestCompleteWorkdayPolicyValidation(t *testing.T) {
	for _, tc := range []struct {
		category Category
		minutes  int
	}{{Restday, -1}, {Restday, 1441}, {Workday, 480}, {Holiday, 480}} {
		if ValidateRule(Rule{Overtime: map[Category]OvertimePolicy{tc.category: {CompleteWorkdayMinutes: tc.minutes}}}) == nil {
			t.Fatal("invalid complete-workday policy accepted", tc)
		}
	}
	if e := ValidateRule(Rule{Overtime: map[Category]OvertimePolicy{Restday: {Enabled: true, CompleteWorkdayMinutes: 480}}}); e != nil {
		t.Fatal(e)
	}
}
