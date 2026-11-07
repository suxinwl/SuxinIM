package hrm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

type attRuleApplyRequest struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Reason    string `json:"reason"`
}

type attRuleApplyPreview struct {
	RuleID            string   `json:"rule_id"`
	StartDate         string   `json:"start_date"`
	EndDate           string   `json:"end_date"`
	EmployeeCount     int      `json:"employee_count"`
	DayCount          int      `json:"day_count"`
	LockedMonths      []string `json:"locked_months"`
	ManualRosterCount int      `json:"manual_roster_count"`
	Issues            []string `json:"issues"`
	CanApply          bool     `json:"can_apply"`
	EmployeeIDs       []string `json:"-"`
}

type AttendanceRulePreview = attRuleApplyPreview

func PreviewAttendanceRule(ctx context.Context, id, start, end string) (AttendanceRulePreview, error) {
	a, err := attLoadRule(ctx, id)
	if err != nil {
		return AttendanceRulePreview{}, err
	}
	return attPreviewPublishedRule(ctx, a, attRuleApplyRequest{StartDate: start, EndDate: end})
}

func ApplyAttendanceRule(ctx context.Context, id, start, end, reason string) (AttRule, AttendanceRulePreview, []string, error) {
	return attApplyPublishedRule(ctx, id, attRuleApplyRequest{StartDate: start, EndDate: end, Reason: reason})
}

func QueueAttendanceRecalculation(ctx context.Context, start, end string, employees []string) ([]string, error) {
	if core.UID(ctx) <= 0 || !core.Has(ctx, "biz-suite.hrm.write") {
		return nil, fmt.Errorf("没有考勤计算权限")
	}
	dates, err := attRange(start, end)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	err = core.DB().Transaction(ctx, func(c context.Context, _ gdb.TX) error {
		for len(dates) > 0 {
			n := 1
			for n < len(dates) && dates[n][:7] == dates[0][:7] {
				n++
			}
			id, e := attQueueBatch(c, AttBatchRequest{StartDate: dates[0], EndDate: dates[n-1], EmployeeIDs: employees})
			if e != nil {
				return e
			}
			ids = append(ids, id)
			dates = dates[n:]
		}
		return nil
	})
	return ids, err
}

func attValidateDayCandidates(day AttCalendarDay, shifts map[string]bool) error {
	if day.CandidateShiftIDs == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, id := range *day.CandidateShiftIDs {
		if !shifts[id] || seen[id] {
			return fmt.Errorf("候选班次不存在或重复")
		}
		seen[id] = true
	}
	return nil
}

func attPublishedRules(ctx context.Context) ([]AttRule, map[int64]int64, error) {
	rows, err := core.Model("biz_att_rule").Ctx(ctx).Where("status", "published").All()
	if err != nil {
		return nil, nil, err
	}
	rules := make([]AttRule, 0, len(rows))
	for _, row := range rows {
		rules = append(rules, attDecodeRule(row))
	}
	depts, err := core.HostModel("auth_dept").Ctx(ctx).Fields("id,pid").All()
	if err != nil {
		return nil, nil, err
	}
	parents := map[int64]int64{}
	for _, d := range depts {
		parents[d["id"].Int64()] = d["pid"].Int64()
	}
	return rules, parents, nil
}

