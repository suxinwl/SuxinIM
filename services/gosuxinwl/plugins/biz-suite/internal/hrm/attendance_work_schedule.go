package hrm

import (
	"fmt"
	"strings"

	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
)

// AttWorkSchedule expresses the ordinary working week without asking HR to
// maintain seven copied assignments or a rotation anchor. Shift references are
// still explicit: a production staff Saturday may have different hours.
// Absence of this field preserves the legacy weekly/rotation configuration.
type AttWorkSchedule struct {
	Mode                string   `json:"mode"`
	WeekdayShiftIDs     []string `json:"weekday_shift_ids"`
	SaturdayShiftIDs    []string `json:"saturday_shift_ids"`
	RestdayShiftIDs     []string `json:"restday_shift_ids"`
	DutyShiftIDs        []string `json:"duty_shift_ids,omitempty"`
	DutyGroup           string   `json:"duty_group,omitempty"`
	DutyExpectedMembers int      `json:"duty_expected_members,omitempty"`
}

func attHasWorkSchedule(c AttConfig) bool {
	return c.WorkSchedule != nil && c.WorkSchedule.Mode != ""
}

func attWorkScheduleCategory(mode string) string {
	switch mode {
	case "production_employee":
		return "员工"
	case "production_staff", "after_sales":
		return "生产职员"
	default:
		return ""
	}
}

// Materialization accepts incomplete drafts, but never invents missing shift
// references. Publication validates each reference and all punch windows.
func attWorkScheduleCycle(c AttConfig) (AttCycle, error) {
	if !attHasWorkSchedule(c) {
		return c.Cycle, nil
	}
	p := c.WorkSchedule
	if attWorkScheduleCategory(p.Mode) == "" {
		return AttCycle{}, fmt.Errorf("工作周策略无效，请选择生产员工、生产职员或售后留守")
	}
	out := AttCycle{Mode: "weekly", Weekdays: make([]AttCalendarDay, 7)}
	for i := range out.Weekdays {
		ids := p.WeekdayShiftIDs
		category, required := eng.Workday, true
		if i == 5 && p.Mode != "production_employee" {
			ids = p.SaturdayShiftIDs
		} else if i >= 5 {
			ids, category, required = p.RestdayShiftIDs, eng.Restday, false
		}
		day := AttCalendarDay{Category: category, ScheduledWork: required}
		if len(ids) > 0 {
			day.ShiftID = ids[0]
		}
		// Even a single candidate must constrain a globally automatic rule;
		// weekday/saturday/duty variants must not leak into another day.
		candidates := append([]string{}, ids...)
		day.CandidateShiftIDs = &candidates
		out.Weekdays[i] = day
	}
	return out, nil
}

func attValidateWorkSchedule(c AttConfig, shifts map[string]bool) error {
	if !attHasWorkSchedule(c) {
		return nil
	}
	p := c.WorkSchedule
	category := attWorkScheduleCategory(p.Mode)
	if category == "" {
		return fmt.Errorf("工作周策略无效，请选择生产员工、生产职员或售后留守")
	}
	if c.Category != category {
		return fmt.Errorf("工作周策略与考勤分类不一致：该策略适用于%s", category)
	}
	if c.Rule.EvidenceMode != "local" {
		return fmt.Errorf("工作周策略须使用本地打卡核算，请完善本地取卡窗口后生效")
	}
	lists := []struct {
		name string
		ids  []string
	}{{"周一至周五", p.WeekdayShiftIDs}, {"休息日实际出勤", p.RestdayShiftIDs}}
	if p.Mode != "production_employee" {
		lists = append(lists, struct {
			name string
			ids  []string
		}{"周六正班", p.SaturdayShiftIDs})
	} else if len(p.SaturdayShiftIDs) > 0 {
		return fmt.Errorf("生产员工周六为休息日，请在休息日实际出勤中选择加班核算班次")
	}
	for _, list := range lists {
		if len(list.ids) == 0 {
			return fmt.Errorf("待配置：请选择%s对应的班次", list.name)
		}
		seen := map[string]bool{}
		for _, id := range list.ids {
			if strings.TrimSpace(id) == "" || !shifts[id] || seen[id] {
				return fmt.Errorf("%s引用了不存在或重复的班次，请重新选择", list.name)
			}
			seen[id] = true
		}
	}
	if p.Mode == "after_sales" {
		return attValidateAfterSales(c)
	}
	return nil
}

// The source has separate Saturday cells. Never replace those hours with the
// weekday schedule; a missing Saturday remains an incomplete draft.
func attSourceWorkSchedule(c *AttConfig, source ImportRuleDraft, weekday []eng.Shift) {
	if c.Category != "员工" && c.Category != "生产职员" {
		return
	}
	p := &AttWorkSchedule{Mode: "production_employee", WeekdayShiftIDs: []string{}, SaturdayShiftIDs: []string{}, RestdayShiftIDs: []string{}}
	for _, shift := range weekday {
		p.WeekdayShiftIDs = append(p.WeekdayShiftIDs, shift.ID)
		p.RestdayShiftIDs = append(p.RestdayShiftIDs, shift.ID)
	}
	if c.Category == "生产职员" {
		p.Mode = "production_staff"
		if strings.Contains(source.Remarks, "留守") {
			p.Mode = "after_sales"
			p.DutyGroup = "source:" + source.SourceKey
			p.DutyExpectedMembers = len(localSourceNames(source.NamesText))
		}
		for _, shift := range c.Shifts {
			if strings.HasPrefix(shift.ID, "saturday") {
				p.SaturdayShiftIDs = append(p.SaturdayShiftIDs, shift.ID)
			}
		}
	}
	if p.Mode == "after_sales" {
		// The explicit /18:00/20:00 source alternatives identify the longer
		// duty template; employee assignment still uses configured evidence.
		for _, ids := range [][]string{p.WeekdayShiftIDs, p.SaturdayShiftIDs} {
			latest := 0
			for _, shift := range c.Shifts {
				if attStringContains(ids, shift.ID) {
					latest = max(latest, attShiftLastEnd(shift))
				}
			}
			for _, shift := range c.Shifts {
				if len(ids) > 1 && attStringContains(ids, shift.ID) && attShiftLastEnd(shift) == latest {
					p.DutyShiftIDs = append(p.DutyShiftIDs, shift.ID)
				}
			}
		}
	}
	c.WorkSchedule = p
	c.Cycle, _ = attWorkScheduleCycle(*c)
}

func attStringContains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
