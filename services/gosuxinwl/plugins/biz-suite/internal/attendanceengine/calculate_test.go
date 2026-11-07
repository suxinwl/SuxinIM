package attendanceengine

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func stamp(date, clock string) int64 {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", date+" "+clock, Location)
	if err != nil {
		panic(err)
	}
	return t.Unix()
}
func at(clock string) int64 { return stamp("2026-10-29", clock) }
func punch(id, kind, clock string) Punch {
	return Punch{ID: id, At: at(clock), Type: kind, Status: "normal"}
}
func baseline() Input {
	return Input{EmployeeID: "E1", Date: "2026-10-29", Category: Workday, AsOf: at("23:59:59"), Rule: Rule{Version: "r1", Window: PunchWindow{120, 120, 120, 120}, AttendanceMode: "fixed"}, Shift: Shift{ID: "day", Version: "s1", Segments: []Segment{{ID: "day", Kind: "work", StartMinute: 540, EndMinute: 1080}, {ID: "lunch", Kind: "break", StartMinute: 720, EndMinute: 780}}}, Punches: []Punch{punch("in", "in", "09:00:00"), punch("out", "out", "18:00:00")}}
}
func hasIssue(r Result, code string) bool {
	for _, s := range r.Issues {
		if s == code {
			return true
		}
	}
	return false
}
func requireClean(t *testing.T, r Result) {
	t.Helper()
	if r.State != "calculated" {
		t.Fatalf("state=%s issues=%v trace=%+v", r.State, r.Issues, r.Trace)
	}
}

func TestActualCreditedAndOvertimeAreIndependent(t *testing.T) {
	in := baseline()
	in.Rule.StandardMinutes = 450
	r := Calculate(in)
	requireClean(t, r)
	if r.ActualSeconds != 8*3600 || r.CreditedSeconds != 450*60 || r.OvertimeSeconds != 0 || !r.FullAttendanceEligible {
		t.Fatalf("bad metrics: %+v", r)
	}
	// Production staff's scheduled Saturday attendance may also be overtime.
	in.Category = Restday
	in.ScheduledWork = true
	in.Rule.StandardMinutes = 480
	in.Shift.Segments = append(in.Shift.Segments, Segment{ID: "sat-ot", Kind: "overtime", StartMinute: 540, EndMinute: 1080})
	in.Rule.Overtime = map[Category]OvertimePolicy{Restday: {Enabled: true}}
	r = Calculate(in)
	requireClean(t, r)
	if r.ActualSeconds != 8*3600 || r.CreditedSeconds != 8*3600 || r.OvertimeSeconds != 8*3600 {
		t.Fatalf("overlap double counted: %+v", r)
	}
}

func TestMissingIsNotAbsenceAndManualCreditNeedsReason(t *testing.T) {
	in := baseline()
	in.Punches = in.Punches[:1]
	r := Calculate(in)
	requireClean(t, r)
	if r.MissingCount != 1 || r.ConfirmedAbsentSeconds != 0 || r.CreditedSeconds != 0 || r.FullAttendanceEligible {
		t.Fatalf("missing inferred absence: %+v", r)
	}
	in.Adjustments = []Adjustment{{ID: "adjust-1", Kind: "credit", Status: "approved", StartAt: at("09:00:00"), EndAt: at("18:00:00")}}
	r = Calculate(in)
	if !hasIssue(r, "adjustment_audit_missing") || r.CreditedSeconds != 0 {
		t.Fatalf("unaudited credit used %+v", r)
	}
	in.Adjustments[0].Reason = "人事核实当天正常上班，终端故障"
	r = Calculate(in)
	requireClean(t, r)
	if r.CreditedSeconds != 8*3600 || r.ActualSeconds != 0 || r.MissingCount != 1 {
		t.Fatalf("adjustment fabricated actual work %+v", r)
	}
	in.Adjustments = append(in.Adjustments, Adjustment{ID: "waive", Kind: "waive_missing", Status: "approved", Reason: "补签已核实"})
	r = Calculate(in)
	if r.MissingCount != 0 || !r.FullAttendanceEligible || r.ActualSeconds != 0 {
		t.Fatalf("waiver failed %+v", r)
	}
}

