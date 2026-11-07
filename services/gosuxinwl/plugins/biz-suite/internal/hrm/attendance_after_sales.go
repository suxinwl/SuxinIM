package hrm

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func attShiftLastEnd(s eng.Shift) int {
	end := 0
	for _, segment := range s.Segments {
		if segment.Kind != "break" {
			end = max(end, segment.EndMinute*60)
		}
	}
	return end
}

func attShiftLastWindow(s eng.Shift) *eng.LocalPunchWindows {
	var last eng.Segment
	for _, segment := range s.Segments {
		if segment.Kind != "break" && segment.EndMinute >= last.EndMinute {
			last = segment
		}
	}
	if last.EvidenceID != "" {
		for _, period := range s.EvidencePeriods {
			if period.ID == last.EvidenceID {
				return period.LocalWindows
			}
		}
	}
	return last.LocalWindows
}

func attValidateAfterSales(c AttConfig) error {
	p := c.WorkSchedule
	if strings.TrimSpace(p.DutyGroup) == "" || p.DutyGroup == "source:" {
		return fmt.Errorf("待配置：请填写售后留守组编号，同组人员须使用同一编号")
	}
	if p.DutyExpectedMembers < 1 || p.DutyExpectedMembers > 1000 {
		return fmt.Errorf("待配置：请确认售后留守组应有人数，不能按暂时选中的人员推定完整组")
	}
	shifts := map[string]eng.Shift{}
	for _, shift := range c.Shifts {
		shifts[shift.ID] = shift
	}
	seen := map[string]bool{}
	for _, id := range p.DutyShiftIDs {
		if _, ok := shifts[id]; !ok || seen[id] || !attStringContains(append(append([]string{}, p.WeekdayShiftIDs...), p.SaturdayShiftIDs...), id) {
			return fmt.Errorf("留守班次不存在、重复或未列入工作日候选")
		}
		seen[id] = true
	}
	for _, ids := range [][]string{p.WeekdayShiftIDs, p.SaturdayShiftIDs} {
		regular, duty := []eng.Shift{}, []eng.Shift{}
		for _, id := range ids {
			if seen[id] {
				duty = append(duty, shifts[id])
			} else {
				regular = append(regular, shifts[id])
			}
		}
		if len(regular) == 0 || len(duty) == 0 {
			return fmt.Errorf("待配置：售后周一至周五及周六均须配置普通班、留守班，并标记留守班次")
		}
		for _, normal := range regular {
			for _, extended := range duty {
				if attShiftLastEnd(normal) >= attShiftLastEnd(extended) {
					return fmt.Errorf("留守班结束时间须晚于普通班")
				}
				a, b := attShiftLastWindow(normal), attShiftLastWindow(extended)
				if a == nil || b == nil || a.OutEndSecond == nil || b.OutStartSecond == nil {
					return fmt.Errorf("待配置：请完善普通班与留守班的最后下班取卡窗口")
				}
				// Local windows are half-open; equal boundaries are adjacent.
				if *a.OutEndSecond > *b.OutStartSecond {
					return fmt.Errorf("普通班与留守班最后下班窗口重叠，请设置可区分的窗口分界后生效")
				}
			}
		}
	}
	return nil
}

