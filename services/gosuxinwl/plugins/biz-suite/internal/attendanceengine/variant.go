package attendanceengine

import (
	"strings"
	"time"
)

func selectVariant(in Input, date time.Time, r *Result) Input {
	v := in.Shift.OvertimeVariant
	if v == nil {
		return in
	}
	if in.Rule.EvidenceMode == "local" {
		return selectLocalVariant(in, date, r)
	}
	// Selection must use per-segment evidence windows. A broad collection
	// horizon can include the next day's morning punches, especially when an
	// open-ended OT search ends at 06:00 and OutAfter extends collection to 08:00.
	type triggerWindow struct{ in, out span }
	windows := []triggerWindow{}
	for _, s := range v.Shift.Segments {
		if s.Kind != "overtime" {
			continue
		}
		w := in.Rule.Window
		if s.Window != nil {
			w = *s.Window
		}
		inStart := max(v.TriggerMinute, s.StartMinute) - w.InBeforeMinutes
		inEnd := s.StartMinute + w.InAfterMinutes
		outStart := max(v.TriggerMinute, s.StartMinute, s.OvertimeStartMinute)
		outEnd := s.EndMinute
		if !s.OpenEnded {
			outStart = max(outStart, s.EndMinute-w.OutBeforeMinutes)
			outEnd += w.OutAfterMinutes
		}
		// For open-ended OT, EndMinute is the explicit possible finish horizon.
		// OutAfter is still available to assignPunches after a variant is selected
		// by actual evening evidence; it is not evidence for selecting the variant.
		windows = append(windows, triggerWindow{
			in:  span{date.Unix() + int64(inStart)*60, date.Unix() + int64(inEnd)*60},
			out: span{date.Unix() + int64(outStart)*60, date.Unix() + int64(outEnd)*60},
		})
	}
	excluded := map[string]bool{}
	adjustments := map[string]Adjustment{}
	conflicts := map[string]bool{}
	for _, a := range in.Adjustments {
		if previous, ok := adjustments[a.ID]; ok && previous != a {
			conflicts[a.ID] = true
		}
		adjustments[a.ID] = a
	}
	for id, a := range adjustments {
		if !conflicts[id] && strings.TrimSpace(id) != "" && a.Status == "approved" && a.Kind == "exclude_punch" && strings.TrimSpace(a.Reason) != "" && a.PunchID != "" {
			excluded[a.PunchID] = true
		}
	}
	trigger := false
	for _, p := range in.Punches {
		if excluded[p.ID] || p.Status == "missing" || p.At > in.AsOf || p.Type == "outside" {
			continue
		}
		if in.Shift.SourceGroupID != "" && p.SourceGroupID != "" && in.Shift.SourceGroupID != p.SourceGroupID || in.Shift.SourceScheduleID != "" && p.SourceScheduleID != "" && in.Shift.SourceScheduleID != p.SourceScheduleID {
			continue
		}
		inWindow, outWindow := false, false
		for _, w := range windows {
			inWindow = inWindow || p.At >= w.in.start && p.At <= w.in.end
			outWindow = outWindow || p.At >= w.out.start && p.At <= w.out.end
		}
		if p.Type == "in" && inWindow || p.Type == "out" && outWindow {
			trigger = true
		}
		if p.Type != "in" && p.Type != "out" && (inWindow || outWindow) {
			addIssue(r, "variant_punch_type_unknown", "晚间卡类型不明确，条件班次需要人工复核", "", p.ID)
		}
	}
	if !trigger {
		addTrace(r, "base_shift_selected", "未发现条件范围内的晚间上下班卡，使用常规班次", "", 0)
		return in
	}
	old := in.Shift
	in.Shift = v.Shift
	if in.Shift.Version == "" {
		in.Shift.Version = old.Version
	}
	if in.Shift.SourceGroupID == "" {
		in.Shift.SourceGroupID = old.SourceGroupID
	}
	if in.Shift.SourceScheduleID == "" {
		in.Shift.SourceScheduleID = old.SourceScheduleID
	}
	r.ShiftVersion = in.Shift.Version
	addTrace(r, "overtime_variant_selected", "按显式条件配置及晚间实际打卡启用加班班次；加班缺卡须复核", "", 0, in.Shift.ID)
	return in
}