func TestSourceMissingAndUnknownAreNotRealPunches(t *testing.T) {
	in := baseline()
	in.Punches[0].Status = "missing"
	r := Calculate(in)
	if r.MissingCount != 1 || r.ActualSeconds != 0 || r.ConfirmedAbsentSeconds != 0 {
		t.Fatalf("source placeholder counted %+v", r)
	}
	in.Punches[0].Status = "normal"
	in.Punches[0].Type = ""
	r = Calculate(in)
	if !hasIssue(r, "punch_type_unknown") || r.State != "review" {
		t.Fatal(r)
	}
	in.Punches[0].Type = "in"
	in.Punches[0].Status = "abnormal"
	r = Calculate(in)
	if !hasIssue(r, "punch_abnormal") {
		t.Fatal(r)
	}
	in.Rule.AllowAbnormalPunches = true
	r = Calculate(in)
	requireClean(t, r)
}

func TestGraceDisplayAndDeductionAreSeparate(t *testing.T) {
	in := baseline()
	in.Rule.LateGraceMinutes = 5
	in.Punches[0].At = at("09:03:00")
	r := Calculate(in)
	requireClean(t, r)
	if r.RawLateSeconds != 180 || r.LateSeconds != 0 || r.GraceSeconds != 0 || r.CreditedSeconds != 8*3600 {
		t.Fatal(r)
	}
	in.Rule.GraceMode = "grace"
	r = Calculate(in)
	if r.GraceSeconds != 180 || r.LateCount != 0 {
		t.Fatal(r)
	}
	in.Punches[0].At = at("09:08:00")
	in.Rule.LateDeductionMode = "excess"
	r = Calculate(in)
	if r.LateSeconds != 480 || r.CreditedSeconds != 8*3600-180 {
		t.Fatal(r)
	}
	in.Rule.LateDeductionMode = "full"
	r = Calculate(in)
	if r.CreditedSeconds != 8*3600-480 {
		t.Fatal(r)
	}
	in.Rule.AttendanceMode = "actual"
	r = Calculate(in)
	if r.CreditedSeconds != 8*3600-480 {
		t.Fatalf("actual mode double deducted %+v", r)
	}
}

func TestFlexAndPreviousOvertimeEntitlement(t *testing.T) {
	in := baseline()
	in.Rule.FlexMinutes = 30
	in.Punches[0].At = at("09:10:30")
	in.Punches[1].At = at("18:10:30")
	r := Calculate(in)
	requireClean(t, r)
	if r.FlexAppliedSeconds != 630 || r.ActualSeconds != 8*3600 || r.EarlySeconds != 0 || r.LateSeconds != 0 {
		t.Fatal(r)
	}
	in = baseline()
	in.Rule.AttendanceMode = "actual"
	in.Rule.LateArrivalTiers = []LateArrivalTier{{PreviousOvertimeMinutes: 120, DelayMinutes: 30}, {PreviousOvertimeMinutes: 180, DelayMinutes: 60}}
	in.PreviousDate = "2026-10-28"
	in.PreviousOvertimeMinutes = 180
	in.Punches[0].At = at("10:00:00")
	r = Calculate(in)
	requireClean(t, r)
	if r.LateArrivalMinutes != 60 || r.ActualSeconds != 7*3600 || r.CreditedSeconds != 8*3600 || r.LateCount != 0 {
		t.Fatalf("entitlement not separated %+v", r)
	}
	in.PreviousDate = "2026-10-27"
	r = Calculate(in)
	if !hasIssue(r, "previous_day_unresolved") {
		t.Fatal(r)
	}
}

