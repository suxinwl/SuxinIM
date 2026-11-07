package attendanceengine

import (
	"sort"
	"time"
)

// Calculate is deterministic: it never consults a clock, database or network,
// and never mutates its input. A review result is provisional, not publishable.
func Calculate(in Input) Result {
	r := Result{EngineVersion: Version, EmployeeID: in.EmployeeID, Date: in.Date, Category: in.Category, RuleVersion: in.Rule.Version, ShiftVersion: in.Shift.Version, State: "calculated", Segments: []SegmentResult{}, Trace: []Trace{}, Issues: []string{}, OvertimeBuckets: []OvertimeBucket{}, FullAttendanceReasons: []string{}}
	date, err := ParseDate(in.Date)
	if err != nil {
		addIssue(&r, "date_invalid", err.Error(), "")
	}
	if err := ValidateRule(in.Rule); err != nil {
		addIssue(&r, "rule_invalid", err.Error(), "")
	}
	if err := ValidateShift(in.Shift); err != nil {
		addIssue(&r, "shift_invalid", err.Error(), "")
	}
	if in.Rule.EvidenceMode == "local" {
		if err := ValidateLocalShift(in.Shift); err != nil {
			addIssue(&r, "local_window_missing", "本地取卡配置待完成："+err.Error(), "")
		}
	}
	if !validCategory(in.Category) {
		addIssue(&r, "category_missing", "必须明确工作日、休息日或节假日", "")
	}
	if in.AsOf <= 0 {
		addIssue(&r, "as_of_missing", "必须提供固定的计算截至时间", "")
	}
	if len(r.Issues) > 0 {
		r.State = "review"
		return r
	}
	in = selectVariant(in, date, &r)
	if len(in.Shift.Segments) == 0 {
		actual := false
		for _, p := range in.Punches {
			if !p.Placeholder && (in.Rule.EvidenceMode == "local" || p.Status != "missing") && p.At >= date.Unix() && p.At < date.AddDate(0, 0, 1).Unix() && p.At <= in.AsOf {
				actual = true
			}
		}
		if in.Category != Workday && !in.ScheduledWork && !actual {
			r.State = "rest"
			addTrace(&r, "rest_day", "已确认休息日且无实际打卡", "", 0)
		} else {
			r.State = "review"
			addIssue(&r, "roster_missing", "缺少已确认班次，不根据打卡猜测班次", "")
		}
		return r
	}
	var e evidence
	in.Adjustments, e = prepareAdjustments(in, &r)
	optional := in.Category != Workday && !in.ScheduledWork
	work, allParts := buildWork(in, date, 0)
	baseEvidence := e
	probeEvidence := e
	probeResult := Result{}
	prepareAbsences(in, allParts, &probeEvidence, &probeResult)
	delay := int64(0)
	if !optional && in.Rule.LateDepartureAfterSecond != nil {
		previous := date.AddDate(0, 0, -1)
		if in.PreviousDate != previous.Format("2006-01-02") || in.PreviousOutAt < 0 || in.PreviousOutAt > in.AsOf || in.PreviousOutAt > 0 && in.PreviousOutAt < previous.Unix() {
			addIssue(&r, "previous_day_unresolved", "晚走次日晚到需要前一工作日期已核实的下班实卡", "")
		} else if in.PreviousOutAt > previous.Unix()+int64(*in.Rule.LateDepartureAfterSecond) {
			delay = int64(in.Rule.LateDepartureDelayMinutes) * 60
			r.LateArrivalMinutes = int(delay / 60)
			addTrace(&r, "late_departure_entitlement", "前日确认下班实卡晚于本地阈值，应用次日允许晚到；不依赖加班核定时长", "", delay)
		}
	}
	if !optional && len(in.Rule.LateArrivalTiers) > 0 {
		if in.PreviousDate != date.AddDate(0, 0, -1).Format("2006-01-02") || in.PreviousOvertimeMinutes < 0 {
			addIssue(&r, "previous_day_unresolved", "晚走次日晚到规则需要上一自然日已核实的加班结果", "")
		} else {
			for _, tier := range in.Rule.LateArrivalTiers {
				if in.PreviousOvertimeMinutes >= tier.PreviousOvertimeMinutes {
					delay = max(delay, int64(tier.DelayMinutes)*60)
				}
			}
			r.LateArrivalMinutes = int(delay / 60)
			if delay > 0 {
				addTrace(&r, "late_arrival_entitlement", "按上一自然日加班档位豁免上班前段，不增加实际工作时长", "", delay)
			}
		}
	}
	// Flex moves the whole shift, including breaks and overtime. Selection still
	// uses configured windows and explicit source types; ambiguity is not guessed.
	flex := int64(0)
	if in.Rule.FlexMinutes > 0 && !optional {
		probe, _ := buildWork(in, date, 0)
		setRequiredBoundaries(probe, probeEvidence, delay)
		tmp := Result{}
		assignPunches(in, probe, &tmp)
		for _, w := range probe {
			if w.segment.Kind == "work" && len(w.parts) > 0 {
				if w.input != nil {
					flex = min(max(w.input.At-w.window.start, 0), int64(in.Rule.FlexMinutes)*60)
				}
				break
			}
		}
		if flex > 0 {
			work, allParts = buildWork(in, date, flex)
			r.FlexAppliedSeconds = flex
			r.FlexAppliedMinutes = int(flex / 60)
			addTrace(&r, "flex_shift", "按允许的弹性上班延后整班，休息及下班时间同步平移", "", flex)
		}
	}
	e = baseEvidence
	prepareAbsences(in, allParts, &e, &r)
	setRequiredBoundaries(work, e, delay)
	assignPunches(in, work, &r)
	allActual := []span{}
	expected := []span{}
	creditParts := [][]span{}
	overtimeParts := [][]span{}
	for _, w := range work {
		if w.segment.Kind == "work" && !optional {
			expected = append(expected, w.parts...)
		}
	}
	r.ExpectedSeconds = duration(expected)
	r.StandardSeconds = r.ExpectedSeconds
	if in.Rule.StandardMinutes > 0 && r.ExpectedSeconds > 0 {
		r.StandardSeconds = int64(in.Rule.StandardMinutes) * 60
	}
	for _, w := range work {
		s, actual, credit := calculateSegment(in, w, e, optional, &r)
		allActual = append(allActual, actual...)
		creditParts = append(creditParts, credit)
		ot := []span{}
		if w.segment.Kind == "overtime" || optional {
			ot = actual
			if w.segment.OvertimeStartMinute > 0 {
				ot = overlap(ot, []span{{date.Unix() + int64(w.segment.OvertimeStartMinute)*60 + flex, date.Unix() + 3*86400}})
			}
		}
		overtimeParts = append(overtimeParts, ot)
		r.Segments = append(r.Segments, s)
	}
	r.ActualSeconds = duration(allActual)
	r.LeaveSeconds = duration(overlap(expected, e.leave))
	r.OutSeconds = duration(overlap(expected, e.out))
	r.TripSeconds = duration(overlap(expected, e.trip))
	r.ConfirmedAbsentSeconds = duration(overlap(expected, e.absent))
	if duration(overlap(e.absent, allActual)) > 0 {
		addIssue(&r, "confirmed_absence_conflict", "人工旷工确认与有效工作证据重叠，请复核", "")
	}
	creditInput := in
	if in.Rule.AttendanceMode == "fixed_complete" {
		if completeCreditWork(work, creditParts, r.Segments) {
			creditInput.Rule.AttendanceMode = "fixed"
			addTrace(&r, "complete_work_credit", "完整正班证据已满足，核定按每日标准工时；实际时长另存不改写", "", r.StandardSeconds)
		} else {
			creditInput.Rule.AttendanceMode = "actual"
			addTrace(&r, "incomplete_work_credit", "正班证据未完整覆盖，不补足标准工时；保留有效实际及已确认假勤", "", 0)
		}
	}
	applyCredit(creditInput, creditParts, e, &r)
	checkPartialWork(in, work, allActual, &r)
	applyCompleteWorkDeduction(in, work, allActual, &r)
	calculateOvertime(in, work, overtimeParts, e.overtime, &r)
	for _, s := range r.Segments {
		r.RawLateSeconds += s.RawLateSeconds
		r.RawEarlySeconds += s.RawEarlySeconds
		r.LateSeconds += s.LateSeconds
		r.EarlySeconds += s.EarlySeconds
		r.GraceSeconds += s.GraceSeconds
		r.MissingCount += s.MissingCount
		if s.LateSeconds > 0 {
			r.LateCount++
		}
		if s.EarlySeconds > 0 {
			r.EarlyCount++
		}
		if s.State == "pending" {
			r.State = "pending"
		}
	}
	if in.Rule.EvidenceMode == "local" {
		r.MissingCount = countLocalMissing(in, work, r.Segments)
	}
	if optional && r.ActualSeconds == 0 && r.MissingCount == 0 {
		r.State = "rest"
	}
	if len(r.Issues) > 0 {
		r.State = "review"
	}
	fullAttendance(in, e, expected, &r)
	addTrace(&r, "actual_union", "实际工作按有证据的工作及加班区间并集计量，扣休息和假勤，不重复累计", "", r.ActualSeconds)
	addTrace(&r, "separate_metrics", "实际工作、核定出勤与加班为独立指标，不相加为总工时", "", r.CreditedSeconds)
	return r
}

