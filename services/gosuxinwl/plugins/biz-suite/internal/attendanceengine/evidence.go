package attendanceengine

import (
	"reflect"
	"sort"
	"strings"
)

type evidence struct {
	leave, out, trip, overtime, paidLeave, credit, absent []span
	dayUnitCredit                                         []span
	absences                                              []Absence
	waivers                                               map[string]map[string]bool
}

func (e evidence) attendance() []span {
	return union(append(append(append([]span{}, e.leave...), e.out...), e.trip...))
}
func (e evidence) waived(kind, segment string) bool {
	return e.waivers[kind][""] || e.waivers[kind][segment]
}

// Only approved, auditable adjustments participate. Returning a filtered copy
// also prevents invalid exclude_punch requests from suppressing real evidence.
func prepareAdjustments(in Input, r *Result) ([]Adjustment, evidence) {
	e := evidence{waivers: map[string]map[string]bool{}}
	valid := []Adjustment{}
	seen := map[string]Adjustment{}
	conflicts := map[string]bool{}
	for _, a := range in.Adjustments {
		if old, ok := seen[a.ID]; ok && old != a {
			conflicts[a.ID] = true
		}
		seen[a.ID] = a
	}
	keys := []string{}
	for id := range seen {
		keys = append(keys, id)
	}
	sort.Strings(keys)
	for _, id := range keys {
		a := seen[id]
		if conflicts[id] {
			addIssue(r, "adjustment_identity_conflict", "同一人工调整标识内容冲突", "", id)
			continue
		}
		if a.Status != "approved" {
			continue
		}
		if strings.TrimSpace(a.ID) == "" || strings.TrimSpace(a.Reason) == "" {
			addIssue(r, "adjustment_audit_missing", "已批准调整必须有标识及理由", "", a.ID)
			continue
		}
		if !enum(a.Kind, "credit", "absence", "waive_late", "waive_early", "waive_missing", "exclude_punch") {
			addIssue(r, "adjustment_kind_invalid", "无法识别的人工调整类型", "", a.ID)
			continue
		}
		if a.SegmentID != "" {
			found := false
			for _, s := range in.Shift.Segments {
				found = found || s.ID == a.SegmentID
			}
			if !found {
				addIssue(r, "adjustment_segment_invalid", "人工调整对应的班段不存在", "", a.ID)
				continue
			}
		}
		switch a.Kind {
		case "credit", "absence":
			if a.StartAt <= 0 || a.EndAt <= a.StartAt {
				addIssue(r, "adjustment_interval_invalid", "人工核定或旷工确认必须有有效起止时间", "", a.ID)
				continue
			}
			if a.Kind == "credit" {
				e.credit = append(e.credit, span{a.StartAt, a.EndAt})
			} else {
				e.absent = append(e.absent, span{a.StartAt, a.EndAt})
			}
		case "exclude_punch":
			found := false
			for _, p := range in.Punches {
				found = found || p.ID == a.PunchID
			}
			if !found || a.PunchID == "" {
				addIssue(r, "adjustment_punch_invalid", "排除打卡的调整未关联有效来源卡", "", a.ID)
				continue
			}
		default:
			if e.waivers[a.Kind] == nil {
				e.waivers[a.Kind] = map[string]bool{}
			}
			e.waivers[a.Kind][a.SegmentID] = true
		}
		valid = append(valid, a)
		addTrace(r, "manual_adjustment", a.Reason, a.SegmentID, 0, evidenceID("adjustment", a.ID))
	}
	if duration(overlap(e.credit, e.absent)) > 0 {
		addIssue(r, "adjustment_overlap", "人工核定出勤与旷工确认区间冲突", "")
	}
	return valid, e
}

func absenceSpans(a Absence) ([]span, bool) {
	if !enum(a.Unit, "", "hour", "day", "halfday") {
		return nil, false
	}
	if (a.Unit == "day" || a.Unit == "halfday") && len(a.Slices) == 0 {
		return nil, false
	}
	if len(a.Slices) > 0 {
		v := []span{}
		for _, s := range a.Slices {
			if s.StartAt <= 0 || s.EndAt <= s.StartAt {
				return nil, false
			}
			if a.StartAt > 0 && s.StartAt < a.StartAt || a.EndAt > 0 && s.EndAt > a.EndAt {
				return nil, false
			}
			v = append(v, span{s.StartAt, s.EndAt})
		}
		return union(v), true
	}
	if a.StartAt <= 0 || a.EndAt <= a.StartAt {
		return nil, false
	}
	return []span{{a.StartAt, a.EndAt}}, true
}

func prepareAbsences(in Input, work []span, e *evidence, r *Result) {
	seen := map[string]Absence{}
	conflicts := map[string]bool{}
	for _, a := range in.Absences {
		if old, ok := seen[a.ID]; ok && !reflect.DeepEqual(old, a) {
			conflicts[a.ID] = true
		}
		seen[a.ID] = a
	}
	ids := []string{}
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	attendance := []span{}
	for _, id := range ids {
		a := seen[id]
		if conflicts[id] {
			addIssue(r, "approval_identity_conflict", "同一审批标识内容或状态冲突，请先合并最新状态", "", id)
			continue
		}
		if a.Status != "approved" {
			continue
		}
		if strings.TrimSpace(a.ID) == "" || !enum(a.Kind, "leave", "out", "trip", "overtime") {
			addIssue(r, "approval_invalid", "审批标识或业务类型无效", "", a.ID)
			continue
		}
		spans, ok := absenceSpans(a)
		if !ok {
			addIssue(r, "approval_interval_unresolved", "审批时间或计量单位无法直接换算；天/半天须提供经核对的当日班段区间", "", a.ID)
			continue
		}
		if a.Kind == "overtime" {
			e.overtime = append(e.overtime, spans...)
			addTrace(r, "overtime_approval", "已批准加班仅作为时间交集依据", "", duration(spans), evidenceID("approval", id))
			continue
		}
		spans = overlap(spans, work)
		if len(spans) == 0 {
			continue
		}
		if duration(overlap(spans, attendance)) > 0 {
			addIssue(r, "approval_overlap", "已批准的请假、外出、出差区间重叠，待人工复核", "", id)
		}
		attendance = append(attendance, spans...)
		e.absences = append(e.absences, a)
		switch a.Kind {
		case "leave":
			e.leave = append(e.leave, spans...)
			policy, exists := in.Rule.LeavePolicies[a.LeaveType]
			if !exists {
				addIssue(r, "leave_policy_missing", "假别尚未明确核定出勤及全勤规则", "", id)
			} else if policy.CreditAttendance {
				e.paidLeave = append(e.paidLeave, spans...)
			}
		case "out":
			e.out = append(e.out, spans...)
		case "trip":
			e.trip = append(e.trip, spans...)
		}
		if a.Unit == "day" || a.Unit == "halfday" {
			credited := a.Kind == "leave" && in.Rule.LeavePolicies[a.LeaveType].CreditAttendance || a.Kind == "out" && in.Rule.CreditOut || a.Kind == "trip" && in.Rule.CreditTrip
			if credited {
				e.dayUnitCredit = append(e.dayUnitCredit, spans...)
			}
		}
		message := "已批准假勤按工作班段交集去重计量"
		if a.Unit == "day" || a.Unit == "halfday" {
			message = "天/半天审批使用核实后的当日班段区间；按当日标准工时折算核定工时"
		}
		addTrace(r, "approved_absence", message, "", duration(spans), evidenceID("approval", id))
	}
}