func TestApprovedAbsenceDedupeOverlapAndUnits(t *testing.T) {
	in := baseline()
	in.Rule.AttendanceMode = "actual"
	in.Rule.LeavePolicies = map[string]LeavePolicy{"年假": {CreditAttendance: true, FullAttendanceAllowed: true}}
	a := Absence{ID: "L1", Kind: "leave", LeaveType: "年假", Status: "approved", StartAt: at("09:00:00"), EndAt: at("10:00:00")}
	in.Absences = []Absence{a, a}
	in.Punches[0].At = at("10:00:00")
	r := Calculate(in)
	requireClean(t, r)
	if r.LeaveSeconds != 3600 || r.ActualSeconds != 7*3600 || r.CreditedSeconds != 8*3600 || r.LateCount != 0 || !r.FullAttendanceEligible {
		t.Fatal(r)
	}
	in.Absences = append(in.Absences, Absence{ID: "O1", Kind: "out", Status: "approved", StartAt: at("09:30:00"), EndAt: at("10:30:00")})
	r = Calculate(in)
	if !hasIssue(r, "approval_overlap") || r.State != "review" {
		t.Fatal(r)
	}
	in.Absences = []Absence{a}
	in.Absences[0].Status = "canceled"
	r = Calculate(in)
	if r.LeaveSeconds != 0 || r.LateSeconds != 3600 {
		t.Fatalf("canceled approval still applied %+v", r)
	}
	in.Absences[0] = a
	in.Absences[0].Unit = "day"
	r = Calculate(in)
	if !hasIssue(r, "approval_interval_unresolved") {
		t.Fatal(r)
	}
	in.Absences[0].Slices = []Interval{{StartAt: a.StartAt, EndAt: a.EndAt}}
	r = Calculate(in)
	requireClean(t, r)
}

func TestOvertimeDailyRoundingAndApprovalIntersection(t *testing.T) {
	in := baseline()
	in.Category = Restday
	in.Shift.Segments = []Segment{{ID: "a", Kind: "overtime", StartMinute: 540, EndMinute: 560}, {ID: "b", Kind: "overtime", StartMinute: 600, EndMinute: 620}}
	in.Rule.Window = PunchWindow{5, 5, 5, 5}
	in.Rule.Overtime = map[Category]OvertimePolicy{Restday: {Enabled: true}}
	in.Punches = []Punch{punch("a-in", "in", "09:00:00"), punch("a-out", "out", "09:20:00"), punch("b-in", "in", "10:00:00"), punch("b-out", "out", "10:20:00")}
	r := Calculate(in)
	requireClean(t, r)
	if r.RawOvertimeSeconds != 40*60 || r.OvertimeSeconds != 30*60 || r.ActualSeconds != 40*60 {
		t.Fatal(r)
	}
	in.Rule.Overtime[Restday] = OvertimePolicy{Enabled: true, RoundingScope: "segment"}
	r = Calculate(in)
	if r.OvertimeSeconds != 0 {
		t.Fatal(r)
	}
	in.Rule.Overtime[Restday] = OvertimePolicy{Enabled: true, RequireApproval: true}
	a := Absence{ID: "ot", Kind: "overtime", Status: "approved", StartAt: at("09:10:00"), EndAt: at("10:20:00")}
	in.Absences = []Absence{a, a}
	r = Calculate(in)
	if r.RawOvertimeSeconds != 30*60 || r.OvertimeSeconds != 30*60 {
		t.Fatal(r)
	}
	in.Absences[0].Status = "canceled"
	in.Absences = in.Absences[:1]
	r = Calculate(in)
	if r.OvertimeSeconds != 0 || r.ActualSeconds != 40*60 {
		t.Fatal(r)
	}
}

