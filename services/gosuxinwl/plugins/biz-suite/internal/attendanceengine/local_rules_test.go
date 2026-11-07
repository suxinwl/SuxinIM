package attendanceengine

import (
	"testing"
)

func sec(h, m, s int) int                      { return h*3600 + m*60 + s }
func localW(a, b, c, d int) *LocalPunchWindows { return &LocalPunchWindows{&a, &b, &c, &d} }
func localInput() Input {
	return Input{EmployeeID: "local", Date: "2026-10-31", Category: Workday, AsOf: stamp("2026-11-03", "00:00:00"), Rule: Rule{EvidenceMode: "local", AttendanceMode: "actual", Overtime: map[Category]OvertimePolicy{Workday: {Enabled: true, MinimumMinutes: 30, RoundingMinutes: 30}, Restday: {Enabled: true, MinimumMinutes: 30, RoundingMinutes: 30, CompleteWorkdayMinutes: 480}}}}
}
func localCard(id string, seconds int) Punch {
	return Punch{ID: id, At: stamp("2026-10-31", "00:00:00") + int64(seconds), Type: "outside", Status: "位置异常", SourceGroupID: "wrong", SourceScheduleID: "wrong", ScheduledAt: stamp("2026-09-01", "00:00:00")}
}
func localShift() Shift {
	return Shift{ID: "day", SourceGroupID: "local-only", Segments: []Segment{
		{ID: "am", Kind: "work", StartMinute: 480, EndMinute: 720, LocalWindows: localW(sec(6, 0, 0), sec(10, 0, 0), sec(10, 0, 0), sec(12, 40, 0))},
		{ID: "pm", Kind: "work", StartMinute: 810, EndMinute: 1050, LocalWindows: localW(sec(12, 40, 0), sec(15, 0, 0), sec(15, 0, 0), sec(20, 0, 0))},
	}}
}
func TestLocalSecondBoundaryAndSourceLabelsAreIrrelevant(t *testing.T) {
	in := localInput()
	in.Shift = localShift()
	in.Punches = []Punch{localCard("am-in", sec(8, 0, 0)), localCard("am-out", sec(12, 39, 59)), localCard("pm-in", sec(12, 40, 0)), localCard("pm-out", sec(17, 30, 0))}
	in.Punches[0].Type = "out"
	in.Punches[0].Status = "missing" // valid actual stamp is not a placeholder
	in.Punches[1].Type = "in"
	r := Calculate(in)
	if r.State != "calculated" || r.ActualSeconds != 8*3600 || r.LateSeconds != 0 || r.EarlySeconds != 0 || len(r.PunchAssignments) != 4 {
		t.Fatalf("unexpected local result: %+v", r)
	}
	if r.Segments[0].OutPunchID != "am-out" || r.Segments[1].InPunchID != "pm-in" {
		t.Fatalf("second boundary was not exclusive: %+v", r.Segments)
	}
}
func TestLocalMissingConfigurationDoesNotClaimNormal(t *testing.T) {
	in := localInput()
	in.Shift = localShift()
	in.Shift.Segments[1].LocalWindows.InStartSecond = nil
	if ValidateLocalShift(in.Shift) == nil {
		t.Fatal("incomplete window published")
	}
	r := Calculate(in)
	if r.State != "review" || r.ActualSeconds != 0 || r.LateSeconds != 0 || r.EarlySeconds != 0 {
		t.Fatalf("unconfigured rule calculated: %+v", r)
	}
}
func TestLocalOverlapNeverUsesNearestSourcePlan(t *testing.T) {
	in := localInput()
	in.Shift = localShift()
	*in.Shift.Segments[0].LocalWindows.OutEndSecond = sec(13, 0, 0)
	p := localCard("overlap", sec(12, 45, 0))
	p.Type = "in"
	p.ScheduledAt = stamp(in.Date, "13:30:00")
	in.Punches = []Punch{p}
	r := Calculate(in)
	if r.State != "review" || len(r.PunchAssignments) != 0 {
		t.Fatalf("ambiguous card assigned by source: %+v", r)
	}
}
func TestLocalPlaceholderAndWindowClosing(t *testing.T) {
	in := localInput()
	in.Shift = Shift{ID: "short", Segments: []Segment{{ID: "morning", Kind: "work", StartMinute: 480, EndMinute: 720, LocalWindows: localW(sec(6, 0, 0), sec(10, 0, 0), sec(10, 0, 0), sec(12, 40, 0))}}}
	in.Punches = []Punch{localCard("in", sec(8, 0, 0)), localCard("source-missing", sec(12, 0, 0))}
	in.Punches[1].Placeholder = true
	in.AsOf = stamp(in.Date, "12:39:59")
	r := Calculate(in)
	if r.State != "pending" || r.MissingCount != 0 {
		t.Fatalf("window prematurely closed: %+v", r)
	}
	in.AsOf++
	r = Calculate(in)
	if r.State != "review" || r.MissingCount != 1 || r.ActualSeconds != 0 {
		t.Fatalf("placeholder became actual: %+v", r)
	}
}
func TestLocalPreviousDayAndCrossMonthWindows(t *testing.T) {
	in := localInput()
	in.Shift = Shift{ID: "night", Segments: []Segment{{ID: "night", Kind: "work", StartMinute: 1200, EndMinute: 1920, LocalWindows: localW(sec(18, 0, 0), sec(22, 0, 0), sec(30, 0, 0), sec(33, 0, 0))}}}
	in.Punches = []Punch{localCard("in", sec(20, 0, 0)), localCard("out", sec(32, 0, 0))}
	r := Calculate(in)
	if r.State != "calculated" || r.ActualSeconds != 12*3600 || r.Date != "2026-10-31" {
		t.Fatalf("night attribution: %+v", r)
	}
	in.Shift.Segments[0] = Segment{ID: "midnight", Kind: "work", StartMinute: 0, EndMinute: 480, LocalWindows: localW(-3600, 3600, sec(6, 0, 0), sec(10, 0, 0))}
	in.Punches = []Punch{localCard("prev", -1), localCard("out", sec(8, 0, 0))}
	r = Calculate(in)
	if r.State != "calculated" || r.ActualSeconds != 8*3600 || r.Segments[0].InPunchID != "prev" {
		t.Fatalf("previous day window lost: %+v", r)
	}
}
func qualityShift() Shift {
	am := Segment{ID: "am", Kind: "work", StartMinute: 480, EndMinute: 700, LocalWindows: localW(sec(6, 0, 0), sec(10, 0, 0), sec(10, 0, 0), sec(12, 0, 0))}
	pm := Segment{ID: "pm", Kind: "work", StartMinute: 790, EndMinute: 1050, LocalWindows: localW(sec(12, 0, 0), sec(15, 0, 0), sec(15, 0, 0), sec(17, 35, 0))}
	variantPM := pm
	variantPM.EndMinute = 1035
	ot := Segment{ID: "evening", Kind: "overtime", StartMinute: 1065, EndMinute: 1800, OvertimeStartMinute: 1080, OpenEnded: true, LocalWindows: localW(sec(17, 35, 0), sec(18, 0, 0), sec(18, 0, 0), sec(30, 0, 0))}
	return Shift{ID: "quality-normal", Segments: []Segment{am, pm}, OvertimeVariant: &ShiftVariant{TriggerMinute: 1065, Shift: Shift{ID: "quality-overtime", Segments: []Segment{am, variantPM, ot}}}}
}
func TestLocalQualityConditionalShiftAndMissingReturnReview(t *testing.T) {
	for _, tc := range []struct {
		name         string
		evening      []Punch
		wantOT       int64
		wantState    string
		wantPresence int64
	}{
		{"normal", nil, 0, "calculated", 8 * 3600},
		{"evening", []Punch{localCard("return", sec(17, 45, 0)), localCard("finish", sec(21, 0, 0))}, 3 * 3600, "calculated", 11 * 3600},
		{"missing-return", []Punch{localCard("finish", sec(21, 0, 0))}, 0, "review", 7*3600 + 45*60},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := localInput()
			in.Shift = qualityShift()
			in.Rule.AttendanceMode = "fixed_complete"
			in.Rule.StandardMinutes = 480
			end := sec(17, 30, 0)
			if tc.evening != nil {
				end = sec(17, 15, 0)
			}
			in.Punches = []Punch{localCard("a", sec(8, 0, 0)), localCard("b", sec(11, 40, 0)), localCard("c", sec(13, 10, 0)), localCard("d", end)}
			in.Punches = append(in.Punches, tc.evening...)
			r := Calculate(in)
			if r.State != tc.wantState || r.OvertimeSeconds != tc.wantOT || r.ActualSeconds != tc.wantPresence {
				t.Fatalf("quality result %+v", r)
			}
			if tc.wantState == "calculated" && r.CreditedSeconds != 8*3600 {
				t.Fatalf("complete standard not credited: %+v", r)
			}
		})
	}
}
func injectionShift(long bool) Shift {
	end := sec(16, 30, 0)
	if long {
		end = sec(20, 0, 0)
	}
	s := Shift{ID: "injection", EvidencePeriods: []EvidencePeriod{{ID: "presence", StartSecond: sec(8, 0, 0), EndSecond: end, LocalWindows: localW(sec(6, 0, 0), sec(10, 0, 0), sec(10, 0, 0), sec(22, 0, 0))}}, Segments: []Segment{{ID: "normal", Kind: "work", StartMinute: 480, EndMinute: 990, EvidenceID: "presence"}}}
	if long {
		s.Segments = append(s.Segments, Segment{ID: "extra", Kind: "overtime", StartMinute: 990, EndMinute: 1200, EvidenceID: "presence"})
	}
	return s
}
func TestLocalInjectionContinuousEvidenceAndSingleMealDeduction(t *testing.T) {
	for _, tc := range []struct {
		name               string
		long               bool
		end                int
		category           Category
		wantActual, wantOT int64
		state              string
	}{
		{"normal", false, sec(16, 30, 0), Workday, 8 * 3600, 0, "calculated"},
		{"long-weekday", true, sec(20, 0, 0), Workday, 11*3600 + 30*60, 3*3600 + 30*60, "calculated"},
		{"long-weekend", true, sec(20, 0, 0), Restday, 11*3600 + 30*60, 11*3600 + 30*60, "calculated"},
		{"morning-only", false, sec(12, 0, 0), Restday, 4 * 3600, 4 * 3600, "calculated"},
		{"unknown-meal", false, sec(14, 0, 0), Restday, 6 * 3600, 6 * 3600, "review"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := localInput()
			in.Shift = injectionShift(tc.long)
			in.Category = tc.category
			in.Rule.AttendanceMode = "fixed_complete"
			in.Rule.StandardMinutes = 480
			in.Rule.CompleteWorkDeductionMinutes = 30
			in.Rule.PartialWorkPolicy = &PartialWorkPolicy{Mode: "review", ActualRanges: []LocalTimeRange{{sec(8, 0, 0), sec(12, 0, 0)}}}
			in.Punches = []Punch{localCard("in", sec(8, 0, 0)), localCard("out", tc.end)}
			r := Calculate(in)
			if r.State != tc.state || r.ActualSeconds != tc.wantActual || r.OvertimeSeconds != tc.wantOT {
				t.Fatalf("injection result %+v", r)
			}
			if len(r.PunchAssignments) != 2 {
				t.Fatalf("continuous evidence duplicated %+v", r.PunchAssignments)
			}
			if tc.end >= sec(16, 30, 0) && (r.DeductedSeconds != 1800 || r.PresenceSeconds-r.ActualSeconds != 1800) {
				t.Fatalf("meal was not deducted once %+v", r)
			}
			if tc.category == Workday && r.CreditedSeconds != 8*3600 {
				t.Fatalf("normal credit %+v", r)
			}
		})
	}
}
func TestLocalContinuousPeriodMissingExitIsOneMissingCard(t *testing.T) {
	in := localInput()
	in.Shift = injectionShift(true)
	in.Punches = []Punch{localCard("in", sec(8, 0, 0))}
	r := Calculate(in)
	if r.State != "review" || r.MissingCount != 1 {
		t.Fatalf("one evidence endpoint counted twice: %+v", r)
	}
}
func TestLocalSlicingNightDoesNotRequireInvented0330Punch(t *testing.T) {
	in := localInput()
	in.Category = Restday
	in.Rule.AttendanceMode = "fixed_complete"
	in.Rule.StandardMinutes = 480
	in.Shift = Shift{ID: "slicing-night", EvidencePeriods: []EvidencePeriod{{ID: "middle", StartSecond: sec(23, 30, 0), EndSecond: sec(28, 30, 0), LocalWindows: localW(sec(23, 15, 0), sec(24, 30, 0), sec(27, 0, 0), sec(28, 45, 0))}}, Segments: []Segment{
		{ID: "first", Kind: "work", StartMinute: 1140, EndMinute: 1380, LocalWindows: localW(sec(17, 0, 0), sec(21, 0, 0), sec(21, 0, 0), sec(23, 15, 0))},
		{ID: "second", Kind: "work", StartMinute: 1410, EndMinute: 1650, EvidenceID: "middle"},
		{ID: "extension", Kind: "overtime", StartMinute: 1650, EndMinute: 1710, EvidenceID: "middle"},
		{ID: "last", Kind: "overtime", StartMinute: 1740, EndMinute: 1920, LocalWindows: localW(sec(28, 45, 0), sec(30, 0, 0), sec(30, 0, 0), sec(33, 0, 0))},
	}}
	in.Punches = []Punch{localCard("a", sec(19, 0, 0)), localCard("b", sec(23, 0, 0)), localCard("c", sec(23, 30, 0)), localCard("d", sec(28, 30, 0)), localCard("e", sec(29, 0, 0)), localCard("f", sec(32, 0, 0))}
	r := Calculate(in)
	if r.State != "calculated" || r.ActualSeconds != 12*3600 || r.OvertimeSeconds != 12*3600 || r.MissingCount != 0 || len(r.PunchAssignments) != 6 {
		t.Fatalf("slicing result %+v", r)
	}
}
func TestLocalLateDepartureUsesConfirmedActualOutNotOvertimeCredit(t *testing.T) {
	for _, tc := range []struct {
		out   string
		delay int
		late  int64
	}{{"21:30:00", 0, 3600}, {"21:30:01", 60, 0}} {
		t.Run(tc.out, func(t *testing.T) {
			in := localInput()
			in.Shift = Shift{ID: "office", Segments: []Segment{{ID: "day", Kind: "work", StartMinute: 540, EndMinute: 1080, LocalWindows: localW(sec(7, 0, 0), sec(11, 0, 0), sec(16, 0, 0), sec(20, 0, 0))}}}
			threshold := sec(21, 30, 0)
			in.Rule.LateDepartureAfterSecond = &threshold
			in.Rule.LateDepartureDelayMinutes = 60
			in.PreviousDate = "2026-10-30"
			in.PreviousOutAt = stamp(in.PreviousDate, tc.out)
			in.PreviousOvertimeMinutes = 0
			in.Punches = []Punch{localCard("in", sec(10, 0, 0)), localCard("out", sec(18, 0, 0))}
			r := Calculate(in)
			if r.LateArrivalMinutes != tc.delay || r.LateSeconds != tc.late {
				t.Fatalf("late departure entitlement %+v", r)
			}
		})
	}
}
func TestLocalProductionStaffOvertimeCapKeepsActualExcess(t *testing.T) {
	in := localInput()
	in.Shift = Shift{ID: "evening", Segments: []Segment{{ID: "ot", Kind: "overtime", StartMinute: 1080, EndMinute: 1440, OpenEnded: true, LocalWindows: localW(sec(17, 0, 0), sec(19, 0, 0), sec(19, 0, 0), sec(24, 0, 0))}}}
	p := in.Rule.Overtime[Workday]
	p.MaximumMinutes = 120
	in.Rule.Overtime[Workday] = p
	in.Punches = []Punch{localCard("in", sec(18, 0, 0)), localCard("out", sec(22, 0, 0))}
	r := Calculate(in)
	if r.State != "calculated" || r.ActualSeconds != 4*3600 || r.OvertimeSeconds != 2*3600 {
		t.Fatalf("overtime cap hid actual excess %+v", r)
	}
}