func buildWork(in Input, date time.Time, offset int64) ([]segmentWork, []span) {
	segments := append([]Segment(nil), in.Shift.Segments...)
	sort.SliceStable(segments, func(i, j int) bool {
		if segments[i].StartMinute == segments[j].StartMinute {
			return segments[i].ID < segments[j].ID
		}
		return segments[i].StartMinute < segments[j].StartMinute
	})
	breaks := []span{}
	for _, s := range segments {
		if s.Kind == "break" {
			breaks = append(breaks, span{date.Unix() + int64(s.StartMinute)*60 + offset, date.Unix() + int64(s.EndMinute)*60 + offset})
		}
	}
	work := []segmentWork{}
	all := []span{}
	for _, s := range segments {
		if s.Kind == "break" {
			continue
		}
		window := in.Rule.Window
		if s.Window != nil {
			window = *s.Window
		}
		period := span{date.Unix() + int64(s.StartMinute)*60 + offset, date.Unix() + int64(s.EndMinute)*60 + offset}
		parts := subtract([]span{period}, breaks)
		all = append(all, parts...)
		work = append(work, segmentWork{segment: s, window: period, parts: parts, windowRule: window})
	}
	return work, union(all)
}

// Approved absences at the edge change the expected punch time; internal
// absences remove duration without inventing additional punch obligations.
func setRequiredBoundaries(work []segmentWork, e evidence, delay int64) {
	first := true
	for i := range work {
		w := &work[i]
		required := subtract(w.parts, e.attendance())
		if w.segment.Kind == "work" {
			required = subtract(required, e.absent)
		}
		if w.segment.Kind == "work" && first && len(w.parts) > 0 {
			first = false
			if delay > 0 {
				required = subtract(required, []span{{w.parts[0].start, w.parts[0].start + delay}})
			}
		}
		if len(required) > 0 {
			w.window.start, w.window.end = required[0].start, required[len(required)-1].end
		} else {
			w.segment.PunchMode = "none"
		}
	}
}

