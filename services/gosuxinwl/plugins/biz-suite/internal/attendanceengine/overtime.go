package attendanceengine

import (
	"fmt"
	"sort"
	"time"
)

type overtimeWork struct {
	date     string
	category Category
	parts    map[int][]span
}

func roundedOvertime(raw int64, p OvertimePolicy) int64 {
	if raw < int64(p.MinimumMinutes)*60 {
		return 0
	}
	if p.MaximumMinutes > 0 {
		raw = min(raw, int64(p.MaximumMinutes)*60)
	}
	quantum := p.RoundingMinutes
	if quantum == 0 {
		quantum = 30
	}
	return raw / (int64(quantum) * 60) * (int64(quantum) * 60)
}

func calculateOvertime(in Input, work []segmentWork, parts [][]span, approved []span, r *Result) {
	completeWorkday := completeWorkdayOvertimeEligible(in, work, parts, approved, r)
	buckets := map[string]*overtimeWork{}
	for i, intervals := range parts {
		for _, interval := range union(intervals) {
			for at := interval.start; at < interval.end; {
				end := interval.end
				date, category := in.Date, in.Category
				if in.Rule.OvertimeDateMode == "calendar" {
					t := time.Unix(at, 0).In(Location)
					date = t.Format("2006-01-02")
					next := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, Location).AddDate(0, 0, 1).Unix()
					end = min(end, next)
					if date != in.Date {
						var ok bool
						category, ok = in.DayCategories[date]
						if !ok || !validCategory(category) {
							addIssue(r, "overtime_category_missing", "跨零点加班缺少下一日的工作/休息/节假日类别", r.Segments[i].ID)
							at = end
							continue
						}
					}
				}
				p := in.Rule.Overtime[category]
				if p.Enabled {
					eligible := []span{{at, end}}
					if p.RequireApproval {
						eligible = overlap(eligible, approved)
					}
					key := date + ":" + string(category)
					if buckets[key] == nil {
						buckets[key] = &overtimeWork{date, category, map[int][]span{}}
					}
					buckets[key].parts[i] = append(buckets[key].parts[i], eligible...)
				}
				at = end
			}
		}
	}
	keys := []string{}
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	remainingDeductions := map[int]int64{}
	for i, s := range r.Segments {
		remainingDeductions[i] = s.DeductedSeconds
	}
	for _, key := range keys {
		b := buckets[key]
		p := in.Rule.Overtime[b.category]
		bucket := OvertimeBucket{Date: b.date, Category: b.category}
		splitWorkday := in.Category == Restday && !in.ScheduledWork && b.category == Restday && p.CompleteWorkdayMinutes > 0
		seen := []span{}
		rawBySegment := map[int]int64{}
		roundedBySegment := map[int]int64{}
		order := make([]int, len(r.Segments))
		for i := range order {
			order[i] = i
		}
		if splitWorkday {
			// Work is the independently credited base. An overlapping OT segment
			// cannot consume its same physical minutes a second time.
			sort.SliceStable(order, func(i, j int) bool {
				return r.Segments[order[i]].Kind == "work" && r.Segments[order[j]].Kind != "work"
			})
		}
		for _, i := range order {
			unique := subtract(b.parts[i], seen)
			seen = append(seen, unique...)
			raw := duration(unique)
			if deduction := min(raw, remainingDeductions[i]); deduction > 0 {
				raw -= deduction
				remainingDeductions[i] -= deduction
			}
			rawBySegment[i] = raw
			bucket.RawSeconds += raw
			r.Segments[i].RawOvertimeSeconds += raw
			if !splitWorkday && p.RoundingScope == "segment" {
				roundedBySegment[i] = roundedOvertime(raw, p)
				bucket.Seconds += roundedBySegment[i]
			}
		}
		if splitWorkday {
			applyComplete := completeWorkday && (in.Rule.OvertimeDateMode != "calendar" || b.date == in.Date)
			calculateWorkdayOvertime(p, rawBySegment, applyComplete, &bucket, r)
			r.OvertimeBuckets = append(r.OvertimeBuckets, bucket)
			r.RawOvertimeSeconds += bucket.RawSeconds
			r.OvertimeSeconds += bucket.Seconds
			continue
		}
		if p.RoundingScope != "segment" {
			bucket.Seconds = roundedOvertime(bucket.RawSeconds, p)
		} else if p.MaximumMinutes > 0 {
			bucket.Seconds = min(bucket.Seconds, roundedOvertime(int64(p.MaximumMinutes)*60, p))
		}
		remaining := bucket.Seconds
		for i := range r.Segments {
			limit := rawBySegment[i]
			if p.RoundingScope == "segment" {
				limit = roundedBySegment[i]
			}
			value := min(remaining, limit)
			r.Segments[i].OvertimeSeconds += value
			remaining -= value
		}
		r.OvertimeBuckets = append(r.OvertimeBuckets, bucket)
		r.RawOvertimeSeconds += bucket.RawSeconds
		r.OvertimeSeconds += bucket.Seconds
		message := "同一工作日类别加班先汇总，再按配置粒度向下取整"
		if p.RoundingScope == "segment" {
			message = "按明确配置逐段向下取整，再合并加班"
		}
		addTrace(r, "overtime_rounding", message+"："+b.date+" "+string(b.category), "", bucket.Seconds)
	}
}