func TestLocalCompleteCreditDoesNotTreatHavingTwoCardsAsFullAttendance(t *testing.T) {
	in := localInput()
	in.Shift = localShift()
	in.Rule.AttendanceMode = "fixed_complete"
	in.Rule.StandardMinutes = 480
	in.Punches = []Punch{localCard("a", sec(8, 1, 0)), localCard("b", sec(12, 0, 0)), localCard("c", sec(13, 30, 0)), localCard("d", sec(17, 30, 0))}
	for i := range in.Punches {
		in.Punches[i].Status = "normal"
	}
	r := Calculate(in)
	if r.State != "calculated" || r.CreditedSeconds != 479*60 || r.LateSeconds != 60 {
		t.Fatalf("incomplete normal work got full credit %+v", r)
	}
}

func TestLocalInjectionConditionalUsesExtendedActualExit(t *testing.T) {
	for _, tc := range []struct {
		end     int
		variant bool
		wantOT  int64
	}{{sec(16, 30, 0), false, 0}, {sec(17, 0, 0), true, 30 * 60}, {sec(20, 0, 0), true, 210 * 60}} {
		in := localInput()
		in.Shift = injectionShift(false)
		in.Shift.OvertimeVariant = &ShiftVariant{TriggerMinute: 990, Shift: injectionShift(true)}
		in.Rule.AttendanceMode = "fixed_complete"
		in.Rule.StandardMinutes = 480
		in.Rule.CompleteWorkDeductionMinutes = 30
		in.Punches = []Punch{localCard("a", sec(8, 0, 0)), localCard("b", tc.end)}
		r := Calculate(in)
		selected := false
		for _, tr := range r.Trace {
			selected = selected || tr.Code == "overtime_variant_selected"
		}
		if r.State != "calculated" || selected != tc.variant || r.OvertimeSeconds != tc.wantOT || r.MissingCount != 0 || r.EarlySeconds != 0 {
			t.Fatalf("continuous conditional at %d: %+v", tc.end, r)
		}
	}
}