func attPreviewPublishedRule(ctx context.Context, a AttRule, in attRuleApplyRequest) (attRuleApplyPreview, error) {
	p := attRuleApplyPreview{RuleID: a.ID, LockedMonths: []string{}, Issues: []string{}}
	if !queryScope(ctx).All || !core.Has(ctx, "biz-suite.hrm.write") {
		return p, fmt.Errorf("仅全部人事数据范围的管理员可以调整制度")
	}
	if a.Status != "draft" {
		return p, fmt.Errorf("只有草稿可以生效；已发布制度请保存为新版本")
	}
	if err := attRuleSourceEditable(a); err != nil {
		return p, err
	}
	if err := validateAttConfig(a); err != nil {
		return p, err
	}
	p.StartDate = in.StartDate
	if p.StartDate == "" {
		p.StartDate = max(a.EffectiveFrom, "2026-10-01")
	}
	p.EndDate = in.EndDate
	if p.EndDate == "" {
		p.EndDate = time.Now().In(core.Location).Format("2006-01-02")
	}
	if a.EffectiveTo != "" && p.EndDate > a.EffectiveTo {
		p.EndDate = a.EffectiveTo
	}
	if p.StartDate < a.EffectiveFrom {
		return p, fmt.Errorf("重算开始日期不能早于制度生效日期")
	}
	if _, err := attDate(p.StartDate); err != nil {
		return p, err
	}
	if _, err := attDate(p.EndDate); err != nil {
		return p, err
	}
	// Future policies still need a real, unambiguous employee scope. Preview
	// their first effective date without manufacturing future attendance.
	future := p.StartDate > p.EndDate
	previewEnd := p.EndDate
	if future {
		previewEnd = p.StartDate
	}
	dates, err := attRange(p.StartDate, previewEnd)
	if err != nil {
		return p, err
	}
	people, err := attEmployees(ctx)
	if err != nil {
		return p, err
	}
	rules, parents, err := attPublishedRules(ctx)
	if err != nil {
		return p, err
	}
	a.Status = "published"
	all := append(append([]AttRule(nil), rules...), a)
	seenIssue := map[string]bool{}
	snapshots := map[string]AttEmployee{}
	rows, err := core.Model("biz_att_roster").Ctx(ctx).WhereBetween("work_date", p.StartDate, previewEnd).All()
	if err != nil {
		return p, err
	}
	for _, row := range rows {
		roster := attDecodeRoster(row)
		if roster.Employee.ID == roster.EmployeeID {
			snapshots[roster.EmployeeID+":"+roster.WorkDate] = roster.Employee
		}
	}
	for id, person := range people {
		affected := false
		for _, date := range dates {
			dailyPerson := person
			if historical, ok := snapshots[id+":"+date]; ok {
				dailyPerson = historical
			}
			familyApplies := false
			for _, candidate := range all {
				if candidate.FamilyID != a.FamilyID {
					continue
				}
				if _, e := resolveAttRule([]AttRule{candidate}, dailyPerson, date, parents); e == nil {
					familyApplies = true
					break
				}
			}
			if !familyApplies {
				continue
			}
			affected = true
			if !future {
				p.DayCount++
			}
			if _, e := resolveAttRule(all, dailyPerson, date, parents); e != nil {
				message := person.Name + "（" + person.EmployeeNo + "）" + date + "：" + e.Error()
				if !seenIssue[message] && len(p.Issues) < 100 {
					p.Issues = append(p.Issues, message)
					seenIssue[message] = true
				}
			}
		}
		if affected {
			p.EmployeeIDs = append(p.EmployeeIDs, id)
		}
	}
	sort.Strings(p.EmployeeIDs)
	p.EmployeeCount = len(p.EmployeeIDs)
	if p.EmployeeCount == 0 {
		p.Issues = append(p.Issues, "制度范围未匹配任何有效人员，请先确认适用人员或部门")
	}
	if p.EmployeeCount > 0 && !future {
		p.ManualRosterCount, err = core.Model("biz_att_roster").Ctx(ctx).WhereIn("employee_id", p.EmployeeIDs).WhereBetween("work_date", p.StartDate, p.EndDate).WhereNotIn("source", []string{"weekly", "cycle", "automatic"}).Count()
		if err != nil {
			return p, err
		}
		months, e := core.Model("biz_att_month").Ctx(ctx).WhereBetween("month", p.StartDate[:7], p.EndDate[:7]).Where("status", "locked").All()
		if e != nil {
			return p, e
		}
		for _, month := range months {
			p.LockedMonths = append(p.LockedMonths, month["month"].String())
		}
	}
	p.CanApply = len(p.Issues) == 0 && len(p.LockedMonths) == 0
	return p, nil
}

func attRulePreview(r *ghttp.Request) {
	var in attRuleApplyRequest
	if err := r.Parse(&in); err != nil {
		attError(r, err)
		return
	}
	a, err := attLoadRule(r.Context(), r.GetRouter("id").String())
	if err != nil {
		attError(r, err)
		return
	}
	p, err := attPreviewPublishedRule(r.Context(), a, in)
	if err != nil {
		attError(r, err)
		return
	}
	attOK(r, p)
}

