package attendanceengine

import (
	"fmt"
	"sort"
	"time"
)

type localBoundary struct {
	id, role string
	at       int64
	window   span
	segments []int
}

func localWindow(w *LocalPunchWindows, role string) (int, int, bool) {
	if w == nil {
		return 0, 0, false
	}
	a, b := w.InStartSecond, w.InEndSecond
	if role == "out" {
		a, b = w.OutStartSecond, w.OutEndSecond
	}
	if a == nil || b == nil || *a >= *b {
		return 0, 0, false
	}
	return *a, *b, true
}

func localBoundaryFor(in Input, w segmentWork, role string) (localBoundary, bool) {
	date, err := ParseDate(in.Date)
	if err != nil {
		return localBoundary{}, false
	}
	windows := w.segment.LocalWindows
	id := "segment:" + w.segment.ID + ":" + role
	at := w.window.start
	if role == "out" {
		at = w.window.end
	}
	if w.segment.EvidenceID != "" {
		found := false
		for _, period := range in.Shift.EvidencePeriods {
			if period.ID != w.segment.EvidenceID {
				continue
			}
			found = true
			windows = period.LocalWindows
			id = "evidence:" + period.ID + ":" + role
			at = date.Unix() + int64(period.StartSecond)
			if role == "out" {
				at = date.Unix() + int64(period.EndSecond)
			}
			break
		}
		if !found {
			return localBoundary{}, false
		}
	}
	a, b, ok := localWindow(windows, role)
	return localBoundary{id: id, role: role, at: at, window: span{date.Unix() + int64(a), date.Unix() + int64(b)}}, ok
}

func localPunchMode(in Input, s Segment) string {
	if s.PunchMode == "none" {
		return "none"
	}
	if s.EvidenceID != "" {
		for _, period := range in.Shift.EvidencePeriods {
			if period.ID == s.EvidenceID && period.PunchMode != "" {
				return period.PunchMode
			}
		}
	}
	return punchMode(s)
}

func boundaryRequired(mode, role string) bool { return mode == "both" || mode == role }

func assignLocalPunches(in Input, work []segmentWork, r *Result) {
	boundaries := []localBoundary{}
	indices := map[string]int{}
	for i, w := range work {
		mode := localPunchMode(in, w.segment)
		for _, role := range []string{"in", "out"} {
			if !boundaryRequired(mode, role) {
				continue
			}
			b, ok := localBoundaryFor(in, w, role)
			if !ok {
				work[i].issue = true
				addIssue(r, "local_window_missing", "本地取卡范围尚未配置，暂不判断正常、迟到或早退", w.segment.ID)
				continue
			}
			if j, ok := indices[b.id]; ok {
				boundaries[j].segments = append(boundaries[j].segments, i)
			} else {
				indices[b.id] = len(boundaries)
				b.segments = []int{i}
				boundaries = append(boundaries, b)
			}
		}
	}
	excluded := map[string]bool{}
	for _, a := range in.Adjustments {
		if a.Status == "approved" && a.Kind == "exclude_punch" {
			excluded[a.PunchID] = true
		}
	}
	seen := map[string]Punch{}
	chosen := map[int][]Punch{}
	punches := append([]Punch(nil), in.Punches...)
	sort.Slice(punches, func(i, j int) bool {
		if punches[i].At == punches[j].At {
			return punches[i].ID < punches[j].ID
		}
		return punches[i].At < punches[j].At
	})
	for _, p := range punches {
		if p.Placeholder {
			addTrace(r, "source_placeholder", "来源缺卡占位不作为实际打卡", "", 0, p.ID)
			continue
		}
		if p.ID == "" || p.At <= 0 {
			addIssue(r, "punch_identity_missing", "实际打卡缺少有效时间或来源标识", "", p.ID)
			continue
		}
		if p.At > in.AsOf || excluded[p.ID] {
			continue
		}
		if old, ok := seen[p.ID]; ok {
			if old != p {
				addIssue(r, "punch_identity_conflict", "同一打卡标识对应不同内容，请核对来源", "", p.ID)
			}
			continue
		}
		seen[p.ID] = p
		matches := []int{}
		for j, b := range boundaries {
			if p.At >= b.window.start && p.At < b.window.end {
				matches = append(matches, j)
			}
		}
		if len(matches) == 0 {
			continue
		}
		if len(matches) > 1 {
			for _, j := range matches {
				for _, k := range boundaries[j].segments {
					work[k].issue = true
				}
			}
			addIssue(r, "punch_ambiguous", "实际打卡落入多个本地取卡范围，请调整分界或人工确认；未重复用卡", "", p.ID)
			continue
		}
		chosen[matches[0]] = append(chosen[matches[0]], p)
	}
	for j, b := range boundaries {
		cards := chosen[j]
		if len(cards) == 0 {
			continue
		}
		latest := in.Rule.PunchSelection == "latest" || enum(in.Rule.PunchSelection, "", "first_last") && b.role == "out"
		sort.Slice(cards, func(i, j int) bool {
			if in.Rule.PunchSelection == "nearest" {
				a, z := absolute(cards[i].At-b.at), absolute(cards[j].At-b.at)
				if a != z {
					return a < z
				}
			}
			if cards[i].At != cards[j].At {
				if latest {
					return cards[i].At > cards[j].At
				}
				return cards[i].At < cards[j].At
			}
			return cards[i].ID < cards[j].ID
		})
		if in.Rule.PunchSelection == "nearest" && len(cards) > 1 && cards[0].At != cards[1].At && absolute(cards[0].At-b.at) == absolute(cards[1].At-b.at) {
			for _, k := range b.segments {
				work[k].issue = true
			}
			addIssue(r, "selection_ambiguous", "本地最近取卡出现等距候选，请人工确认", "", cards[0].ID, cards[1].ID)
			continue
		}
		p := cards[0]
		ids := []string{}
		for _, k := range b.segments {
			if b.role == "in" {
				work[k].input = &p
			} else {
				work[k].output = &p
			}
			ids = append(ids, work[k].segment.ID)
			addTrace(r, "local_punch_selected", fmt.Sprintf("按本地%s窗口归属实际打卡，来源类型和来源状态不参与判定", map[string]string{"in": "上班", "out": "下班"}[b.role]), work[k].segment.ID, 0, p.ID)
		}
		r.PunchAssignments = append(r.PunchAssignments, PunchAssignment{PunchID: p.ID, BoundaryID: b.id, Role: b.role, SegmentIDs: ids})
	}
}