func TestLocalOvertimeRoundingBoundariesAndCombinedSegments(t *testing.T) {
	for _, tc := range []struct{ minutes, expected int }{{29, 0}, {30, 30}, {59, 30}, {60, 60}} {
		in := localInput()
		in.Shift = Shift{ID: "ot", Segments: []Segment{{ID: "ot", Kind: "overtime", StartMinute: 1080, EndMinute: 1440, OpenEnded: true, LocalWindows: localW(sec(17, 0, 0), sec(18, 1, 0), sec(18, 1, 0), sec(24, 0, 0))}}}
		in.Punches = []Punch{localCard("a", sec(18, 0, 0)), localCard("b", sec(18, tc.minutes, 0))}
		r := Calculate(in)
		if r.State != "calculated" || r.OvertimeSeconds != int64(tc.expected*60) {
			t.Fatalf("rounding %d: %+v", tc.minutes, r)
		}
	}
	in := localInput()
	in.Shift = Shift{ID: "two", Segments: []Segment{
		{ID: "first", Kind: "overtime", StartMinute: 540, EndMinute: 560, LocalWindows: localW(sec(8, 30, 0), sec(9, 5, 0), sec(9, 5, 0), sec(9, 30, 0))},
		{ID: "second", Kind: "overtime", StartMinute: 600, EndMinute: 620, LocalWindows: localW(sec(9, 30, 0), sec(10, 5, 0), sec(10, 5, 0), sec(10, 30, 0))},
	}}
	in.Punches = []Punch{localCard("a", sec(9, 0, 0)), localCard("b", sec(9, 20, 0)), localCard("c", sec(10, 0, 0)), localCard("d", sec(10, 20, 0))}
	r := Calculate(in)
	if r.State != "calculated" || r.RawOvertimeSeconds != 40*60 || r.OvertimeSeconds != 30*60 {
		t.Fatalf("two twenty-minute segments %+v", r)
	}
}