func attApplyPublishedRule(ctx context.Context, id string, in attRuleApplyRequest) (AttRule, attRuleApplyPreview, []string, error) {
	attMu.Lock()
	defer attMu.Unlock()
	var a AttRule
	var p attRuleApplyPreview
	ids := []string{}
	err := core.DB().Transaction(ctx, func(c context.Context, _ gdb.TX) error {
		var err error
		a, err = attLoadRule(c, id)
		if err != nil {
			return err
		}
		p, err = attPreviewPublishedRule(c, a, in)
		if err != nil {
			return err
		}
		if !p.CanApply {
			if len(p.Issues) > 0 {
				return fmt.Errorf("制度暂不能生效：%s", strings.Join(p.Issues, "；"))
			}
			return fmt.Errorf("制度暂不能生效：请先重开锁定月份 %s", strings.Join(p.LockedMonths, "、"))
		}
		old := a
		a.Status = "published"
		if _, err = core.Model("biz_att_rule").Ctx(c).Where("id", a.ID).Where("status", "draft").Update(g.Map{"status": "published", "updated_at": core.Now()}); err != nil {
			return err
		}
		reason := strings.TrimSpace(in.Reason)
		if reason == "" {
			reason = "保存制度并生效，自动生成日历和重算"
		}
		if err = attAudit(c, "rule", a.ID, "publish", reason, old, a); err != nil {
			return err
		}
		// Legacy rules retain their explicitly generated rosters. Local rules
		// opt into automatic attendance; old WeCom reference data cannot do so.
		if p.EmployeeCount == 0 || p.StartDate > p.EndDate || a.Config.Rule.EvidenceMode != "local" {
			return nil
		}
		if _, err = core.Model("biz_att_month").Ctx(c).WhereBetween("month", p.StartDate[:7], p.EndDate[:7]).Update(g.Map{"change_pending": 1}); err != nil {
			return err
		}
		start, _ := attDate(p.StartDate)
		end, _ := attDate(p.EndDate)
		first := start
		for !start.After(end) {
			// Calendar-month batches can be reused directly by month closing.
			last := time.Date(start.Year(), start.Month()+1, 1, 0, 0, 0, 0, core.Location).AddDate(0, 0, -1)
			if last.After(end) {
				last = end
			}
			batch, e := attQueueBatch(c, AttBatchRequest{StartDate: start.Format("2006-01-02"), EndDate: last.Format("2006-01-02"), EmployeeIDs: p.EmployeeIDs, Automatic: true})
			if e != nil {
				return e
			}
			if batch != "" {
				ids = append(ids, batch)
			}
			start = last.AddDate(0, 0, 1)
		}
		// Keep the main batches exactly monthly for closing. A changed first
		// or last shift can also alter the adjacent day's cross-night punch
		// ownership or late-arrival entitlement; those dates are incremental
		// jobs, never in-place edits to previous result versions.
		for _, boundary := range []time.Time{first.AddDate(0, 0, -1), end.AddDate(0, 0, 1)} {
			if err = attInvalidateExactDate(c, boundary, p.EmployeeIDs); err != nil {
				return err
			}
		}
		return nil
	})
	return a, p, ids, err
}

func attRuleApply(r *ghttp.Request) {
	var in attRuleApplyRequest
	if err := r.Parse(&in); err != nil {
		attError(r, err)
		return
	}
	a, p, ids, err := attApplyPublishedRule(r.Context(), r.GetRouter("id").String(), in)
	if err != nil {
		attError(r, err)
		return
	}
	status := "queued"
	if len(ids) == 0 {
		status = "published"
	}
	attOK(r, g.Map{"rule": a, "preview": p, "batch_ids": ids, "status": status})
}