func TestOvernightCategorySplitAndBreaks(t *testing.T) {
	in := baseline()
	in.Category = Restday
	in.Rule.Overtime = map[Category]OvertimePolicy{Restday: {Enabled: true}, Holiday: {Enabled: true}}
	in.Shift.Segments = []Segment{{ID: "night", Kind: "overtime", StartMinute: 23 * 60, EndMinute: 26 * 60}, {ID: "break", Kind: "break", StartMinute: 24*60 + 30, EndMinute: 25 * 60}}
	in.Punches = []Punch{punch("n-in", "in", "23:00:00"), {ID: "n-out", At: stamp("2026-10-30", "02:00:00"), Type: "out", Status: "normal"}}
	in.AsOf = stamp("2026-10-30", "06:00:00")
	r := Calculate(in)
	requireClean(t, r)
	if r.OvertimeSeconds != 150*60 || len(r.OvertimeBuckets) != 1 || r.Date != "2026-10-29" {
		t.Fatal(r)
	}
	in.Rule.OvertimeDateMode = "calendar"
	in.DayCategories = map[string]Category{"2026-10-30": Holiday}
	r = Calculate(in)
	requireClean(t, r)
	if len(r.OvertimeBuckets) != 2 || r.OvertimeBuckets[0].Seconds != 60*60 || r.OvertimeBuckets[1].Seconds != 90*60 {
		t.Fatal(r)
	}
	in.DayCategories = nil
	r = Calculate(in)
	if !hasIssue(r, "overtime_category_missing") {
		t.Fatal(r)
	}
}

func TestPunchSelectionAndAmbiguity(t *testing.T) {
	in := baseline()
	in.Punches = []Punch{punch("i1", "in", "08:50:00"), punch("i2", "in", "09:02:00"), punch("o1", "out", "17:58:00"), punch("o2", "out", "18:10:00")}
	for _, tc := range []struct{ mode, inID, outID string }{{"first_last", "i1", "o2"}, {"earliest", "i1", "o1"}, {"latest", "i2", "o2"}, {"nearest", "i2", "o1"}} {
		t.Run(tc.mode, func(t *testing.T) {
			in.Rule.PunchSelection = tc.mode
			r := Calculate(in)
			requireClean(t, r)
			if r.Segments[0].InPunchID != tc.inID || r.Segments[0].OutPunchID != tc.outID {
				t.Fatal(r)
			}
		})
	}
	in.Rule.PunchSelection = "nearest"
	in.Punches[0].At = at("08:58:00")
	r := Calculate(in)
	if !hasIssue(r, "selection_ambiguous") {
		t.Fatal(r)
	}
	// A midpoint punch may not supply two different in boundaries.
	in = baseline()
	in.Shift.Segments = []Segment{{ID: "a", Kind: "work", StartMinute: 540, EndMinute: 570}, {ID: "b", Kind: "work", StartMinute: 600, EndMinute: 630}}
	in.Punches = []Punch{punch("mid", "in", "09:30:00")}
	r = Calculate(in)
	if !hasIssue(r, "punch_ambiguous") {
		t.Fatal(r)
	}
}

func TestExemptionPendingAndRestTemplate(t *testing.T) {
	in := baseline()
	in.Punches = nil
	in.Shift.Segments[0].PunchMode = "none"
	r := Calculate(in)
	requireClean(t, r)
	if r.MissingCount != 0 || r.ActualSeconds != 0 || r.CreditedSeconds != 8*3600 {
		t.Fatal(r)
	}
	in = baseline()
	in.AsOf = at("12:00:00")
	in.Punches = in.Punches[:1]
	r = Calculate(in)
	if r.State != "pending" || r.MissingCount != 0 || r.FullAttendanceEligible {
		t.Fatal(r)
	}
	in = baseline()
	in.Category = Restday
	in.Punches = nil
	r = Calculate(in)
	if r.State != "rest" || r.MissingCount != 0 || r.ExpectedSeconds != 0 {
		t.Fatal(r)
	}
	in.Punches = []Punch{punch("in", "in", "09:00:00"), punch("out", "out", "18:00:00")}
	in.Rule.Overtime = map[Category]OvertimePolicy{Restday: {Enabled: true}}
	r = Calculate(in)
	requireClean(t, r)
	if r.OvertimeSeconds != 8*3600 || r.CreditedSeconds != 0 || r.ExpectedSeconds != 0 {
		t.Fatal(r)
	}
	in.Shift = Shift{}
	r = Calculate(in)
	if !hasIssue(r, "roster_missing") {
		t.Fatal(r)
	}
	in.Punches = nil
	r = Calculate(in)
	if r.State != "rest" {
		t.Fatal(r)
	}
}