func calculateSegment(in Input, w segmentWork, e evidence, optional bool, r *Result) (SegmentResult, []span, []span) {
	s := SegmentResult{ID: w.segment.ID, Kind: w.segment.Kind, StartAt: w.window.start, EndAt: w.window.end, State: "calculated", Issues: []string{}}
	work := w.segment.Kind == "work" && !optional
	if work {
		s.ExpectedSeconds = duration(w.parts)
		s.LeaveSeconds = duration(overlap(w.parts, e.leave))
		s.OutSeconds = duration(overlap(w.parts, e.out))
		s.TripSeconds = duration(overlap(w.parts, e.trip))
		s.ConfirmedAbsentSeconds = duration(overlap(w.parts, e.absent))
	}
	if w.input != nil {
		s.InPunchID = w.input.ID
	}
	if w.output != nil {
		s.OutPunchID = w.output.ID
	}
	actual := []span{}
	if w.input != nil && w.output != nil {
		if w.output.At <= w.input.At {
			addIssue(r, "punch_order_invalid", "选取的下班卡不晚于上班卡", s.ID, w.input.ID, w.output.ID)
			s.Issues = append(s.Issues, "punch_order_invalid")
		} else {
			actual = overlap(w.parts, []span{{w.input.At, w.output.At}})
			actual = subtract(actual, e.attendance())
		}
	}
	s.ActualSeconds = duration(actual)
	if optional && w.input == nil && w.output == nil {
		if w.issue {
			s.State = "review"
		}
		return s, actual, nil
	}
	mode := punchMode(w.segment)
	if in.Rule.EvidenceMode == "local" {
		mode = localPunchMode(in, w.segment)
	}
	required := subtract(w.parts, e.attendance())
	if work {
		required = subtract(required, e.absent)
	}
	if len(required) > 0 {
		// The window start also includes an explicit late-arrival entitlement.
		required = overlap(required, []span{w.window})
	}
	needsIn := (mode == "both" || mode == "in") && len(required) > 0
	needsOut := (mode == "both" || mode == "out") && len(required) > 0
	closed := in.AsOf >= w.window.end+int64(w.windowRule.OutAfterMinutes)*60
	if in.Rule.EvidenceMode == "local" {
		closed = in.AsOf >= w.window.end
		if b, ok := localBoundaryFor(in, w, "out"); ok && boundaryRequired(mode, "out") {
			closed = in.AsOf >= b.window.end
		}
	}
	if w.segment.OpenEnded && w.output != nil && in.Rule.EvidenceMode != "local" {
		closed = true
	}
	if !closed {
		s.State = "pending"
	}
	if closed {
		if needsIn && w.input == nil {
			s.MissingCount++
		}
		if needsOut && w.output == nil {
			s.MissingCount++
		}
	}
	if e.waived("waive_missing", s.ID) {
		s.MissingCount = 0
	}
	if w.input != nil && needsIn {
		s.RawLateSeconds = max(w.input.At-w.window.start, 0)
	}
	if w.output != nil && needsOut && !w.segment.OpenEnded {
		s.RawEarlySeconds = max(w.window.end-w.output.At, 0)
	}
	voluntaryRestday := optional && in.Category == Restday && in.Rule.Overtime[Restday].CompleteWorkdayMinutes > 0
	if in.Rule.EvidenceMode == "local" && w.segment.Kind == "overtime" {
		if s.RawLateSeconds > 0 || s.RawEarlySeconds > 0 {
			addTrace(r, "local_overtime_actual_boundary", "加班按有效实际出勤核算，不将加班区间不足记为正班迟到早退", s.ID, s.RawLateSeconds+s.RawEarlySeconds)
		}
	} else if voluntaryRestday {
		if s.RawLateSeconds > 0 || s.RawEarlySeconds > 0 {
			addTrace(r, "voluntary_restday_boundary", "自愿休息日按有效实际出勤核算，不计迟到早退处罚；原始边界差仅供核对且不补足整班", s.ID, s.RawLateSeconds+s.RawEarlySeconds)
		}
	} else {
		if s.RawLateSeconds > int64(in.Rule.LateGraceMinutes)*60 {
			s.LateSeconds = s.RawLateSeconds
		} else if in.Rule.GraceMode == "grace" {
			s.GraceSeconds += s.RawLateSeconds
		}
		if s.RawEarlySeconds > int64(in.Rule.EarlyGraceMinutes)*60 {
			s.EarlySeconds = s.RawEarlySeconds
		} else if in.Rule.GraceMode == "grace" {
			s.GraceSeconds += s.RawEarlySeconds
		}
	}
	if e.waived("waive_late", s.ID) {
		s.LateSeconds = 0
	}
	if e.waived("waive_early", s.ID) {
		s.EarlySeconds = 0
	}
	credit := []span{}
	if work {
		credit = append(credit, e.paidLeave...)
		if in.Rule.CreditOut {
			credit = append(credit, e.out...)
		}
		if in.Rule.CreditTrip {
			credit = append(credit, e.trip...)
		}
		// A shift start moved by the previous day's overtime is an explicit
		// credited exemption, independent of measured presence.
		entitlement := subtract(w.parts, append(e.attendance(), required...))
		entitlement = subtract(entitlement, e.absent)
		credit = append(credit, entitlement...)
		complete := (!needsIn || w.input != nil) && (!needsOut || w.output != nil) || e.waived("waive_missing", s.ID)
		if in.Rule.AttendanceMode == "fixed" {
			if complete {
				credit = append(credit, required...)
			}
		} else {
			credit = append(credit, actual...)
			// Explicit no/one-sided punching waives proof at that boundary for
			// credited attendance only; it never manufactures actual work.
			if mode == "none" {
				credit = append(credit, required...)
			}
			if mode == "in" && w.input != nil {
				credit = append(credit, overlap(required, []span{{w.input.At, w.window.end}})...)
			}
			if mode == "out" && w.output != nil {
				credit = append(credit, overlap(required, []span{{w.window.start, w.output.At}})...)
			}
		}
		credit = append(credit, e.credit...)
		credit = subtract(overlap(w.parts, credit), e.absent)
		if !closed {
			credit = overlap(credit, []span{{1, in.AsOf}})
		}
	}
	if s.MissingCount > 0 {
		addTrace(r, "missing_not_absence", "缺卡仅标记证据不完整，不自动判定旷工", s.ID, 0)
		if in.Rule.EvidenceMode == "local" {
			addIssue(r, "punch_incomplete", "本地班段缺少必要实卡，待复核且不自动判旷工", s.ID)
			s.Issues = append(s.Issues, "punch_incomplete")
		}
	}
	if s.MissingCount > 0 && (w.segment.Kind == "overtime" || optional) && (w.input != nil || w.output != nil) {
		addIssue(r, "overtime_punch_incomplete", "加班打卡不完整，待人工复核，未推定工作时长", s.ID)
		s.Issues = append(s.Issues, "overtime_punch_incomplete")
	}
	if w.issue || len(s.Issues) > 0 {
		s.State = "review"
	}
	return s, actual, credit
}

