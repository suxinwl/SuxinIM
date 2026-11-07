package hrm

import (
	"bytes"
	"strings"
	"testing"
	"time"

	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
)

func localSourceFixture(t *testing.T) ImportPreview {
	t.Helper()
	p, err := ParseAttendanceWorkbook(bytes.NewReader(attendanceWorkbookFixture(t, "部门上班时间.xlsx")), "部门上班时间.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func localSourceFind(t *testing.T, p ImportPreview, match func(ImportRuleDraft) bool) ImportRuleDraft {
	t.Helper()
	for _, r := range p.Rules {
		if match(r) {
			return r
		}
	}
	t.Fatal("required workbook source row was not imported")
	return ImportRuleDraft{}
}
func localSourceMinutes(s eng.Shift, kind string) int {
	n := 0
	for _, p := range s.Segments {
		if p.Kind == kind {
			n += p.EndMinute - p.StartMinute
		}
	}
	return n
}
func localSourceWindow(a, b, c, d int) *eng.LocalPunchWindows {
	return &eng.LocalPunchWindows{InStartSecond: &a, InEndSecond: &b, OutStartSecond: &c, OutEndSecond: &d}
}
func localSourceAt(date string, seconds int) int64 {
	t, _ := time.ParseInLocation("2006-01-02", date, eng.Location)
	return t.Unix() + int64(seconds)
}
func localSourcePunch(id string, seconds int) eng.Punch {
	return eng.Punch{ID: id, At: localSourceAt("2026-10-31", seconds), Type: "outside", Status: "时间异常"}
}
func localSourceInput(config AttConfig, shift eng.Shift) eng.Input {
	return eng.Input{EmployeeID: "fixture-only", Date: "2026-10-31", Category: eng.Workday, AsOf: localSourceAt("2026-11-03", 0), Rule: config.Rule, Shift: shift}
}

func TestLocalSourceQualityGroupsPreserveDistinctClockTimes(t *testing.T) {
	p := localSourceFixture(t)
	for _, tc := range []struct{ row, amEnd, pmStart, variantEnd, otStart int }{{11, 710, 800, 1050, 1080}, {12, 690, 780, 1050, 1080}, {13, 700, 790, 1035, 1065}} {
		source := localSourceFind(t, p, func(r ImportRuleDraft) bool {
			return r.Category == "员工" && r.DirectoryDepartment == "品质部" && r.Row == tc.row
		})
		config, _ := localSourceConfig(source)
		if len(config.Shifts) != 1 {
			t.Fatalf("quality row %d lost its personal scope schedule: %+v", tc.row, config.Shifts)
		}
		s := config.Shifts[0]
		if s.Segments[0].EndMinute != tc.amEnd || s.Segments[1].StartMinute != tc.pmStart || s.Segments[1].EndMinute != 1050 || s.OvertimeVariant == nil {
			t.Fatalf("quality normal row %d %+v", tc.row, s)
		}
		v := s.OvertimeVariant.Shift
		if v.Segments[1].EndMinute != tc.variantEnd || v.Segments[2].Kind != "overtime" || v.Segments[2].StartMinute != tc.otStart {
			t.Fatalf("quality variant row %d %+v", tc.row, v)
		}
		if tc.row == 13 && (v.Segments[2].OvertimeStartMinute != 1080 || !strings.Contains(source.Fields["D"].Value, "17:15")) {
			t.Fatal("confirmed correction lost original cell or 18:00 overtime threshold")
		}
		if config.Rule.StandardMinutes != 480 || config.Rule.AttendanceMode != "fixed_complete" {
			t.Fatal("production standard must be independently credited as eight hours")
		}
	}
}

func TestLocalSourceMissingWindowsRemainUnconfigured(t *testing.T) {
	p := localSourceFixture(t)
	source := localSourceFind(t, p, func(r ImportRuleDraft) bool {
		return r.Category == "员工" && r.DirectoryDepartment == "组装事业部"
	})
	config, _ := localSourceConfig(source)
	for _, s := range config.Shifts {
		if eng.ValidateLocalShift(s) == nil {
			t.Fatal("source with no window limits was silently published with defaults")
		}
		for _, segment := range s.Segments {
			if w := segment.LocalWindows; w != nil && (w.InStartSecond != nil || w.InEndSecond != nil || w.OutStartSecond != nil || w.OutEndSecond != nil) {
				t.Fatalf("invented punch window %+v", w)
			}
		}
		result := eng.Calculate(localSourceInput(config, s))
		if result.State != "review" || result.ActualSeconds != 0 || result.LateSeconds != 0 || result.EarlySeconds != 0 {
			t.Fatalf("unconfigured import claimed attendance %+v", result)
		}
	}
	staff := localSourceFind(t, p, func(r ImportRuleDraft) bool {
		return r.Category == "职员" && strings.Contains(r.CheckinText, "12") && strings.Contains(r.CheckinText, "40") && strings.Contains(r.ScheduleText, "13")
	})
	c, _ := localSourceConfig(staff)
	found := false
	for _, s := range c.Shifts {
		for _, segment := range s.Segments {
			if segment.StartMinute == 810 && segment.LocalWindows != nil && segment.LocalWindows.InStartSecond != nil {
				found = *segment.LocalWindows.InStartSecond == 12*3600+40*60
				if segment.LocalWindows.InEndSecond != nil {
					t.Fatal("hint supplied earliest only; latest must remain null")
				}
			}
		}
	}
	if !found {
		t.Fatal("explicit 12:40 source boundary was not retained")
	}
}

func TestLocalSourceBigSmallWeekUsesConfirmedAnchor(t *testing.T) {
	p := localSourceFixture(t)
	source := localSourceFind(t, p, func(r ImportRuleDraft) bool {
		return r.Category == "职员" && strings.Contains(r.Remarks, "大小周")
	})
	config, _ := localSourceConfig(source)
	if config.Cycle.Mode != "cycle" || config.Cycle.Anchor != "2026-10-05" || len(config.Cycle.Days) != 14 {
		t.Fatalf("anchor was guessed from current weekday %+v", config.Cycle)
	}
	first, err := attCycleDay(config, "2026-10-10")
	if err != nil {
		t.Fatal(err)
	}
	second, err := attCycleDay(config, "2026-10-17")
	if err != nil {
		t.Fatal(err)
	}
	if first.ShiftID == "" || second.ShiftID != "" {
		t.Fatalf("first/second Saturday lost explicit cycle: %+v %+v", first, second)
	}
}

func TestLocalSourceSlicingDayNightKeepContinuousEvidence(t *testing.T) {
	p := localSourceFixture(t)
	sources := []ImportRuleDraft{}
	for _, r := range p.Rules {
		if r.Category == "员工" && r.DirectoryDepartment == "分切事业部" {
			sources = append(sources, r)
		}
	}
	if len(sources) != 2 {
		t.Fatalf("expected white/night source rows, got %d", len(sources))
	}
	config, _ := localGroupConfig(sources)
	if len(config.Shifts) != 2 || config.ShiftSelection != "auto" {
		t.Fatal("white/night must be explicit candidates")
	}
	for _, s := range config.Shifts {
		if localSourceMinutes(s, "work") != 480 || s.OvertimeVariant == nil {
			t.Fatalf("normal slicing eight-hour rule %+v", s)
		}
		if err := eng.ValidateShift(s); err != nil {
			t.Fatal(err)
		}
		if s.Name == "夜班" {
			v := s.OvertimeVariant.Shift
			if localSourceMinutes(v, "work") != 480 || localSourceMinutes(v, "overtime") != 240 || len(v.EvidencePeriods) != 1 {
				t.Fatalf("night shift should be 8+4 with one shared period %+v", v)
			}
			period := v.EvidencePeriods[0]
			if period.StartSecond != 23*3600+30*60 || period.EndSecond != 28*3600+30*60 {
				t.Fatalf("night continuous evidence incorrectly cut at 03:30 %+v", period)
			}
			shared := 0
			for _, seg := range v.Segments {
				if seg.EvidenceID == period.ID {
					shared++
				}
			}
			if shared != 2 {
				t.Fatal("03:30 is a metric split, not another punch obligation")
			}
			s.Segments[0].LocalWindows = localSourceWindow(17*3600, 21*3600, 21*3600, 23*3600+15*60)
			s.Segments[1].LocalWindows = localSourceWindow(23*3600+15*60, 24*3600+30*60, 27*3600, 28*3600+45*60)
			s.OvertimeVariant.Shift.Segments[0].LocalWindows = s.Segments[0].LocalWindows
			s.OvertimeVariant.Shift.EvidencePeriods[0].LocalWindows = s.Segments[1].LocalWindows
			last := len(s.OvertimeVariant.Shift.Segments) - 1
			s.OvertimeVariant.Shift.Segments[last].LocalWindows = localSourceWindow(28*3600+45*60, 30*3600, 30*3600, 33*3600)
			in := localSourceInput(config, s)
			in.Category = eng.Restday
			in.Punches = []eng.Punch{localSourcePunch("a", 19*3600), localSourcePunch("b", 23*3600), localSourcePunch("c", 23*3600+30*60), localSourcePunch("d", 28*3600+30*60), localSourcePunch("e", 29*3600), localSourcePunch("f", 32*3600)}
			result := eng.Calculate(in)
			if result.State != "calculated" || result.ActualSeconds != 12*3600 || result.OvertimeSeconds != 12*3600 || len(result.PunchAssignments) != 6 {
				t.Fatalf("imported night candidate failed real six-card evidence %+v", result)
			}
		} else if s.Name == "白班" {
			s.Segments[0].LocalWindows = localSourceWindow(6*3600, 10*3600, 10*3600, 12*3600)
			s.Segments[1].LocalWindows = localSourceWindow(12*3600, 15*3600, 15*3600, 17*3600+45*60)
			s.OvertimeVariant.Shift.Segments[0].LocalWindows = s.Segments[0].LocalWindows
			s.OvertimeVariant.Shift.Segments[1].LocalWindows = s.Segments[1].LocalWindows
			s.OvertimeVariant.Shift.Segments[2].LocalWindows = localSourceWindow(17*3600+45*60, 19*3600, 19*3600, 24*3600)
			for _, hour := range []int{20, 21} {
				in := localSourceInput(config, s)
				in.Category = eng.Restday
				in.Punches = []eng.Punch{localSourcePunch("a", 8*3600), localSourcePunch("b", 11*3600+30*60), localSourcePunch("c", 13*3600), localSourcePunch("d", 17*3600+30*60), localSourcePunch("e", 18*3600), localSourcePunch("f", hour*3600)}
				result := eng.Calculate(in)
				want := int64((hour - 10) * 3600)
				if result.State != "calculated" || result.ActualSeconds != want || result.OvertimeSeconds != want {
					t.Fatalf("slicing white actual finish %d must yield %d seconds: %+v", hour, want, result)
				}
			}
		}
	}
}

func TestLocalSourceInjectionNetHoursAndPartialMealPolicy(t *testing.T) {
	p := localSourceFixture(t)
	for _, night := range []bool{false, true} {
		source := localSourceFind(t, p, func(r ImportRuleDraft) bool {
			return r.Category == "员工" && r.DirectoryDepartment == "注塑事业部" && strings.Contains(r.ScheduleText, "夜班") == night
		})
		config, _ := localSourceConfig(source)
		if config.Rule.CompleteWorkDeductionMinutes != 30 || config.Rule.PartialWorkPolicy == nil || config.Rule.PartialWorkPolicy.Mode != "review" || len(config.Rule.PartialWorkPolicy.ActualRanges) != 1 {
			t.Fatal("injection meal must be explicit and only known morning exempt")
		}
		s := config.Shifts[0]
		if s.OvertimeVariant == nil {
			t.Fatal("injection long shift not imported")
		}
		start := 8 * 3600
		if night {
			start = 20 * 3600
		}
		// Test-only windows are explicit test inputs, never importer defaults.
		for i := range s.Segments {
			s.Segments[i].LocalWindows = localSourceWindow(start-7200, start+7200, start+7200, start+14*3600)
		}
		for i := range s.OvertimeVariant.Shift.EvidencePeriods {
			s.OvertimeVariant.Shift.EvidencePeriods[i].LocalWindows = localSourceWindow(start-7200, start+7200, start+7200, start+14*3600)
		}
		for _, end := range []int{start + 8*3600 + 30*60, start + 12*3600} {
			in := localSourceInput(config, s)
			in.Punches = []eng.Punch{localSourcePunch("in", start), localSourcePunch("out", end)}
			r := eng.Calculate(in)
			wantOT := int64(0)
			if end == start+12*3600 {
				wantOT = 210 * 60
			}
			if r.State != "calculated" || r.CreditedSeconds != 480*60 || r.OvertimeSeconds != wantOT || r.DeductedSeconds != 30*60 || r.ActualSeconds != int64(end-start-30*60) {
				t.Fatalf("injection %s result %+v", s.Name, r)
			}
		}
		if !night {
			in := localSourceInput(config, s)
			in.Category = eng.Restday
			in.Punches = []eng.Punch{localSourcePunch("in", 8*3600), localSourcePunch("out", 12*3600)}
			r := eng.Calculate(in)
			if r.State != "calculated" || r.ActualSeconds != 4*3600 || r.DeductedSeconds != 0 || r.OvertimeSeconds != 4*3600 {
				t.Fatalf("08-12 must not invent lunch %+v", r)
			}
			in.Punches[1] = localSourcePunch("out", 14*3600)
			r = eng.Calculate(in)
			if r.State != "review" || r.DeductedSeconds != 0 {
				t.Fatalf("unknown partial meal silently deducted %+v", r)
			}
		}
	}
}

func TestLocalSourceProductionStaffCapsOvertimeNotActual(t *testing.T) {
	p := localSourceFixture(t)
	source := localSourceFind(t, p, func(r ImportRuleDraft) bool {
		return r.Category == "生产职员" && strings.Contains(r.NamesText, "邱雪梅")
	})
	config, _ := localSourceConfig(source)
	if config.Rule.Overtime[eng.Workday].MaximumMinutes != 120 {
		t.Fatal("weekday fixed overtime cap lost")
	}
	s := config.Shifts[0]
	if len(s.Segments) != 3 || s.Segments[2].Kind != "overtime" || !s.Segments[2].OpenEnded {
		t.Fatalf("night overtime must keep actual finish evidence %+v", s)
	}
	s.Segments[0].LocalWindows = localSourceWindow(6*3600, 10*3600, 10*3600, 12*3600+20*60)
	s.Segments[1].LocalWindows = localSourceWindow(12*3600+20*60, 15*3600, 15*3600, 17*3600+45*60)
	s.Segments[2].LocalWindows = localSourceWindow(17*3600+45*60, 18*3600+30*60, 18*3600+30*60, 24*3600)
	in := localSourceInput(config, s)
	in.Punches = []eng.Punch{localSourcePunch("a", 8*3600), localSourcePunch("b", 11*3600+50*60), localSourcePunch("c", 13*3600+20*60), localSourcePunch("d", 17*3600+30*60), localSourcePunch("e", 18*3600), localSourcePunch("f", 22*3600)}
	r := eng.Calculate(in)
	if r.State != "calculated" || r.ActualSeconds != 12*3600 || r.CreditedSeconds != 8*3600 || r.OvertimeSeconds != 2*3600 {
		t.Fatalf("production staff actual overtime hidden %+v", r)
	}
	if config.Cycle.Weekdays[5].Category != eng.Workday || config.Cycle.Weekdays[5].ShiftID != "saturday" {
		t.Fatal("production staff Saturday must retain its own source schedule")
	}
}

func TestLocalSourceOfficeTwoCardsAndLateDepartureThreshold(t *testing.T) {
	p := localSourceFixture(t)
	two := localSourceFind(t, p, func(r ImportRuleDraft) bool {
		return r.Category == "职员" && strings.Contains(r.Remarks, "只打两个卡")
	})
	config, _ := localSourceConfig(two)
	s := config.Shifts[0]
	if len(s.EvidencePeriods) != 1 {
		t.Fatal("two-card office schedule requires one continuous evidence period")
	}
	for _, segment := range s.Segments {
		if segment.EvidenceID != s.EvidencePeriods[0].ID {
			t.Fatal("lunch must not invent two extra physical punches")
		}
	}
	late := localSourceFind(t, p, func(r ImportRuleDraft) bool {
		return r.Category == "职员" && (strings.Contains(r.Remarks, "21:30") || strings.Contains(r.Remarks, "21：30"))
	})
	config, _ = localSourceConfig(late)
	if config.Rule.LateDepartureAfterSecond == nil || *config.Rule.LateDepartureAfterSecond != 21*3600+30*60 || config.Rule.LateDepartureDelayMinutes != 60 {
		t.Fatal("late departure must depend on actual exit, not overtime credit")
	}
}