func attSelectAfterSalesShift(ctx context.Context, person AttEmployee, date string, day AttCalendarDay, rule AttRule, punches []eng.Punch, absences []eng.Absence) (string, string, error) {
	allowed := []string{}
	if day.CandidateShiftIDs != nil {
		allowed = *day.CandidateShiftIDs
	} else if day.ShiftID != "" {
		allowed = []string{day.ShiftID}
	}
	regular, duty := []string{}, []string{}
	optional := day.Category != eng.Workday && !day.ScheduledWork
	type restMatch struct {
		shift     eng.Shift
		signature string
	}
	restMatches := []restMatch{}
	allRest, pending, examined := true, false, 0
	for _, shift := range rule.Config.Shifts {
		if !attStringContains(allowed, shift.ID) {
			continue
		}
		in := attDailyEvidence(eng.Input{EmployeeID: person.ID, Date: date, Category: day.Category, ScheduledWork: day.ScheduledWork, Rule: rule.Config.Rule, Shift: shift, Punches: punches, Absences: absences, AsOf: time.Now().Unix()})
		if _, _, err := attAttachAdjustments(ctx, person.ID, date, &in); err != nil {
			return "", "", err
		}
		result := eng.Calculate(in)
		examined++
		allRest = allRest && result.State == "rest" && len(result.Issues) == 0
		pending = pending || result.State == "pending"
		// Voluntary rest-day work is measured from the actual evidence. It
		// need not finish an ordinary/duty template, and there is no duty
		// requirement on a day the local calendar says nobody must work.
		if optional {
			if result.State == "calculated" && result.MissingCount == 0 && len(result.Issues) == 0 && len(result.PunchAssignments) > 0 {
				restMatches = append(restMatches, restMatch{shift: shift, signature: attAfterSalesRestSignature(result)})
			}
			continue
		}
		// Early departure from a 20:00 template must not also qualify an
		// 18:00 exit as a complete duty shift. Source labels are irrelevant.
		if !attAfterSalesCompleteEvidence(result, in) {
			continue
		}
		if attStringContains(rule.Config.WorkSchedule.DutyShiftIDs, shift.ID) {
			duty = append(duty, shift.ID)
		} else {
			regular = append(regular, shift.ID)
		}
	}
	if len(restMatches) > 0 {
		for _, match := range restMatches[1:] {
			if match.signature != restMatches[0].signature {
				return "", "休息日实际打卡存在不同的班次核算结果，请确认当天出勤班次", nil
			}
		}
		// Morning-only work has the same evidence and accounting under both
		// templates. Prefer the shorter template without inventing evening
		// attendance. Different evidence/results remain an explicit conflict.
		sort.Slice(restMatches, func(i, j int) bool {
			a, b := attShiftLastEnd(restMatches[i].shift), attShiftLastEnd(restMatches[j].shift)
			if a != b {
				return a < b
			}
			return restMatches[i].shift.ID < restMatches[j].shift.ID
		})
		return restMatches[0].shift.ID, "", nil
	}
	if len(duty) == 1 {
		return duty[0], "", nil
	}
	if len(duty) > 1 || len(regular) > 1 {
		return "", "实际打卡符合多个售后班次，请复核普通班或留守班", nil
	}
	if len(regular) == 1 {
		return regular[0], "", nil
	}
	if examined > 0 && allRest && day.Category != eng.Workday && !day.ScheduledWork {
		return "", "", nil
	}
	if pending {
		return "", "等待取卡窗口结束后识别售后普通班或留守班", nil
	}
	return "", "售后打卡缺失或未完成对应班次，请复核；不能自动当作普通班", nil
}

// Approval coverage and an explicit roster can waive missing-card penalties,
// but neither proves that an employee physically stayed until duty ended.
// Reviewed punch adjustments are already included in the immutable input.
func attAfterSalesCompleteEvidence(result eng.Result, in eng.Input) bool {
	return result.State == "calculated" && result.MissingCount == 0 && result.RawEarlySeconds == 0 && len(result.Issues) == 0 && len(result.PunchAssignments) > 0 && len(in.Shift.Segments) > 0 && attLatestAssignedOut(result, in.Punches) >= mustAttDateUnix(in.Date)+int64(attShiftLastEnd(in.Shift))
}

func attAfterSalesRestSignature(result eng.Result) string {
	active := []eng.SegmentResult{}
	for _, segment := range result.Segments {
		if segment.InPunchID != "" || segment.OutPunchID != "" || segment.ActualSeconds > 0 {
			active = append(active, segment)
		}
	}
	return attJSON(g.Map{
		"actual": result.ActualSeconds, "presence": result.PresenceSeconds, "deducted": result.DeductedSeconds,
		"credited": result.CreditedSeconds, "raw_overtime": result.RawOvertimeSeconds, "overtime": result.OvertimeSeconds,
		"buckets": result.OvertimeBuckets, "active_segments": active,
	})
}

