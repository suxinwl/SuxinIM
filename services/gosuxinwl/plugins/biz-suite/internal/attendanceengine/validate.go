package attendanceengine

import (
	"fmt"
	"sort"
	"strings"
	"time"
	_ "time/tzdata"
)

var Location = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	return loc
}()

func ParseDate(value string) (time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02", value, Location)
	if err != nil || t.Format("2006-01-02") != value || t.Year() < 1900 || t.Year() > 9998 {
		return time.Time{}, fmt.Errorf("日期必须为有效的 YYYY-MM-DD")
	}
	return t, nil
}

func validCategory(category Category) bool {
	return category == Workday || category == Restday || category == Holiday
}
func enum(value string, allowed ...string) bool {
	for _, v := range allowed {
		if value == v {
			return true
		}
	}
	return false
}

func validateWindow(w PunchWindow) error {
	for _, value := range []int{w.InBeforeMinutes, w.InAfterMinutes, w.OutBeforeMinutes, w.OutAfterMinutes} {
		if value < 0 || value > 1440 {
			return fmt.Errorf("取卡窗口分钟应在0至1440之间")
		}
	}
	return nil
}

func ValidateRule(r Rule) error {
	if r.CompleteWorkDeductionMinutes < 0 || r.CompleteWorkDeductionMinutes > 1440 {
		return fmt.Errorf("完整正班扣减分钟无效")
	}
	if r.LateDepartureAfterSecond != nil && (*r.LateDepartureAfterSecond < 0 || *r.LateDepartureAfterSecond >= 172800 || r.LateDepartureDelayMinutes <= 0 || r.LateDepartureDelayMinutes > 1440) {
		return fmt.Errorf("晚走次日晚到的时间阈值或顺延分钟无效")
	}
	if r.LateDepartureAfterSecond == nil && r.LateDepartureDelayMinutes != 0 {
		return fmt.Errorf("晚走顺延需明确前日下班时间阈值")
	}
	if !enum(r.EvidenceMode, "", "legacy", "local") {
		return fmt.Errorf("打卡证据模式无效")
	}
	if r.PartialWorkPolicy != nil {
		if !enum(r.PartialWorkPolicy.Mode, "actual", "review") {
			return fmt.Errorf("部分出勤核定方式无效")
		}
		for _, v := range r.PartialWorkPolicy.ActualRanges {
			if v.StartSecond < 0 || v.EndSecond <= v.StartSecond || v.EndSecond > 172800 {
				return fmt.Errorf("已明确的部分出勤时段无效")
			}
		}
	}
	if err := validateWindow(r.Window); err != nil {
		return err
	}
	if !enum(r.PunchSelection, "", "first_last", "earliest", "latest", "nearest") {
		return fmt.Errorf("取卡方式无效")
	}
	if !enum(r.AttendanceMode, "", "actual", "fixed", "fixed_complete") || !enum(r.GraceMode, "", "normal", "grace") || !enum(r.OvertimeDateMode, "", "workdate", "calendar") {
		return fmt.Errorf("工时、宽限或跨日分类方式无效")
	}
	if !enum(r.LateDeductionMode, "", "none", "excess", "full") || !enum(r.EarlyDeductionMode, "", "none", "excess", "full") {
		return fmt.Errorf("迟到早退扣减方式无效")
	}
	for _, value := range []int{r.LateGraceMinutes, r.EarlyGraceMinutes, r.FlexMinutes, r.StandardMinutes} {
		if value < 0 || value > 2880 {
			return fmt.Errorf("考勤规则分钟超出0至2880范围")
		}
	}
	seen := map[int]bool{}
	for _, tier := range r.LateArrivalTiers {
		if tier.PreviousOvertimeMinutes <= 0 || tier.PreviousOvertimeMinutes > 2880 || tier.DelayMinutes < 0 || tier.DelayMinutes > 1440 || seen[tier.PreviousOvertimeMinutes] {
			return fmt.Errorf("晚走次日晚到档位无效或阈值重复")
		}
		seen[tier.PreviousOvertimeMinutes] = true
	}
	for category, p := range r.Overtime {
		if !validCategory(category) || !enum(p.RoundingScope, "", "daily", "segment") {
			return fmt.Errorf("加班类别或取整范围无效")
		}
		if p.MinimumMinutes < 0 || p.MinimumMinutes > 2880 || p.RoundingMinutes < 0 || p.RoundingMinutes > 1440 || p.MaximumMinutes < 0 || p.MaximumMinutes > 2880 {
			return fmt.Errorf("加班分钟配置无效")
		}
		if p.CompleteWorkdayMinutes < 0 || p.CompleteWorkdayMinutes > 1440 || p.CompleteWorkdayMinutes > 0 && category != Restday {
			return fmt.Errorf("完整白班加班核定仅可用于休息日，分钟须在0至1440之间")
		}
		if p.MaximumMinutes > 0 && p.MaximumMinutes < p.MinimumMinutes {
			return fmt.Errorf("加班上限不能小于起算阈值")
		}
	}
	for name := range r.LeavePolicies {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("假别规则名称不能为空")
		}
	}
	return nil
}