// A fixed completed workday is an explicit credit, never an extension of punch
// timestamps. All work segments need complete actual evidence. Optional night
// segments may still be incomplete: that keeps the result under review without
// removing a separately evidenced full workday.
func completeWorkdayOvertimeEligible(in Input, work []segmentWork, actual [][]span, approved []span, r *Result) bool {
	p := in.Rule.Overtime[Restday]
	if in.Category != Restday || in.ScheduledWork || !p.Enabled || p.CompleteWorkdayMinutes <= 0 {
		return false
	}
	planned := []span{}
	complete := true
	for i, w := range work {
		if w.segment.Kind != "work" {
			continue
		}
		planned = append(planned, w.parts...)
		if w.input == nil || w.output == nil || w.issue || r.Segments[i].State != "calculated" || duration(w.parts) == 0 || duration(subtract(w.parts, actual[i])) > 0 {
			complete = false
		}
	}
	if len(planned) == 0 || !complete {
		addTrace(r, "complete_workday_not_earned", "白班未完整实际出勤，不套用整班加班核定；白天仅按有效实际，晚间单独取整", "", 0)
		return false
	}
	if in.Rule.OvertimeDateMode == "calendar" {
		for _, part := range planned {
			if time.Unix(part.start, 0).In(Location).Format("2006-01-02") != in.Date || time.Unix(part.end-1, 0).In(Location).Format("2006-01-02") != in.Date {
				addTrace(r, "complete_workday_calendar_unresolved", "白班跨自然日，未擅自分配或重复核定整班时长；各日期仅按获准实际区间核算", "", 0)
				return false
			}
		}
	}
	if p.RequireApproval && duration(subtract(planned, approved)) > 0 {
		addTrace(r, "complete_workday_approval_incomplete", "批准加班未覆盖完整白班，不套用整班核定；仅保留批准与实际交集", "", 0)
		return false
	}
	return true
}

// Complete-workday mode separates the base from additional overtime. An
// incomplete base retains exact eligible actual seconds; only the additional
// segments use the configured threshold and rounding, so base fractions cannot
// top up an evening interval shorter than one quantum.
func calculateWorkdayOvertime(p OvertimePolicy, raw map[int]int64, complete bool, b *OvertimeBucket, r *Result) {
	var additionalRaw int64
	additionalBySegment := map[int]int64{}
	for i, s := range r.Segments {
		if s.Kind == "work" {
			b.WorkdayActualSeconds += raw[i]
		} else {
			additionalRaw += raw[i]
		}
	}
	b.WorkdayCreditedSeconds = b.WorkdayActualSeconds
	if complete && b.WorkdayActualSeconds > 0 {
		b.CompleteWorkdayApplied = true
		b.WorkdayCreditedSeconds = int64(p.CompleteWorkdayMinutes) * 60
		addTrace(r, "complete_workday_overtime_credit", fmt.Sprintf("完整白班实际%d秒，按明确规则核定%d秒休息日加班；不修改实际工时或打卡跨度", b.WorkdayActualSeconds, b.WorkdayCreditedSeconds), "", b.WorkdayCreditedSeconds)
	}
	uncapped := p
	uncapped.MaximumMinutes = 0
	if p.RoundingScope == "segment" {
		for i, s := range r.Segments {
			if s.Kind != "work" {
				additionalBySegment[i] = roundedOvertime(raw[i], uncapped)
				b.AdditionalSeconds += additionalBySegment[i]
			}
		}
	} else {
		b.AdditionalSeconds = roundedOvertime(additionalRaw, uncapped)
	}
	beforeCap := b.WorkdayCreditedSeconds + b.AdditionalSeconds
	if p.MaximumMinutes > 0 {
		cap := int64(p.MaximumMinutes) * 60
		b.WorkdayCreditedSeconds = min(b.WorkdayCreditedSeconds, cap)
		quantum := int64(p.RoundingMinutes) * 60
		if quantum == 0 {
			quantum = 30 * 60
		}
		b.AdditionalSeconds = min(b.AdditionalSeconds, (cap-b.WorkdayCreditedSeconds)/quantum*quantum)
	}
	b.Seconds = b.WorkdayCreditedSeconds + b.AdditionalSeconds
	if b.Seconds < beforeCap {
		addTrace(r, "complete_workday_overtime_cap", "白班核定与额外加班合并后应用当日上限；额外加班仍保持配置取整粒度", "", b.Seconds)
	}
	var accrued, allocated int64
	remainingAdditional := b.AdditionalSeconds
	for i, s := range r.Segments {
		if s.Kind == "work" && b.WorkdayActualSeconds > 0 {
			// Allocation is bookkeeping of the explicit credit, not invented
			// actual intervals. Cumulative proportions preserve exact totals.
			accrued += raw[i]
			target := accrued * b.WorkdayCreditedSeconds / b.WorkdayActualSeconds
			r.Segments[i].OvertimeSeconds += target - allocated
			allocated = target
		} else if s.Kind != "work" {
			limit := raw[i]
			if p.RoundingScope == "segment" {
				limit = additionalBySegment[i]
			}
			value := min(remainingAdditional, limit)
			r.Segments[i].OvertimeSeconds += value
			remainingAdditional -= value
		}
	}
	addTrace(r, "complete_workday_overtime_split", fmt.Sprintf("白班加班核定%d秒，额外加班实际%d秒单独取整为%d秒，合计%d秒；白班余数不参与晚间取整", b.WorkdayCreditedSeconds, additionalRaw, b.AdditionalSeconds, b.Seconds), "", b.Seconds)
}