func productionInput() Input {
	in := baseline()
	in.Rule.StandardMinutes = 480
	in.Rule.Window = PunchWindow{15, 15, 30, 30}
	in.Rule.Overtime = map[Category]OvertimePolicy{Workday: {Enabled: true}}
	in.Shift = Shift{ID: "production-base", Version: "v1", Segments: []Segment{{ID: "am", Kind: "work", StartMinute: 480, EndMinute: 700}, {ID: "pm", Kind: "work", StartMinute: 790, EndMinute: 1050}}}
	in.Shift.OvertimeVariant = &ShiftVariant{TriggerMinute: 1065, Shift: Shift{ID: "production-ot", Segments: []Segment{{ID: "am", Kind: "work", StartMinute: 480, EndMinute: 700}, {ID: "pm", Kind: "work", StartMinute: 790, EndMinute: 1035}, {ID: "evening", Kind: "overtime", StartMinute: 1065, EndMinute: 1440, OvertimeStartMinute: 1080, OpenEnded: true}}}}
	in.Punches = []Punch{punch("am-in", "in", "08:00:00"), punch("am-out", "out", "11:40:00"), punch("pm-in", "in", "13:10:00"), punch("pm-out", "out", "17:30:00")}
	in.AsOf = stamp("2026-10-30", "03:00:00")
	return in
}

func TestProductionConditionalEveningOvertime(t *testing.T) {
	in := productionInput()
	r := Calculate(in)
	requireClean(t, r)
	if r.ActualSeconds != 480*60 || r.CreditedSeconds != 480*60 || r.EarlyCount != 0 || r.OvertimeSeconds != 0 || len(r.Segments) != 2 {
		t.Fatalf("no overtime day used wrong shift %+v", r)
	}
	for _, tc := range []struct {
		end     string
		minutes int
	}{{"21:00:00", 180}, {"20:59:59", 150}, {"18:20:00", 0}} {
		t.Run(tc.end, func(t *testing.T) {
			in := productionInput()
			in.Punches[3].At = at("17:15:00")
			in.Punches = append(in.Punches, punch("ot-in", "in", "17:45:00"), punch("ot-out", "out", tc.end))
			r := Calculate(in)
			requireClean(t, r)
			if r.OvertimeSeconds != int64(tc.minutes)*60 || r.EarlyCount != 0 || r.CreditedSeconds != 480*60 {
				t.Fatalf("wrong overtime %+v", r)
			}
		})
	}
	in = productionInput()
	in.Punches[3].At = at("17:15:00")
	in.Punches = append(in.Punches, punch("ot-in", "in", "17:40:00"), punch("ot-out", "out", "21:00:00"))
	r = Calculate(in)
	requireClean(t, r)
	if r.OvertimeSeconds != 180*60 || r.MissingCount != 0 {
		t.Fatal(r)
	}
	in.Punches = append(in.Punches[:4], punch("ot-out", "out", "21:00:00"))
	r = Calculate(in)
	if r.State != "review" || !hasIssue(r, "overtime_punch_incomplete") || r.OvertimeSeconds != 0 {
		t.Fatalf("missing evening in invented hours %+v", r)
	}
}

func TestCalculationDoesNotMutateAndIsDeterministic(t *testing.T) {
	in := productionInput()
	in.Punches = append(in.Punches, punch("ot-out", "out", "21:00:00"), punch("ot-in", "in", "17:45:00"))
	before, _ := json.Marshal(in)
	first := Calculate(in)
	for i := 0; i < 30; i++ {
		if !reflect.DeepEqual(first, Calculate(in)) {
			t.Fatal("result or trace order nondeterministic")
		}
	}
	after, _ := json.Marshal(in)
	if string(before) != string(after) {
		t.Fatal("input was mutated")
	}
	if strings.Contains(string(after), "calculated") {
		t.Fatal("source status overwritten")
	}
}