// Derive ordinary weekdays without requiring a separate human roster action.
// The saved roster is evidence: explicit manual assignments are never replaced.
func attEnsureDerivedRostersLocked(ctx context.Context, request AttBatchRequest) error {
	start, err := attDate(request.StartDate)
	if err != nil {
		return err
	}
	end, err := attDate(request.EndDate)
	if err != nil {
		return err
	}
	dates, err := attRange(start.AddDate(0, 0, -1).Format("2006-01-02"), end.AddDate(0, 0, 1).Format("2006-01-02"))
	if err != nil {
		return err
	}
	rules, parents, err := attPublishedRules(ctx)
	if err != nil {
		return err
	}
	people, err := attEmployees(ctx)
	if err != nil {
		return err
	}
	holidayRows, err := core.Model("biz_hrm_attendance_holiday").Ctx(ctx).WhereBetween("holiday_date", dates[0], dates[len(dates)-1]).All()
	if err != nil {
		return err
	}
	holidays := map[string]gdb.Record{}
	for _, h := range holidayRows {
		holidays[h["holiday_date"].String()] = h
	}
	for _, id := range request.EmployeeIDs {
		person, ok := people[id]
		if !ok {
			continue
		}
		previous, err := core.Model("biz_att_roster").Ctx(ctx).Where("employee_id", id).WhereBetween("work_date", dates[0], dates[len(dates)-1]).All()
		if err != nil {
			return err
		}
		old := map[string]AttRoster{}
		for _, row := range previous {
			a := attDecodeRoster(row)
			old[a.WorkDate] = a
		}
		punches, err := attLoadPunches(ctx, id, start.AddDate(0, 0, -2).Format("2006-01-02 15:04:05"), end.AddDate(0, 0, 4).Format("2006-01-02 15:04:05"))
		if err != nil {
			return err
		}
		absences, err := attLoadAbsences(ctx, id, start.AddDate(0, 0, -2).Format("2006-01-02 15:04:05"), end.AddDate(0, 0, 4).Format("2006-01-02 15:04:05"))
		if err != nil {
			return err
		}
		items := []AttRoster{}
		for _, date := range dates {
			if attLocked(ctx, date) {
				continue
			}
			if a, exists := old[date]; exists && !attGeneratedRosterSource(a.Source) {
				continue
			}
			dailyPerson := person
			if prior, exists := old[date]; exists {
				if prior.Employee.ID != id {
					if err := attDeleteDerivedRoster(ctx, prior, "历史人员归属快照缺失，等待确认后重新配置"); err != nil {
						return err
					}
					continue
				}
				dailyPerson = prior.Employee
			}
			a, err := resolveAttRule(rules, dailyPerson, date, parents)
			if err != nil {
				if previous, exists := old[date]; exists && attGeneratedRosterSource(previous.Source) {
					if err := attDeleteDerivedRoster(ctx, previous, "适用制度已变更或存在同级冲突，等待配置"); err != nil {
						return err
					}
				}
				continue
			}
			if a.Config.Rule.EvidenceMode != "local" {
				continue
			}
			if err := validateAttConfig(a); err != nil {
				if previous, exists := old[date]; exists && attGeneratedRosterSource(previous.Source) {
					if err := attDeleteDerivedRoster(ctx, previous, "制度待配置，不沿用旧自动排班"); err != nil {
						return err
					}
				}
				continue
			}
			day, err := attCycleDay(a.Config, date)
			if err != nil {
				continue
			}
			if _, explicit := a.Config.Calendar[date]; !explicit {
				if h := holidays[date]; len(h) > 0 {
					day.Category, day.ScheduledWork = eng.Holiday, false
					if h["is_workday"].Int() != 0 {
						day.Category, day.ScheduledWork = eng.Workday, true
					}
				}
			}
			roster := AttRoster{EmployeeID: id, WorkDate: date, RuleID: a.ID, ShiftID: day.ShiftID, Category: day.Category, ScheduledWork: day.ScheduledWork, Status: "confirmed", Source: "automatic"}
			if a.Config.ShiftSelection == "auto" || (day.CandidateShiftIDs != nil && len(*day.CandidateShiftIDs) > 0) {
				prior, e := attPreviousRosterEvidence(ctx, id, date, old, punches, absences, time.Now().Unix())
				if e != nil {
					return e
				}
				selection := a
				// This leading day supplies evidence to the requested first day;
				// without flex, yesterday's allowance cannot move its out boundary.
				if date == dates[0] && selection.Config.Rule.FlexMinutes == 0 {
					selection.Config.Rule = attWithoutPriorAllowance(selection.Config.Rule)
				}
				roster.ShiftID, roster.Issue, err = attSelectCompleteShift(ctx, dailyPerson, date, day, selection, punches, absences, prior)
				if err != nil {
					return err
				}
				if roster.Issue != "" {
					roster.Status = "candidate"
				}
			} else if day.ShiftID == "" && (day.Category == eng.Workday || day.ScheduledWork) {
				roster.Status, roster.Issue = "candidate", "工作日尚未指定班次"
			}
			roster.Employee = dailyPerson
			old[date] = roster
			items = append(items, roster)
		}
		if err := attPutRosters(ctx, items, "根据本地制度自动生成考勤日历"); err != nil {
			return err
		}
	}
	return nil
}