func ValidateShift(s Shift) error {
	periods := map[string]EvidencePeriod{}
	for _, p := range s.EvidencePeriods {
		_, duplicate := periods[p.ID]
		if strings.TrimSpace(p.ID) == "" || duplicate || p.StartSecond < 0 || p.EndSecond <= p.StartSecond || p.EndSecond > 172800 || !enum(p.PunchMode, "", "both", "in", "out", "none") {
			return fmt.Errorf("连续采证时段标识、起止或打卡方式无效")
		}
		periods[p.ID] = p
		if p.LocalWindows != nil {
			if err := validateLocalWindows(p.LocalWindows); err != nil {
				return err
			}
		}
	}
	if s.OvertimeVariant != nil {
		v := s.OvertimeVariant
		if v.TriggerMinute < 0 || v.TriggerMinute >= 2880 || v.Shift.OvertimeVariant != nil {
			return fmt.Errorf("条件加班班次触发时间无效或存在嵌套")
		}
		if err := ValidateShift(v.Shift); err != nil {
			return err
		}
		found := false
		for _, segment := range v.Shift.Segments {
			found = found || segment.Kind == "overtime"
		}
		if !found {
			return fmt.Errorf("条件加班班次须含加班段")
		}
	}
	ids := map[string]bool{}
	for _, segment := range s.Segments {
		if strings.TrimSpace(segment.ID) == "" || ids[segment.ID] {
			return fmt.Errorf("班段ID不能为空或重复")
		}
		ids[segment.ID] = true
		if !enum(segment.Kind, "work", "break", "overtime") || !enum(segment.PunchMode, "", "both", "in", "out", "none") {
			return fmt.Errorf("班段类型或打卡方式无效")
		}
		if segment.StartMinute < 0 || segment.StartMinute >= 2880 || segment.EndMinute <= segment.StartMinute || segment.EndMinute > 2880 {
			return fmt.Errorf("班段起止应为工作日零点后的分钟，跨夜结束需加1440，最长覆盖两日")
		}
		if segment.Window != nil {
			if err := validateWindow(*segment.Window); err != nil {
				return err
			}
		}
		if segment.LocalWindows != nil {
			if err := validateLocalWindows(segment.LocalWindows); err != nil {
				return err
			}
		}
		if segment.EvidenceID != "" {
			period, ok := periods[segment.EvidenceID]
			if !ok {
				return fmt.Errorf("班段引用的连续采证时段不存在")
			}
			if segment.StartMinute*60 < period.StartSecond || segment.EndMinute*60 > period.EndSecond {
				return fmt.Errorf("核算班段须位于所引用的连续采证时段内")
			}
		}
		if segment.OpenEnded && segment.Kind != "overtime" {
			return fmt.Errorf("实际下班决定结束仅适用于加班段")
		}
		if segment.OvertimeStartMinute != 0 && (segment.Kind != "overtime" || segment.OvertimeStartMinute < segment.StartMinute || segment.OvertimeStartMinute >= segment.EndMinute) {
			return fmt.Errorf("加班计时开始须在加班段内")
		}
	}
	for i, a := range s.Segments {
		for _, b := range s.Segments[:i] {
			// Attendance credit and overtime are independent metrics and may cover
			// the same period. Duplicate work/OT spans within one metric are invalid.
			if a.Kind == b.Kind && a.Kind != "break" && a.StartMinute < b.EndMinute && b.StartMinute < a.EndMinute {
				return fmt.Errorf("同类工作或加班时段不可重叠")
			}
		}
	}
	return nil
}

func validateLocalWindows(w *LocalPunchWindows) error {
	if w == nil {
		return nil
	}
	for _, pair := range [][2]*int{{w.InStartSecond, w.InEndSecond}, {w.OutStartSecond, w.OutEndSecond}} {
		for _, v := range pair {
			if v != nil && (*v < -86400 || *v > 172800) {
				return fmt.Errorf("本地取卡秒偏移须在前日零点至次日结束范围内")
			}
		}
		if pair[0] != nil && pair[1] != nil && *pair[0] >= *pair[1] {
			return fmt.Errorf("本地取卡结束时间必须晚于开始时间，窗口为左闭右开")
		}
	}
	return nil
}