func mustAttDateUnix(date string) int64 {
	t, _ := attDate(date)
	return t.Unix()
}

type attDutyGroupDay struct {
	Date        string
	Group       string
	EmployeeIDs []string
}

// Membership comes from explicit employee IDs on the effective policy, never
// from names. Historical roster snapshots keep transfers from rewriting scope.
func attAfterSalesGroups(ctx context.Context, dates []string, people map[string]AttEmployee) ([]attDutyGroupDay, error) {
	rules, parents, err := attPublishedRules(ctx)
	if err != nil {
		return nil, err
	}
	hasDuty := false
	for _, rule := range rules {
		hasDuty = hasDuty || attHasWorkSchedule(rule.Config) && rule.Config.WorkSchedule.Mode == "after_sales"
	}
	if !hasDuty || len(dates) == 0 {
		return nil, nil
	}
	rows, err := core.Model("biz_att_roster").Ctx(ctx).WhereBetween("work_date", dates[0], dates[len(dates)-1]).All()
	if err != nil {
		return nil, err
	}
	historical := map[string]AttEmployee{}
	for _, row := range rows {
		roster := attDecodeRoster(row)
		if roster.Employee.ID == roster.EmployeeID {
			historical[roster.WorkDate+":"+roster.EmployeeID] = roster.Employee
		}
	}
	groups := map[string]*attDutyGroupDay{}
	for _, date := range dates {
		for id, person := range people {
			if old, ok := historical[date+":"+id]; ok {
				person = old
			}
			rule, err := resolveAttRule(rules, person, date, parents)
			if err != nil || !attHasWorkSchedule(rule.Config) || rule.Config.WorkSchedule.Mode != "after_sales" {
				continue
			}
			key := date + ":" + rule.Config.WorkSchedule.DutyGroup
			if groups[key] == nil {
				groups[key] = &attDutyGroupDay{Date: date, Group: rule.Config.WorkSchedule.DutyGroup}
			}
			for _, member := range rule.Scope.EmployeeIDs {
				if !attStringContains(groups[key].EmployeeIDs, member) {
					groups[key].EmployeeIDs = append(groups[key].EmployeeIDs, member)
				}
			}
		}
	}
	keys := []string{}
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := []attDutyGroupDay{}
	for _, key := range keys {
		sort.Strings(groups[key].EmployeeIDs)
		out = append(out, *groups[key])
	}
	return out, nil
}