func productionWebPreset(t *testing.T) (Rule, Shift) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required for the web preset integration check")
	}
	module, err := filepath.Abs("../../admin/assets/attendance-v2.js")
	if err != nil {
		t.Fatal(err)
	}
	script := `import {pathToFileURL} from 'node:url'; const {productionFirstDepartmentPreset}=await import(pathToFileURL(process.argv[1])); process.stdout.write(JSON.stringify(productionFirstDepartmentPreset({effective_from:'2026-10-29'})));`
	raw, err := exec.Command(node, "--input-type=module", "-e", script, module).CombinedOutput()
	if err != nil {
		t.Fatalf("web preset: %s %v", raw, err)
	}
	var preset struct {
		Config struct {
			Rule   Rule    `json:"rule"`
			Shifts []Shift `json:"shifts"`
		} `json:"config"`
	}
	if err = json.Unmarshal(raw, &preset); err != nil {
		t.Fatal(err)
	}
	if len(preset.Config.Shifts) == 0 {
		t.Fatal("web preset has no shifts")
	}
	return preset.Config.Rule, preset.Config.Shifts[0]
}

func TestProductionPresetMatchesWebUI(t *testing.T) {
	rule, shift := productionWebPreset(t)
	for _, tc := range []struct {
		end string
		ot  int64
	}{{"", 0}, {"21:00:00", 180}, {"20:59:59", 150}, {"18:20:00", 0}} {
		t.Run("web-"+tc.end, func(t *testing.T) {
			in := productionInput()
			in.Rule = rule
			in.Shift = shift
			if tc.end != "" {
				in.Punches[3].At = at("17:15:00")
				in.Punches = append(in.Punches, punch("ot-in", "in", "17:45:00"), punch("ot-out", "out", tc.end))
			}
			r := Calculate(in)
			requireClean(t, r)
			if r.OvertimeSeconds != tc.ot*60 || r.EarlyCount != 0 || r.MissingCount != 0 {
				t.Fatalf("web preset mismatch %+v", r)
			}
		})
	}
}

func TestProductionWebPresetVariantUsesSeparateInAndOutWindows(t *testing.T) {
	rule, shift := productionWebPreset(t)
	for _, next := range []struct{ at, kind string }{{"07:55:00", "in"}, {"08:00:00", "in"}, {"07:55:00", "unknown"}, {"08:00:00", "unknown"}, {"08:00:00", "out"}} {
		t.Run("normal_day_next_"+next.kind+"_"+next.at, func(t *testing.T) {
			in := productionInput()
			in.Rule, in.Shift = rule, shift
			in.AsOf = stamp("2026-10-30", "12:00:00")
			in.Punches = append(in.Punches, Punch{ID: "next-day-card", Type: next.kind, Status: "normal", At: stamp("2026-10-30", next.at)})
			r := Calculate(in)
			requireClean(t, r)
			if len(r.Segments) != 2 || r.OvertimeSeconds != 0 || r.MissingCount != 0 || r.EarlyCount != 0 || r.ActualSeconds != 480*60 {
				t.Fatalf("next day in switched preceding day to overtime: %+v", r)
			}
		})
	}
	for _, tc := range []struct {
		end     string
		minutes int64
	}{{"02:00:00", 480}, {"06:01:00", 720}} {
		t.Run("valid_overnight_overtime_"+tc.end, func(t *testing.T) {
			in := productionInput()
			in.Rule, in.Shift = rule, shift
			in.AsOf = stamp("2026-10-30", "12:00:00")
			in.Punches[3].At = at("17:15:00")
			in.Punches = append(in.Punches, punch("ot-in", "in", "17:40:00"), Punch{ID: "ot-out", Type: "out", Status: "normal", At: stamp("2026-10-30", tc.end)}, Punch{ID: "next-day-in", Type: "in", Status: "normal", At: stamp("2026-10-30", "08:00:00")})
			r := Calculate(in)
			requireClean(t, r)
			if len(r.Segments) != 3 || r.OvertimeSeconds != tc.minutes*60 || r.MissingCount != 0 || r.EarlyCount != 0 || r.Segments[2].OutPunchID != "ot-out" {
				t.Fatalf("valid overnight evidence lost: %+v", r)
			}
		})
	}
	t.Run("late_out_without_return_requires_review", func(t *testing.T) {
		in := productionInput()
		in.Rule, in.Shift = rule, shift
		in.AsOf = stamp("2026-10-30", "12:00:00")
		in.Punches[3].At = at("17:15:00")
		in.Punches = append(in.Punches, punch("ot-out", "out", "21:00:00"), Punch{ID: "next-day-in", Type: "in", Status: "normal", At: stamp("2026-10-30", "08:00:00")})
		r := Calculate(in)
		if len(r.Segments) != 3 || r.State != "review" || !hasIssue(r, "overtime_punch_incomplete") || r.OvertimeSeconds != 0 {
			t.Fatalf("missing return was accepted or variant evidence discarded: %+v", r)
		}
	})
	t.Run("out_beyond_overtime_window_ignored", func(t *testing.T) {
		in := productionInput()
		in.Rule, in.Shift = rule, shift
		in.AsOf = stamp("2026-10-30", "12:00:00")
		in.Punches = append(in.Punches, Punch{ID: "later-out", Type: "out", Status: "normal", At: stamp("2026-10-30", "08:00:01")})
		r := Calculate(in)
		requireClean(t, r)
		if len(r.Segments) != 2 || r.OvertimeSeconds != 0 || r.MissingCount != 0 || r.EarlyCount != 0 {
			t.Fatalf("unrelated later out switched shift: %+v", r)
		}
	})
}