func deduction(raw int64, grace int, mode string, waived bool) int64 {
	if waived || raw <= int64(grace)*60 {
		return 0
	}
	switch mode {
	case "full":
		return raw
	case "excess":
		return raw - int64(grace)*60
	}
	return 0
}

func applyCredit(in Input, parts [][]span, e evidence, r *Result) {
	var accrued, allocated int64
	var dayAccrued, dayAllocated int64
	for i := range r.Segments {
		s := &r.Segments[i]
		value := duration(parts[i])
		if in.Rule.AttendanceMode == "fixed" && r.ExpectedSeconds > 0 {
			// Cumulative proportional allocation avoids rounding drift across
			// multiple work segments and uses the day's declared standard.
			accrued += value
			target := accrued * r.StandardSeconds / r.ExpectedSeconds
			value = target - allocated
			allocated = target
			value = max(0, value-deduction(s.RawLateSeconds, in.Rule.LateGraceMinutes, in.Rule.LateDeductionMode, s.LateSeconds == 0)-deduction(s.RawEarlySeconds, in.Rule.EarlyGraceMinutes, in.Rule.EarlyDeductionMode, s.EarlySeconds == 0))
		} else if r.ExpectedSeconds > 0 {
			dayPhysical := duration(overlap(parts[i], e.dayUnitCredit))
			dayAccrued += dayPhysical
			target := dayAccrued * r.StandardSeconds / r.ExpectedSeconds
			value = value - dayPhysical + (target - dayAllocated)
			dayAllocated = target
		}
		s.CreditedSeconds = value
		r.CreditedSeconds += value
	}
	if dayAccrued > 0 {
		addTrace(r, "day_unit_standard_credit", "核实后的天/半天假勤区间按其覆盖的班段比例与当日标准工时折算；不假定上午、下午等长", "", dayAllocated)
	}
	if in.Rule.AttendanceMode == "fixed" {
		addTrace(r, "fixed_credit", "核定工时按当日标准与获准区间比例折算，迟早扣减另按显式规则执行", "", r.StandardSeconds)
	} else {
		addTrace(r, "actual_credit", "核定以实际工作为基础，加显式获准的假勤、免卡及人工核定区间，不重复扣减迟早分钟", "", r.CreditedSeconds)
	}
}