func attExpandAfterSalesEmployees(ctx context.Context, ids, dates []string, people map[string]AttEmployee) ([]string, error) {
	groups, err := attAfterSalesGroups(ctx, dates, people)
	if err != nil {
		return nil, err
	}
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	for changed := true; changed; {
		changed = false
		for _, group := range groups {
			intersects := false
			for _, member := range group.EmployeeIDs {
				intersects = intersects || selected[member]
			}
			if !intersects {
				continue
			}
			for _, member := range group.EmployeeIDs {
				if _, ok := people[member]; !ok {
					return nil, fmt.Errorf("售后留守核验需要本组全部有效人员及其数据权限，请先核对人员范围")
				}
				if !selected[member] {
					selected[member], changed = true, true
				}
			}
		}
	}
	out := []string{}
	for id := range selected {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

// Finalize only unpublished rows in the running batch. No previous result or
// locked month is rewritten. Group evidence is part of the batch fingerprint.
func attReviewAfterSalesGroups(ctx context.Context, batchID string, request AttBatchRequest, dates []string, people map[string]AttEmployee) error {
	groups, err := attAfterSalesGroups(ctx, dates, people)
	if err != nil || len(groups) == 0 {
		return err
	}
	rows, err := core.Model("biz_att_result").Ctx(ctx).Where("batch_id", batchID).All()
	if err != nil {
		return err
	}
	results := map[string]gdb.Record{}
	for _, row := range rows {
		results[row["work_date"].String()+":"+row["employee_id"].String()] = row
	}
	rosterRows, err := core.Model("biz_att_roster").Ctx(ctx).WhereIn("employee_id", request.EmployeeIDs).WhereBetween("work_date", dates[0], dates[len(dates)-1]).All()
	if err != nil {
		return err
	}
	rosters := map[string]AttRoster{}
	for _, row := range rosterRows {
		roster := attDecodeRoster(row)
		rosters[roster.WorkDate+":"+roster.EmployeeID] = roster
	}
	for _, group := range groups {
		if request.Automatic && attLocked(ctx, group.Date) {
			continue
		}
		intersects, required := false, false
		duty, unresolved := []string{}, []string{}
		for _, member := range group.EmployeeIDs {
			intersects = intersects || attStringContains(request.EmployeeIDs, member)
			key := group.Date + ":" + member
			roster := rosters[key]
			required = required || roster.Category == eng.Workday || roster.ScheduledWork
			row := results[key]
			var result eng.Result
			_ = json.Unmarshal([]byte(row["result_json"].String()), &result)
			if len(row) == 0 || roster.Status != "confirmed" || result.State != "calculated" || result.MissingCount > 0 || result.RawEarlySeconds > 0 {
				unresolved = append(unresolved, member)
				continue
			}
			rule, e := attLoadRule(ctx, roster.RuleID)
			if e != nil {
				return e
			}
			if !attHasWorkSchedule(rule.Config) || rule.Config.WorkSchedule.Mode != "after_sales" || rule.Config.WorkSchedule.DutyGroup != group.Group {
				unresolved = append(unresolved, member)
				continue
			}
			var evidence struct {
				Calculation eng.Input `json:"calculation"`
			}
			if err := json.Unmarshal([]byte(row["input_json"].String()), &evidence); err != nil {
				return err
			}
			if evidence.Calculation.EmployeeID != member || evidence.Calculation.Date != group.Date || evidence.Calculation.Shift.ID != roster.ShiftID || !attAfterSalesCompleteEvidence(result, evidence.Calculation) {
				unresolved = append(unresolved, member)
				continue
			}
			if attStringContains(rule.Config.WorkSchedule.DutyShiftIDs, roster.ShiftID) {
				duty = append(duty, member)
			}
		}
		if !intersects || !required {
			continue
		}
		message, code := "同组人员证据完整，当天唯一留守人员已核验", "after_sales_duty_verified"
		if len(duty) > 1 {
			message, code = "同一留守组当天有多人符合留守班，请人事复核，不能自动认定唯一留守人员", "after_sales_duty_multiple"
		} else if len(unresolved) > 0 {
			message, code = "同组人员存在缺卡、未完成班次或未核实结果，暂不能确认当天唯一留守人员", "after_sales_duty_incomplete"
		} else if len(duty) == 0 {
			message, code = "当天未识别到完整留守班，请核对留守安排和打卡证据", "after_sales_duty_missing"
		}
		for _, member := range group.EmployeeIDs {
			row := results[group.Date+":"+member]
			if len(row) == 0 {
				continue
			}
			var result eng.Result
			if err := json.Unmarshal([]byte(row["result_json"].String()), &result); err != nil {
				return err
			}
			if code != "after_sales_duty_verified" {
				if result.State != "configuration_pending" && result.State != "pending" {
					result.State = "review"
				}
				result.FullAttendanceEligible = false
				result.Issues = append(result.Issues, message)
			}
			result.Trace = append(result.Trace, eng.Trace{Code: code, Message: message, EvidenceIDs: append([]string{}, group.EmployeeIDs...)})
			var input g.Map
			if err := json.Unmarshal([]byte(row["input_json"].String()), &input); err != nil {
				return err
			}
			input["after_sales_group"] = g.Map{"group": group.Group, "date": group.Date, "employee_ids": group.EmployeeIDs, "duty_employee_ids": duty, "unresolved_employee_ids": unresolved}
			if _, err := core.Model("biz_att_result").Ctx(ctx).Where("id", row["id"]).Where("batch_id", batchID).Update(g.Map{"status": result.State, "result_json": attJSON(result), "input_json": attJSON(input)}); err != nil {
				return err
			}
		}
	}
	return nil
}