func TestExemptPunchesStillProvideActualEvidence(t *testing.T) {
	for _, mode := range []string{"none", "in", "out"} {
		in := baseline()
		in.Shift.Segments[0].PunchMode = mode
		r := Calculate(in)
		requireClean(t, r)
		if r.ActualSeconds != 8*3600 || r.MissingCount != 0 {
			t.Fatalf("%s ignored real evidence %+v", mode, r)
		}
	}
}

func TestVerifiedDayLeaveUsesStandardInActualMode(t *testing.T) {
	in := baseline()
	in.Rule.AttendanceMode = "actual"
	in.Rule.StandardMinutes = 450
	in.Punches = nil
	in.Rule.LeavePolicies = map[string]LeavePolicy{"年假": {CreditAttendance: true, FullAttendanceAllowed: true}}
	in.Absences = []Absence{{ID: "dayleave", Kind: "leave", LeaveType: "年假", Status: "approved", Unit: "day", StartAt: at("09:00:00"), EndAt: at("18:00:00"), Slices: []Interval{{StartAt: at("09:00:00"), EndAt: at("18:00:00")}}}}
	r := Calculate(in)
	requireClean(t, r)
	if r.CreditedSeconds != 450*60 || r.LeaveSeconds != 480*60 || r.ActualSeconds != 0 || r.MissingCount != 0 {
		t.Fatalf("day leave did not preserve standard separation %+v", r)
	}
}

func TestScheduledSaturdayWithLateArrivalRetainsIndependentOvertimeEvidence(t *testing.T) {
	in := baseline()
	in.Category = Restday
	in.ScheduledWork = true
	in.Rule.LateArrivalTiers = []LateArrivalTier{{PreviousOvertimeMinutes: 180, DelayMinutes: 30}}
	in.PreviousDate = "2026-10-28"
	in.PreviousOvertimeMinutes = 180
	in.Rule.Overtime = map[Category]OvertimePolicy{Restday: {Enabled: true}}
	in.Shift.Segments = append(in.Shift.Segments, Segment{ID: "overlapping-ot", Kind: "overtime", StartMinute: 540, EndMinute: 1080})
	in.Punches[0].At = at("09:30:00")
	r := Calculate(in)
	requireClean(t, r)
	if r.MissingCount != 0 || r.ActualSeconds != 450*60 || r.OvertimeSeconds != 450*60 || r.CreditedSeconds != 480*60 {
		t.Fatalf("shared evidence stolen by moved boundary %+v", r)
	}
}