func fullAttendance(in Input, e evidence, expected []span, r *Result) {
	reasons := []string{}
	if r.State == "review" {
		reasons = append(reasons, "存在待复核项目")
	}
	if r.State == "pending" {
		reasons = append(reasons, "班段尚未结束或取卡窗口尚未关闭")
	}
	if r.ExpectedSeconds == 0 {
		reasons = append(reasons, "本日无应出勤工作段")
	}
	if r.LateCount > 0 && !in.Rule.FullAttendance.AllowLate {
		reasons = append(reasons, "迟到影响全勤")
	}
	if r.EarlyCount > 0 && !in.Rule.FullAttendance.AllowEarly {
		reasons = append(reasons, "早退影响全勤")
	}
	if r.MissingCount > 0 && !in.Rule.FullAttendance.AllowMissing {
		reasons = append(reasons, "缺卡影响全勤")
	}
	if r.ConfirmedAbsentSeconds > 0 {
		reasons = append(reasons, "存在人工确认旷工")
	}
	for _, a := range e.absences {
		spans, _ := absenceSpans(a)
		if duration(overlap(spans, expected)) == 0 {
			continue
		}
		allowed := false
		switch a.Kind {
		case "leave":
			allowed = in.Rule.LeavePolicies[a.LeaveType].FullAttendanceAllowed
			for _, kind := range in.Rule.FullAttendance.AllowedLeaveTypes {
				allowed = allowed || kind == a.LeaveType
			}
		case "out":
			allowed = in.Rule.FullAttendance.AllowOut
		case "trip":
			allowed = in.Rule.FullAttendance.AllowTrip
		}
		if !allowed {
			reasons = appendUnique(reasons, "假勤影响全勤："+a.Kind+" "+a.LeaveType)
		}
	}
	r.FullAttendanceReasons = reasons
	r.FullAttendanceEligible = len(reasons) == 0
}