func attSelectCompleteShift(ctx context.Context, person AttEmployee, date string, day AttCalendarDay, rule AttRule, punches []eng.Punch, absences []eng.Absence, previous ...eng.Input) (string, string, error) {
	if attHasWorkSchedule(rule.Config) && rule.Config.WorkSchedule.Mode == "after_sales" {
		return attSelectAfterSalesShift(ctx, person, date, day, rule, punches, absences)
	}
	matches := []string{}
	allRest, examined, pending := true, 0, false
	allowed := map[string]bool{}
	if day.CandidateShiftIDs != nil {
		for _, id := range *day.CandidateShiftIDs {
			allowed[id] = true
		}
	}
	for _, shift := range rule.Config.Shifts {
		if len(allowed) > 0 && !allowed[shift.ID] {
			continue
		}
		in := attDailyEvidence(eng.Input{EmployeeID: person.ID, Date: date, Category: day.Category, ScheduledWork: day.ScheduledWork, Rule: rule.Config.Rule, Shift: shift, Punches: punches, Absences: absences, AsOf: time.Now().Unix()})
		if len(previous) > 0 {
			in.PreviousDate, in.PreviousOutAt, in.PreviousOvertimeMinutes = previous[0].PreviousDate, previous[0].PreviousOutAt, previous[0].PreviousOvertimeMinutes
		}
		if _, _, err := attAttachAdjustments(ctx, person.ID, date, &in); err != nil {
			return "", "", err
		}
		v := eng.Calculate(in)
		pending = pending || v.State == "pending"
		examined++
		allRest = allRest && v.State == "rest" && len(v.Issues) == 0
		if v.State == "calculated" && v.MissingCount == 0 && len(v.Issues) == 0 && len(v.Segments) > 0 && len(v.PunchAssignments) > 0 {
			complete := true
			for _, s := range v.Segments {
				if s.State == "review" || s.State == "pending" || s.MissingCount > 0 {
					complete = false
				}
			}
			if complete {
				matches = append(matches, shift.ID)
			}
		}
	}
	if len(matches) == 1 {
		return matches[0], "", nil
	}
	if len(matches) > 1 {
		return "", "实际打卡同时符合多个班次，请确认当天班次", nil
	}
	if examined > 0 && allRest && day.Category != eng.Workday && !day.ScheduledWork {
		return "", "", nil
	}
	if pending {
		return "", "等待取卡窗口结束后自动识别班次", nil
	}
	return "", "实际打卡不能唯一完整匹配班次，请补充缺卡或确认当天班次", nil
}

func attWithoutPriorAllowance(rule eng.Rule) eng.Rule {
	rule.LateArrivalTiers = nil
	rule.LateDepartureAfterSecond = nil
	rule.LateDepartureDelayMinutes = 0
	return rule
}

// Verify yesterday's physical out boundary; an absent, ambiguous or incomplete
// prior roster never turns into an invented allowance for today.
func attPreviousRosterEvidence(ctx context.Context, employee, date string, rosters map[string]AttRoster, punches []eng.Punch, absences []eng.Absence, asof int64) (eng.Input, error) {
	out := eng.Input{}
	day, err := attDate(date)
	if err != nil {
		return out, err
	}
	previous, ok := rosters[day.AddDate(0, 0, -1).Format("2006-01-02")]
	if !ok || previous.Status != "confirmed" {
		return out, nil
	}
	rule, err := attLoadRule(ctx, previous.RuleID)
	if err != nil {
		return out, err
	}
	in := eng.Input{EmployeeID: employee, Date: previous.WorkDate, Category: previous.Category, ScheduledWork: previous.ScheduledWork, Rule: rule.Config.Rule, Punches: punches, Absences: absences, AsOf: asof}
	for _, shift := range rule.Config.Shifts {
		if shift.ID == previous.ShiftID {
			in.Shift = shift
		}
	}
	if in.Rule.FlexMinutes == 0 {
		in.Rule = attWithoutPriorAllowance(in.Rule)
	}
	in = attDailyEvidence(in)
	if _, _, err := attAttachAdjustments(ctx, employee, previous.WorkDate, &in); err != nil {
		return out, err
	}
	result := eng.Calculate(in)
	if result.State == "calculated" || result.State == "rest" {
		out.PreviousDate, out.PreviousOutAt, out.PreviousOvertimeMinutes = previous.WorkDate, attLatestAssignedOut(result, in.Punches), int(result.RawOvertimeSeconds/60)
	}
	return out, nil
}