// EvidenceBounds supplies the exact collection horizon to adapters, including
// the previous/next day. ok=false means the configuration is still incomplete.
func EvidenceBounds(shift Shift) (start, end int, ok bool) {
	add := func(w *LocalPunchWindows) {
		a, b, valid := w.Bounds()
		if !valid {
			return
		}
		if !ok {
			start, end, ok = a, b, true
		} else {
			start = min(start, a)
			end = max(end, b)
		}
	}
	for _, s := range shift.Segments {
		add(s.LocalWindows)
	}
	for _, p := range shift.EvidencePeriods {
		add(p.LocalWindows)
	}
	if shift.OvertimeVariant != nil {
		a, b, valid := EvidenceBounds(shift.OvertimeVariant.Shift)
		if valid {
			if !ok {
				start, end, ok = a, b, true
			} else {
				start = min(start, a)
				end = max(end, b)
			}
		}
	}
	return
}

func selectLocalVariant(in Input, date time.Time, r *Result) Input {
	v := in.Shift.OvertimeVariant
	probe := in
	probe.Shift = v.Shift
	work, _ := buildWork(probe, date, 0)
	excluded := map[string]bool{}
	for _, a := range in.Adjustments {
		if a.Status == "approved" && a.Kind == "exclude_punch" && a.Reason != "" {
			excluded[a.PunchID] = true
		}
	}
	trigger := false
	for _, w := range work {
		if w.segment.Kind != "overtime" {
			continue
		}
		for _, role := range []string{"in", "out"} {
			b, ok := localBoundaryFor(probe, w, role)
			if !ok {
				continue
			}
			for _, p := range in.Punches {
				if p.Placeholder || p.At <= 0 || p.At > in.AsOf || excluded[p.ID] {
					continue
				}
				if p.At >= b.window.start && p.At < b.window.end {
					// A shared continuous period may begin in the morning. That
					// entry is not evidence that the evening variant was worked.
					if w.segment.EvidenceID != "" && p.At <= date.Unix()+int64(v.TriggerMinute)*60 {
						continue
					}
					trigger = true
				}
			}
		}
	}
	if !trigger {
		addTrace(r, "base_shift_selected", "本地晚间取卡范围内没有实际证据，使用常规班次", "", 0)
		return in
	}
	old := in.Shift
	in.Shift = v.Shift
	if in.Shift.Version == "" {
		in.Shift.Version = old.Version
	}
	r.ShiftVersion = in.Shift.Version
	addTrace(r, "overtime_variant_selected", "本地晚间实际证据触发加班班次；缺返岗或结束卡必须复核，不回退常规班次", "", 0, in.Shift.ID)
	return in
}