// ValidateLocalShift is used at publication as well as calculation. Drafts may
// retain nil boundaries, but they never manufacture normal attendance.
func ValidateLocalShift(s Shift) error {
	if err := ValidateShift(s); err != nil {
		return err
	}
	periods := map[string]EvidencePeriod{}
	type configuredBoundary struct {
		id, label  string
		start, end int
	}
	boundaries := map[string]configuredBoundary{}
	for _, p := range s.EvidencePeriods {
		periods[p.ID] = p
	}
	for _, seg := range s.Segments {
		if seg.Kind == "break" {
			continue
		}
		mode, w := punchMode(seg), seg.LocalWindows
		if seg.EvidenceID != "" {
			p := periods[seg.EvidenceID]
			w = p.LocalWindows
			if p.PunchMode != "" && mode != "none" {
				mode = p.PunchMode
			}
		}
		for _, role := range []string{"in", "out"} {
			if boundaryRequired(mode, role) {
				a, b, ok := localWindow(w, role)
				if !ok {
					return fmt.Errorf("班段%s的%s取卡范围尚未配置", seg.Name+"（"+seg.ID+"）", map[string]string{"in": "上班", "out": "下班"}[role])
				}
				key := "segment:" + seg.ID + ":" + role
				if seg.EvidenceID != "" {
					key = "evidence:" + seg.EvidenceID + ":" + role
				}
				boundaries[key] = configuredBoundary{key, seg.Name + "（" + seg.ID + "）" + map[string]string{"in": "上班", "out": "下班"}[role], a, b}
			}
		}
	}
	ordered := []configuredBoundary{}
	for _, b := range boundaries {
		ordered = append(ordered, b)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].id < ordered[j].id })
	for i, a := range ordered {
		for _, b := range ordered[:i] {
			if a.start < b.end && b.start < a.end {
				return fmt.Errorf("本地取卡范围重叠：%s与%s，请设置唯一分界", a.label, b.label)
			}
		}
	}
	if s.OvertimeVariant != nil {
		return ValidateLocalShift(s.OvertimeVariant.Shift)
	}
	return nil
}

// ResolveCycle applies a dated cycle; an empty shift ID explicitly means rest.
// Dates before the anchor remain valid and use a mathematical positive modulo.
func ResolveCycle(anchor, date string, days []string) (string, error) {
	a, err := ParseDate(anchor)
	if err != nil {
		return "", err
	}
	d, err := ParseDate(date)
	if err != nil {
		return "", err
	}
	if len(days) == 0 || len(days) > 366 {
		return "", fmt.Errorf("排班周期须有1至366天")
	}
	// Count calendar dates, not elapsed Shanghai hours (historical DST exists).
	au := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	du := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	n := int((du.Unix() - au.Unix()) / 86400)
	return days[(n%len(days)+len(days))%len(days)], nil
}

type RosterDay struct {
	Date      string   `json:"date"`
	ShiftID   string   `json:"shift_id"`
	Category  Category `json:"category"`
	Confirmed bool     `json:"confirmed"`
	Source    string   `json:"source"`
}
type Cycle struct {
	Anchor string   `json:"anchor"`
	Days   []string `json:"days"`
}
type GenerateInput struct {
	Start     string               `json:"start"`
	End       string               `json:"end"`
	Cycle     Cycle                `json:"cycle"`
	Calendar  map[string]RosterDay `json:"calendar"`
	Confirmed bool                 `json:"confirmed"`
}

// Generate does not infer shifts from punches. Generated assignments remain
// drafts unless a caller explicitly confirms them after validating scope.
func Generate(in GenerateInput) ([]RosterDay, error) {
	start, err := ParseDate(in.Start)
	if err != nil {
		return nil, err
	}
	end, err := ParseDate(in.End)
	if err != nil {
		return nil, err
	}
	if end.Before(start) || end.Sub(start) > 366*24*time.Hour {
		return nil, fmt.Errorf("排班生成范围不能倒置或超过367天")
	}
	result := []RosterDay{}
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		key := date.Format("2006-01-02")
		if override, ok := in.Calendar[key]; ok {
			if !validCategory(override.Category) {
				return nil, fmt.Errorf("日期 %s 的日类别无效", key)
			}
			override.Date, override.Source = key, "calendar"
			override.Confirmed = in.Confirmed
			result = append(result, override)
			continue
		}
		id, err := ResolveCycle(in.Cycle.Anchor, key, in.Cycle.Days)
		if err != nil {
			return nil, err
		}
		category := Workday
		if id == "" {
			category = Restday
		}
		result = append(result, RosterDay{Date: key, ShiftID: id, Category: category, Confirmed: in.Confirmed, Source: "cycle"})
	}
	return result, nil
}

// Confirmed roster candidates are resolved by explicit priority, never by the
// apparent fit of a punch. Equal-priority conflicting assignments need review.
type RosterCandidate struct {
	Day      RosterDay `json:"day"`
	Priority int       `json:"priority"`
}

func ResolveRoster(date string, candidates []RosterCandidate) (RosterDay, error) {
	if _, err := ParseDate(date); err != nil {
		return RosterDay{}, err
	}
	eligible := []RosterCandidate{}
	for _, c := range candidates {
		if c.Day.Date == date && c.Day.Confirmed {
			eligible = append(eligible, c)
		}
	}
	if len(eligible) == 0 {
		return RosterDay{}, fmt.Errorf("缺少已确认排班，请人工确认")
	}
	sort.SliceStable(eligible, func(i, j int) bool { return eligible[i].Priority > eligible[j].Priority })
	for _, c := range eligible[1:] {
		if c.Priority == eligible[0].Priority && (c.Day.ShiftID != eligible[0].Day.ShiftID || c.Day.Category != eligible[0].Day.Category) {
			return RosterDay{}, fmt.Errorf("同优先级排班冲突，请人工确认")
		}
	}
	if !validCategory(eligible[0].Day.Category) {
		return RosterDay{}, fmt.Errorf("排班日类别无效")
	}
	return eligible[0].Day, nil
}
