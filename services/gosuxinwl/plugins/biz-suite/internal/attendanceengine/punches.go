package attendanceengine

import (
	"sort"
	"strconv"
)

type segmentWork struct {
	segment       Segment
	window        span
	parts         []span
	windowRule    PunchWindow
	input, output *Punch
	issue         bool
}
type boundary struct {
	at            int64
	rankAt        int64 // open-ended out uses segment start, not its distant search horizon
	kind          string
	before, after int
	segments      []int
}

func punchMode(s Segment) string {
	if s.PunchMode == "" {
		return "both"
	}
	return s.PunchMode
}
func addTrace(r *Result, code, message, segment string, seconds int64, ids ...string) {
	r.Trace = append(r.Trace, Trace{Code: code, Message: message, SegmentID: segment, Seconds: seconds, EvidenceIDs: ids})
}
func addIssue(r *Result, code, message, segment string, ids ...string) {
	r.Issues = appendUnique(r.Issues, code)
	addTrace(r, code, message, segment, 0, ids...)
}

func assignPunches(in Input, work []segmentWork, r *Result) {
	if in.Rule.EvidenceMode == "local" {
		assignLocalPunches(in, work, r)
		return
	}
	boundaries := []boundary{}
	index := map[string]int{}
	for i, w := range work {
		for _, kind := range []string{"in", "out"} {
			at, before, after := w.window.start, w.windowRule.InBeforeMinutes, w.windowRule.InAfterMinutes
			rankAt := at
			if kind == "out" {
				at, before, after = w.window.end, w.windowRule.OutBeforeMinutes, w.windowRule.OutAfterMinutes
				rankAt = at
				if w.segment.OpenEnded {
					before = int((w.window.end - w.window.start + 59) / 60)
					rankAt = w.window.start
				}
			}
			key := kind + ":" + strconv.FormatInt(at, 10) + ":" + strconv.Itoa(before) + ":" + strconv.Itoa(after) + ":" + strconv.FormatInt(rankAt, 10)
			if j, ok := index[key]; ok {
				boundaries[j].segments = append(boundaries[j].segments, i)
			} else {
				index[key] = len(boundaries)
				boundaries = append(boundaries, boundary{at: at, rankAt: rankAt, kind: kind, before: before, after: after, segments: []int{i}})
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
		if p.ID == "" {
			addIssue(r, "punch_identity_missing", "打卡缺少来源标识，不能作为可审计证据", "", p.ID)
			continue
		}
		if old, ok := seen[p.ID]; ok {
			if old != p {
				addIssue(r, "punch_identity_conflict", "同一打卡标识对应不同内容，请核对来源", "", p.ID)
			}
			continue
		}
		seen[p.ID] = p
		if excluded[p.ID] {
			addTrace(r, "punch_excluded", "已批准的人工修正排除此卡", "", 0, p.ID)
			continue
		}
		if p.Placeholder || p.Status == "missing" {
			addTrace(r, "source_missing", "源缺卡占位不能当作实际打卡", "", 0, p.ID)
			continue
		}
		if p.At <= 0 {
			addIssue(r, "punch_time_invalid", "打卡缺少有效时间", "", p.ID)
			continue
		}
		if p.At > in.AsOf {
			continue
		}
		matching := []int{}
		for i, b := range boundaries {
			if p.At >= b.at-int64(b.before)*60 && p.At <= b.at+int64(b.after)*60 {
				matching = append(matching, i)
			}
		}
		if len(matching) == 0 {
			continue
		}
		if p.Type == "outside" {
			addTrace(r, "outside_punch", "外出定位打卡不替代上下班卡", "", 0, p.ID)
			continue
		}
		if p.Type != "in" && p.Type != "out" {
			addIssue(r, "punch_type_unknown", "打卡类型未明确，请人工确认归属", "", p.ID)
			continue
		}
		if p.Status == "abnormal" && !in.Rule.AllowAbnormalPunches {
			addIssue(r, "punch_abnormal", "位置或设备等源异常尚未获准参与核算", "", p.ID)
			continue
		}
		if !enum(p.Status, "", "normal", "late", "early", "abnormal") {
			addIssue(r, "punch_status_unknown", "打卡异常状态无法识别", "", p.ID)
			continue
		}
		if in.Shift.SourceGroupID != "" && p.SourceGroupID != "" && p.SourceGroupID != in.Shift.SourceGroupID || in.Shift.SourceScheduleID != "" && p.SourceScheduleID != "" && p.SourceScheduleID != in.Shift.SourceScheduleID {
			addTrace(r, "source_schedule_mismatch", "该卡源排班与本日确认排班不同，未自动套用", "", 0, p.ID)
			continue
		}
		best := []int{}
		distance := int64(1 << 62)
		for _, i := range matching {
			b := boundaries[i]
			if b.kind != p.Type {
				continue
			}
			d := absolute(p.At - b.rankAt)
			if p.ScheduledAt > 0 && p.ScheduledAt == b.at {
				d = -1
			}
			if d < distance {
				distance = d
				best = []int{i}
			} else if d == distance {
				best = append(best, i)
			}
		}
		if len(best) == 0 {
			continue
		}
		if len(best) > 1 {
			// Identical boundaries with different configured windows remain the
			// same physical evidence, not competing shift assignments.
			same := true
			for _, j := range best[1:] {
				same = same && boundaries[j].at == boundaries[best[0]].at
			}
			if !same && !shareableBoundaries(best, boundaries, work) {
				for _, j := range best {
					for _, k := range boundaries[j].segments {
						work[k].issue = true
					}
				}
				addIssue(r, "punch_ambiguous", "打卡距多个班段边界相同，须人工确认，未重复使用", "", p.ID)
				continue
			}
		}
		// One physical observation may support overlapping attendance and OT
		// metrics, even when an approved late-arrival exemption moves only the
		// attendance boundary. It must not supply two distinct work segments.
		for _, metric := range []string{"work", "overtime"} {
			present := false
			for _, j := range best {
				for _, k := range boundaries[j].segments {
					present = present || work[k].segment.Kind == metric
				}
			}
			if present {
				continue
			}
			extra := []int{}
			closest := int64(1 << 62)
			for _, j := range matching {
				b := boundaries[j]
				if b.kind != p.Type {
					continue
				}
				rightMetric := true
				for _, k := range b.segments {
					rightMetric = rightMetric && work[k].segment.Kind == metric
				}
				if !rightMetric {
					continue
				}
				trial := append(append([]int{}, best...), j)
				if !shareableBoundaries(trial, boundaries, work) {
					continue
				}
				d := absolute(p.At - b.rankAt)
				if p.ScheduledAt > 0 && p.ScheduledAt == b.at {
					d = -1
				}
				if d < closest {
					closest = d
					extra = []int{j}
				} else if d == closest {
					extra = append(extra, j)
				}
			}
			if len(extra) > 1 && !shareableBoundaries(extra, boundaries, work) {
				addIssue(r, "punch_metric_ambiguous", "重叠出勤与加班指标存在多个等距班段，待复核", "", p.ID)
			} else {
				best = append(best, extra...)
			}
		}
		for _, j := range best {
			chosen[j] = append(chosen[j], p)
		}
	}
	for j := range boundaries {
		candidates := chosen[j]
		if len(candidates) == 0 {
			continue
		}
		b := boundaries[j]
		sort.Slice(candidates, func(i, j int) bool {
			a, z := candidates[i], candidates[j]
			if in.Rule.PunchSelection == "nearest" {
				da, dz := absolute(a.At-b.at), absolute(z.At-b.at)
				if da != dz {
					return da < dz
				}
			}
			latest := in.Rule.PunchSelection == "latest" || (enum(in.Rule.PunchSelection, "", "first_last") && b.kind == "out")
			if a.At != z.At {
				if latest {
					return a.At > z.At
				}
				return a.At < z.At
			}
			return a.ID < z.ID
		})
		if in.Rule.PunchSelection == "nearest" && len(candidates) > 1 && candidates[0].At != candidates[1].At && absolute(candidates[0].At-b.at) == absolute(candidates[1].At-b.at) {
			for _, i := range b.segments {
				work[i].issue = true
			}
			addIssue(r, "selection_ambiguous", "最近取卡有等距候选，须人工确认", "", candidates[0].ID, candidates[1].ID)
			continue
		}
		p := candidates[0]
		for _, i := range b.segments {
			if b.kind == "in" {
				work[i].input = &p
			} else {
				work[i].output = &p
			}
			addTrace(r, "punch_selected", "按确认排班、取卡窗口及取卡方式选卡", work[i].segment.ID, 0, p.ID)
		}
	}
}

func shareableBoundaries(indices []int, boundaries []boundary, work []segmentWork) bool {
	for i, a := range indices {
		for _, b := range indices[:i] {
			if boundaries[a].at == boundaries[b].at {
				continue
			}
			for _, x := range boundaries[a].segments {
				for _, y := range boundaries[b].segments {
					if work[x].segment.Kind == work[y].segment.Kind || duration(overlap(work[x].parts, work[y].parts)) == 0 {
						return false
					}
				}
			}
		}
	}
	return true
}