func completeCreditWork(work []segmentWork, credit [][]span, results []SegmentResult) bool {
	found := false
	for i, w := range work {
		if w.segment.Kind != "work" {
			continue
		}
		found = true
		if w.issue || results[i].State != "calculated" || results[i].MissingCount > 0 || duration(subtract(w.parts, credit[i])) > 0 {
			return false
		}
	}
	return found
}

func checkPartialWork(in Input, work []segmentWork, actual []span, r *Result) {
	p := in.Rule.PartialWorkPolicy
	if p == nil || p.Mode != "review" {
		return
	}
	planned := []span{}
	for _, w := range work {
		if w.segment.Kind == "work" {
			planned = append(planned, w.parts...)
		}
	}
	observed := overlap(planned, actual)
	if duration(observed) == 0 || duration(subtract(planned, observed)) == 0 {
		return
	}
	date, err := ParseDate(in.Date)
	if err != nil {
		return
	}
	allowed := []span{}
	for _, a := range p.ActualRanges {
		allowed = append(allowed, span{date.Unix() + int64(a.StartSecond), date.Unix() + int64(a.EndSecond)})
	}
	if duration(subtract(observed, allowed)) == 0 {
		addTrace(r, "partial_actual_confirmed", "本次部分出勤完全位于已明确无需餐休扣减的时段，按有效实际核算", "", duration(observed))
		return
	}
	addIssue(r, "partial_meal_unresolved", "部分出勤的餐休或定额扣减依据未明确，保留实际时间并交人事核对，不自动扣减或补足整班", "")
}

func countLocalMissing(in Input, work []segmentWork, results []SegmentResult) int {
	missing := map[string]bool{}
	for i, w := range work {
		if results[i].MissingCount == 0 {
			continue
		}
		mode := localPunchMode(in, w.segment)
		for _, role := range []string{"in", "out"} {
			if !boundaryRequired(mode, role) || role == "in" && w.input != nil || role == "out" && w.output != nil {
				continue
			}
			if b, ok := localBoundaryFor(in, w, role); ok {
				missing[b.id] = true
			}
		}
	}
	return len(missing)
}

func applyCompleteWorkDeduction(in Input, work []segmentWork, actual []span, r *Result) {
	r.PresenceSeconds = r.ActualSeconds
	for i := range r.Segments {
		r.Segments[i].PresenceSeconds = r.Segments[i].ActualSeconds
	}
	minutes := in.Rule.CompleteWorkDeductionMinutes
	if minutes <= 0 {
		return
	}
	planned := []span{}
	complete := true
	for i, w := range work {
		if w.segment.Kind != "work" {
			continue
		}
		planned = append(planned, w.parts...)
		if w.issue || r.Segments[i].State != "calculated" || duration(subtract(w.parts, actual)) > 0 {
			complete = false
		}
	}
	if len(planned) == 0 || !complete {
		addTrace(r, "complete_work_deduction_not_applied", "没有完整正班实际证据，不自动扣除整班餐休定额", "", 0)
		return
	}
	amount := int64(minutes) * 60
	if duration(planned) < amount {
		addIssue(r, "complete_work_deduction_invalid", "完整正班扣减超过有证据的正班时长", "")
		return
	}
	remaining := amount
	for i, w := range work {
		if w.segment.Kind != "work" {
			continue
		}
		value := min(remaining, r.Segments[i].ActualSeconds)
		r.Segments[i].DeductedSeconds = value
		r.Segments[i].ActualSeconds -= value
		remaining -= value
	}
	r.DeductedSeconds = amount - remaining
	r.ActualSeconds = max(0, r.ActualSeconds-r.DeductedSeconds)
	addTrace(r, "complete_work_duration_deduction", fmt.Sprintf("完整正班按明确制度扣除%d分钟一次；保留扣前实际覆盖，不虚构餐休发生时间", minutes), "", r.DeductedSeconds)
}