func attDeleteDerivedRoster(ctx context.Context, a AttRoster, reason string) error {
	if _, err := core.Model("biz_att_roster").Ctx(ctx).Where("id", a.ID).Delete(); err != nil {
		return err
	}
	return attAudit(ctx, "roster", a.ID, "invalidate", reason, a, nil)
}

func attSharedPunchDates(ctx context.Context, employee string, rosters map[string]AttRoster, punches []eng.Punch, absences []eng.Absence, asof int64) (map[string][]string, error) {
	owners := map[string]map[string]bool{}
	for date, roster := range rosters {
		if roster.Status != "confirmed" {
			continue
		}
		rule, err := attLoadRule(ctx, roster.RuleID)
		if err != nil {
			return nil, err
		}
		if rule.Config.Rule.EvidenceMode != "local" {
			continue
		}
		for _, shift := range rule.Config.Shifts {
			if shift.ID != roster.ShiftID {
				continue
			}
			in := attDailyEvidence(eng.Input{EmployeeID: employee, Date: date, Category: roster.Category, ScheduledWork: roster.ScheduledWork, Rule: rule.Config.Rule, Shift: shift, Punches: punches, Absences: absences, AsOf: asof})
			if _, _, err := attAttachAdjustments(ctx, employee, date, &in); err != nil {
				return nil, err
			}
			v := eng.Calculate(in)
			for _, assignment := range v.PunchAssignments {
				if owners[assignment.PunchID] == nil {
					owners[assignment.PunchID] = map[string]bool{}
				}
				owners[assignment.PunchID][date] = true
			}
		}
	}
	out := map[string][]string{}
	for id, dates := range owners {
		if len(dates) < 2 {
			continue
		}
		for date := range dates {
			out[date] = append(out[date], id)
		}
	}
	for date := range out {
		sort.Strings(out[date])
	}
	return out, nil
}

func attLatestAssignedOut(result eng.Result, punches []eng.Punch) int64 {
	wanted := map[string]bool{}
	for _, a := range result.PunchAssignments {
		if a.Role == "out" {
			wanted[a.PunchID] = true
		}
	}
	latest := int64(0)
	for _, p := range punches {
		if !p.Placeholder && wanted[p.ID] && p.At > latest {
			latest = p.At
		}
	}
	return latest
}

func attMonthClose(r *ghttp.Request) {
	if !queryScope(r.Context()).All {
		r.Response.WriteStatus(http.StatusForbidden)
		return
	}
	var in struct {
		BatchID string `json:"batch_id"`
		Reason  string `json:"reason"`
	}
	if err := r.Parse(&in); err != nil {
		attError(r, err)
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		attError(r, fmt.Errorf("请填写月结原因"))
		return
	}
	month := r.GetRouter("month").String()
	start, err := attDate(month + "-01")
	if err != nil {
		attError(r, err)
		return
	}
	attMu.Lock()
	defer attMu.Unlock()
	err = core.DB().Transaction(r.Context(), func(ctx context.Context, _ gdb.TX) error {
		if in.BatchID == "" {
			rows, err := core.Model("biz_att_batch").Ctx(ctx).Where("status", "completed").Where("start_date", month+"-01").Where("end_date", start.AddDate(0, 1, -1).Format("2006-01-02")).All()
			if err != nil {
				return err
			}
			people, err := attEmployees(ctx)
			if err != nil {
				return err
			}
			sequences, err := attCompletedBatchSequences(ctx)
			if err != nil {
				return err
			}
			latest := int64(0)
			for _, row := range rows {
				var request AttBatchRequest
				_ = json.Unmarshal([]byte(row["request_json"].String()), &request)
				if len(request.EmployeeIDs) != len(people) || row["completed_count"].Int() != len(people)*start.AddDate(0, 1, -1).Day() {
					continue
				}
				if seq := sequences[row["id"].String()]; seq > latest {
					latest = seq
					in.BatchID = row["id"].String()
				}
			}
		}
		for _, action := range []string{"review", "publish", "lock"} {
			if err := attMonthChange(ctx, month, action, in.BatchID, in.Reason); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		attError(r, err)
		return
	}
	row, err := core.Model("biz_att_month").Ctx(r.Context()).Where("month", month).One()
	if err != nil {
		attError(r, err)
		return
	}
	attOK(r, g.Map{"month": month, "status": "locked", "batch_id": in.BatchID, "revision": row["revision"].Int()})
}
