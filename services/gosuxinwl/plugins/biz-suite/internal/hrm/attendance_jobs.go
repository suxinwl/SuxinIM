package hrm

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type AttBatchRequest struct {
	QueuedAtUnixNano int64    `json:"queued_at_unix_nano,omitempty"`
	StartDate        string   `json:"start_date"`
	EndDate          string   `json:"end_date"`
	EmployeeIDs      []string `json:"employee_ids"`
	Automatic        bool     `json:"automatic"`
	InputFingerprint string   `json:"input_fingerprint,omitempty"`
}

var attWorkerOnce sync.Once

func StartAttendanceWorker(ctx context.Context) {
	if os.Getenv("GOSUXIN_REHEARSAL") == "true" {
		return
	}
	attWorkerOnce.Do(func() {
		_, _ = core.Model("biz_att_batch").Ctx(ctx).Where("status", "running").Update(g.Map{"status": "failed", "error_message": "服务重启中断，请重新提交试算；原结果不作为正式结果", "finished_at": core.Now()})
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					attProcessQueue(ctx)
				}
			}
		}()
	})
}
func attCreateBatch(r *ghttp.Request) {
	var in AttBatchRequest
	if e := r.Parse(&in); e != nil {
		attError(r, e)
		return
	}
	in.Automatic = false
	id, e := attQueueBatch(r.Context(), in)
	if e != nil {
		attError(r, e)
		return
	}
	attOK(r, g.Map{"id": id, "status": "queued"})
}
func attQueueBatch(ctx context.Context, in AttBatchRequest) (string, error) {
	in.InputFingerprint = "" // Only the server can certify calculated inputs.
	in.QueuedAtUnixNano = time.Now().UnixNano()
	dates, e := attRange(in.StartDate, in.EndDate)
	if e != nil {
		return "", e
	}
	if len(dates) > 62 {
		return "", fmt.Errorf("单次试算最多62天")
	}
	people, e := attEmployees(ctx)
	if e != nil {
		return "", e
	}
	if len(in.EmployeeIDs) == 0 {
		for id := range people {
			in.EmployeeIDs = append(in.EmployeeIDs, id)
		}
	}
	seen := map[string]bool{}
	ids := []string{}
	for _, id := range in.EmployeeIDs {
		if _, ok := people[id]; !ok {
			if in.Automatic {
				continue
			}
			return "", fmt.Errorf("人员不存在、被过滤或无权限")
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if len(ids) == 0 {
		if in.Automatic {
			return "", nil
		}
		return "", fmt.Errorf("没有可计算人员")
	}
	ids, e = attExpandAfterSalesEmployees(ctx, ids, dates, people)
	if e != nil {
		return "", e
	}
	sort.Strings(ids)
	in.EmployeeIDs = ids
	id := guid.S()
	_, e = core.Model("biz_att_batch").Ctx(ctx).Data(g.Map{"id": id, "start_date": in.StartDate, "end_date": in.EndDate, "status": "queued", "request_json": attJSON(in), "employee_count": len(ids), "total_count": len(ids) * len(dates), "created_by": core.UID(ctx), "created_at": core.Now()}).Insert()
	return id, e
}
func attListBatches(r *ghttp.Request) {
	q := core.Model("biz_att_batch").Ctx(r.Context())
	if !queryScope(r.Context()).All {
		q = q.Where("created_by", core.UID(r.Context()))
	}
	rows, e := q.OrderDesc("created_at").Limit(100).All()
	if e != nil {
		attError(r, e)
		return
	}
	for _, x := range rows {
		delete(x, "request_json")
	}
	attOK(r, g.Map{"items": rows})
}
func attBatchDetail(r *ghttp.Request) {
	row, e := core.Model("biz_att_batch").Ctx(r.Context()).Where("id", r.GetRouter("id").String()).One()
	if e != nil {
		attError(r, e)
		return
	}
	if len(row) == 0 || (!queryScope(r.Context()).All && row["created_by"].Int64() != core.UID(r.Context())) {
		r.Response.WriteStatus(404)
		return
	}
	delete(row, "request_json")
	attOK(r, row)
}
func attProcessQueue(ctx context.Context) {
	row, e := core.Model("biz_att_batch").Ctx(ctx).Where("status", "queued").OrderAsc("created_at").One()
	if e == nil && len(row) > 0 {
		_, e = core.Model("biz_att_batch").Ctx(ctx).Where("id", row["id"]).Where("status", "queued").Update(g.Map{"status": "running"})
		if e == nil {
			e = attCalculateBatch(ctx, row)
		}
		if e == nil {
			e = attProjectClockStates(ctx, row["id"].String(), "trial")
		}
		status := "completed"
		message := ""
		if e != nil {
			status = "failed"
			message = e.Error()
		}
		_, _ = core.Model("biz_att_batch").Ctx(ctx).Where("id", row["id"]).Update(g.Map{"status": status, "error_message": message, "finished_at": core.Now()})
		return
	}
	dirty, e := core.Model("biz_att_dirty").Ctx(ctx).Where("status", "pending").OrderAsc("created_at").One()
	if e != nil || len(dirty) == 0 {
		return
	}
	// Keep roster selection and queue creation together, including the dirty
	// acknowledgement: a retry must not leave partially queued date ranges.
	attMu.Lock()
	e = core.DB().Transaction(ctx, func(c context.Context, tx gdb.TX) error {
		if e := attQueueDirty(c, dirty); e != nil {
			return e
		}
		_, e := core.Model("biz_att_dirty").Ctx(c).Where("id", dirty["id"]).Update(g.Map{"status": "processed", "error_message": ""})
		return e
	})
	attMu.Unlock()
	if e != nil {
		_, _ = core.Model("biz_att_dirty").Ctx(ctx).Where("id", dirty["id"]).Update(g.Map{"status": "failed", "error_message": e.Error()})
	}
}

// Automatic recalculation is limited to affected, current employees on dates
// with confirmed rosters. A roster belonging to somebody else is not evidence
// that an employee has an attendance policy for that date.
func attQueueDirty(ctx context.Context, dirty gdb.Record) error {
	var affected []string
	if e := json.Unmarshal([]byte(dirty["employee_ids"].String()), &affected); e != nil {
		return fmt.Errorf("考勤变更人员范围无效: %w", e)
	}
	dates, e := attRange(dirty["start_date"].String(), dirty["end_date"].String())
	if e != nil {
		return e
	}
	people, e := attEmployees(ctx)
	if e != nil {
		return e
	}
	eligible := map[string]bool{}
	if len(affected) == 0 {
		for id := range people {
			eligible[id] = true
		}
	} else {
		for _, id := range affected {
			if _, ok := people[id]; ok {
				eligible[id] = true
			}
		}
	}
	if len(eligible) == 0 {
		return nil
	}
	ids := make([]string, 0, len(eligible))
	for id := range eligible {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	rows, e := core.Model("biz_att_roster").Ctx(ctx).WhereIn("employee_id", ids).Where("status", "confirmed").WhereGTE("work_date", dates[0]).WhereLTE("work_date", dates[len(dates)-1]).Fields("employee_id,work_date").All()
	if e != nil {
		return e
	}
	byDate := map[string][]string{}
	for _, row := range rows {
		date := row["work_date"].String()
		byDate[date] = append(byDate[date], row["employee_id"].String())
	}
	rules, parents, e := attPublishedRules(ctx)
	if e != nil {
		return e
	}
	for _, date := range dates {
		known := map[string]bool{}
		for _, id := range byDate[date] {
			known[id] = true
		}
		for _, id := range ids {
			if known[id] {
				continue
			}
			for _, rule := range rules {
				if rule.Config.Rule.EvidenceMode != "local" {
					continue
				}
				if _, err := resolveAttRule([]AttRule{rule}, people[id], date, parents); err == nil {
					byDate[date] = append(byDate[date], id)
					break
				}
			}
		}
	}
	locked := map[string]bool{}
	months, e := core.Model("biz_att_month").Ctx(ctx).WhereGTE("month", dates[0][:7]).WhereLTE("month", dates[len(dates)-1][:7]).Where("status", "locked").All()
	if e != nil {
		return e
	}
	for _, month := range months {
		value := month["month"].String()
		locked[value] = true
		if _, e = core.Model("biz_att_month").Ctx(ctx).Where("month", value).Update(g.Map{"change_pending": 1}); e != nil {
			return e
		}
	}
	var first, last, selection string
	var members []string
	count := 0
	flush := func() error {
		if first == "" {
			return nil
		}
		_, e := attQueueBatch(ctx, AttBatchRequest{StartDate: first, EndDate: last, EmployeeIDs: members, Automatic: true})
		first, last, selection, members, count = "", "", "", nil, 0
		return e
	}
	for _, date := range dates {
		daily := byDate[date]
		if locked[date[:7]] {
			daily = nil
		}
		sort.Strings(daily)
		key := strings.Join(daily, "\x00")
		// Only identical employee sets on consecutive dates can share a batch;
		// filling a date gap would calculate unassigned days again.
		if len(daily) == 0 || key != selection || count == 62 {
			if e := flush(); e != nil {
				return e
			}
		}
		if len(daily) == 0 {
			continue
		}
		if first == "" {
			first, members, selection = date, daily, key
		}
		last = date
		count++
	}
	return flush()
}

// Synchronizers enqueue invalidation only. No HRM calculation runs on their
// request goroutine. Adjacent dates include night shifts and late-arrival tiers.
func AttendanceChanged(ctx context.Context, start, end time.Time, employees []string) error {
	if start.IsZero() || end.IsZero() {
		return fmt.Errorf("考勤变更日期无效")
	}
	a := start.In(core.Location).AddDate(0, 0, -1).Format("2006-01-02")
	b := end.In(core.Location).AddDate(0, 0, 1).Format("2006-01-02")
	dates, e := attRange(a, b)
	if e != nil {
		return e
	}
	for _, d := range dates {
		if _, e = core.Model("biz_att_month").Ctx(ctx).Where("month", d[:7]).WhereIn("status", []string{"reviewed", "published", "locked"}).Update(g.Map{"change_pending": 1}); e != nil {
			return e
		}
	}
	_, e = core.Model("biz_att_dirty").Ctx(ctx).Data(g.Map{"id": guid.S(), "start_date": a, "end_date": b, "employee_ids": attJSON(employees), "status": "pending", "created_at": core.Now()}).Insert()
	return e
}

// The caller has already expanded the affected rule interval. Enqueue one
// exact boundary date, so handling it does not expand recursively into another
// day or replace the separate complete-month calculation used for month close.
func attInvalidateExactDate(ctx context.Context, date time.Time, employees []string) error {
	if date.IsZero() || len(employees) == 0 {
		return fmt.Errorf("考勤边界变更日期或人员范围为空")
	}
	day := date.In(core.Location).Format("2006-01-02")
	if _, err := core.Model("biz_att_month").Ctx(ctx).Where("month", day[:7]).WhereIn("status", []string{"reviewed", "published", "locked"}).Update(g.Map{"change_pending": 1}); err != nil {
		return err
	}
	_, err := core.Model("biz_att_dirty").Ctx(ctx).Data(g.Map{"id": guid.S(), "start_date": day, "end_date": day, "employee_ids": attJSON(employees), "status": "pending", "created_at": core.Now()}).Insert()
	return err
}

func attCalculateBatch(ctx context.Context, batch gdb.Record) error {
	var request AttBatchRequest
	if e := json.Unmarshal([]byte(batch["request_json"].String()), &request); e != nil {
		return e
	}
	uid := batch["created_by"].Int64()
	if uid > 0 {
		userctx := context.WithValue(ctx, "uid", uid)
		if !core.Has(userctx, "biz-suite.hrm.write") {
			return fmt.Errorf("提交者权限已失效")
		}
		ctx = userctx
	}
	dates, e := attRange(request.StartDate, request.EndDate)
	if e != nil {
		return e
	}
	request.InputFingerprint = ""
	if _, e = core.Model("biz_att_batch").Ctx(ctx).Where("id", batch["id"]).Update(g.Map{"request_json": attJSON(request)}); e != nil {
		return e
	}
	initialSources, e := attSourceFingerprint(ctx, request)
	if e != nil {
		return e
	}
	derivedInputs, e := attPrepareDerivedRosters(ctx, request)
	if e != nil {
		return e
	}
	initialFingerprint, e := attFinishDerivationSnapshot(ctx, request, initialSources)
	if e != nil {
		return e
	}
	// A publication between employee chunks (or just before the initial full
	// fingerprint) must not certify rosters derived from an older policy.
	if e = attVerifyDerivedInputs(ctx, request, derivedInputs); e != nil {
		return e
	}
	people, e := attEmployees(ctx)
	if e != nil {
		return e
	}
	asof := time.Now().Unix()
	done := 0
	for _, employeeID := range request.EmployeeIDs {
		person, ok := people[employeeID]
		if !ok {
			return fmt.Errorf("人员已被过滤或停用，请重新试算")
		}
		start, _ := attDate(request.StartDate)
		end, _ := attDate(request.EndDate)
		from := start.AddDate(0, 0, -2).Format("2006-01-02 15:04:05")
		to := end.AddDate(0, 0, 3).Format("2006-01-02 15:04:05")
		// Build evidence once per person. Inputs stored on every result are immutable
		// snapshots, so a later source correction cannot rewrite a prior explanation.
		punches, e := attLoadPunches(ctx, employeeID, from, to)
		if e != nil {
			return e
		}
		absences, e := attLoadAbsences(ctx, employeeID, from, to)
		if e != nil {
			return e
		}
		compIssues, e := ReconcileCompLeaveDetailed(ctx, employeeID)
		if e != nil {
			return e
		}
		rows, e := core.Model("biz_att_roster").Ctx(ctx).Where("employee_id", employeeID).WhereBetween("work_date", start.AddDate(0, 0, -1).Format("2006-01-02"), end.AddDate(0, 0, 1).Format("2006-01-02")).All()
		if e != nil {
			return e
		}
		rosters := map[string]AttRoster{}
		cats := map[string]eng.Category{}
		for _, row := range rows {
			a := attDecodeRoster(row)
			rosters[a.WorkDate] = a
			cats[a.WorkDate] = a.Category
		}
		sharedPunchDates, e := attSharedPunchDates(ctx, employeeID, rosters, punches, absences, asof)
		if e != nil {
			return e
		}
		previousMinutes := 0
		previousOutAt := int64(0)
		previousDate := ""
		if prev, ok := rosters[start.AddDate(0, 0, -1).Format("2006-01-02")]; ok && prev.Status == "confirmed" && len(sharedPunchDates[prev.WorkDate]) == 0 {
			rule, e := attLoadRule(ctx, prev.RuleID)
			if e != nil {
				return e
			}
			prevInput := eng.Input{EmployeeID: employeeID, Date: prev.WorkDate, Category: prev.Category, ScheduledWork: prev.ScheduledWork, Rule: rule.Config.Rule, Punches: punches, Absences: absences, AsOf: asof, DayCategories: cats}
			for _, s := range rule.Config.Shifts {
				if s.ID == prev.ShiftID {
					prevInput.Shift = s
				}
			}
			prevInput = attDailyEvidence(prevInput)
			if _, _, e = attAttachAdjustments(ctx, employeeID, prev.WorkDate, &prevInput); e != nil {
				return e
			}
			// Without flex, prior-day late-arrival entitlement cannot move the
			// overtime segments. Do not invent a previous-previous OT value when
			// a flex rule makes that dependency material.
			if prevInput.Rule.FlexMinutes == 0 {
				prevInput.Rule = attWithoutPriorAllowance(prevInput.Rule)
			}
			value := eng.Calculate(prevInput)
			if value.State == "calculated" || value.State == "rest" {
				previousMinutes = int(value.RawOvertimeSeconds / 60)
				previousOutAt = attLatestAssignedOut(value, prevInput.Punches)
				previousDate = prev.WorkDate
			}
		}
		for _, date := range dates {
			person = people[employeeID]
			roster, exists := rosters[date]
			input := eng.Input{EmployeeID: employeeID, Date: date, AsOf: asof, Punches: punches, Absences: absences, Category: roster.Category, DayCategories: cats, PreviousDate: previousDate, PreviousOvertimeMinutes: previousMinutes, PreviousOutAt: previousOutAt}
			input.ScheduledWork = roster.ScheduledWork
			result := eng.Result{EmployeeID: employeeID, Date: date, State: "configuration_pending", EngineVersion: eng.Version, Issues: []string{"待配置：没有适用的本地制度或有效班次，请完善考勤设置"}}
			if exists && roster.Status != "confirmed" {
				result.State = "review"
				if strings.HasPrefix(roster.Issue, "等待取卡窗口") {
					result.State = "pending"
				}
				result.Issues = []string{defaultString(roster.Issue, "实际打卡未能唯一匹配班次，请复核")}
			}
			ruleID := ""
			if exists && roster.Status == "confirmed" {
				rule, e := attLoadRule(ctx, roster.RuleID)
				if e != nil {
					return e
				}
				ruleID = rule.ID
				input.Rule = rule.Config.Rule
				input.Rule.ID = rule.ID
				input.Rule.Version = fmt.Sprint(rule.Version)
				for _, shift := range rule.Config.Shifts {
					if shift.ID == roster.ShiftID {
						input.Shift = shift
						input.Shift.Version = fmt.Sprint(rule.Version)
					}
				}
				if roster.Employee.ID != "" {
					person = roster.Employee
				}
			}
			input = attDailyEvidence(input)
			adjustments, overrides, e := attAttachAdjustments(ctx, employeeID, date, &input)
			if e != nil {
				return e
			}
			if exists && roster.Status == "confirmed" {
				if input.Rule.EvidenceMode == "local" && len(input.Shift.Segments) > 0 {
					if err := eng.ValidateLocalShift(input.Shift); err != nil {
						result.State = "configuration_pending"
						result.Issues = []string{"待配置：" + err.Error()}
					} else {
						result = eng.Calculate(input)
					}
				} else {
					result = eng.Calculate(input)
				}
			}
			for _, adj := range overrides {
				if adj.CreditedMinutes != nil {
					result.CreditedSeconds = *adj.CreditedMinutes * 60
				}
				if adj.ActualMinutes != nil {
					result.ActualSeconds = *adj.ActualMinutes * 60
				}
				if adj.ConfirmIssues && exists && roster.Status == "confirmed" && result.State != "configuration_pending" {
					result.State = "calculated"
					result.Trace = append(result.Trace, eng.Trace{Code: "human_review", Message: "人事核定：" + adj.Reason + "；依据：" + adj.Evidence, EvidenceIDs: []string{adj.ID}})
				}
				result.Trace = append(result.Trace, eng.Trace{Code: "adjustment", Message: adj.Reason + "；依据：" + adj.Evidence, EvidenceIDs: []string{adj.ID}})
			}
			if len(sharedPunchDates[date]) > 0 {
				result.State = "review"
				result.FullAttendanceEligible = false
				result.Issues = append(result.Issues, "同一打卡同时落入相邻工作日的采证边界，请调整窗口或确认所属日期")
				result.Trace = append(result.Trace, eng.Trace{Code: "cross_date_punch_conflict", Message: "跨日打卡不能同时计入两个工作日", EvidenceIDs: sharedPunchDates[date]})
			}
			if request.Automatic && attLocked(ctx, date) {
				continue
			}
			if relevant := attCompIssuesForDate(date, input.Shift, compIssues); len(relevant) > 0 {
				if result.State != "configuration_pending" {
					result.State = "review"
				}
				result.Issues = append(result.Issues, relevant...)
				result.FullAttendanceEligible = false
				result.FullAttendanceReasons = append(result.FullAttendanceReasons, "相关调休审批存在台账问题")
			}
			data := g.Map{"id": guid.S(), "batch_id": batch["id"], "employee_id": employeeID, "work_date": date, "rule_id": ruleID, "status": result.State, "employee_json": attJSON(person), "result_json": attJSON(result), "input_json": attJSON(g.Map{"calculation": input, "adjustments": adjustments}), "created_at": core.Now()}
			if _, e = core.Model("biz_att_result").Ctx(ctx).Data(data).Insert(); e != nil {
				return e
			}
			previousDate = ""
			previousMinutes = 0
			previousOutAt = 0
			if result.State == "calculated" || result.State == "rest" {
				previousDate = date
				previousMinutes = int(result.RawOvertimeSeconds / 60)
				previousOutAt = attLatestAssignedOut(result, input.Punches)
			}
			done++
		}
		if _, e = core.Model("biz_att_batch").Ctx(ctx).Where("id", batch["id"]).Update(g.Map{"completed_count": done}); e != nil {
			return e
		}
	}
	if e = attReviewAfterSalesGroups(ctx, batch["id"].String(), request, dates, people); e != nil {
		return e
	}
	finalFingerprint, e := attInputFingerprint(ctx, request)
	if e != nil {
		return e
	}
	if finalFingerprint != initialFingerprint {
		return fmt.Errorf("计算期间业务证据发生变化，请重新试算")
	}
	request.InputFingerprint = finalFingerprint
	_, e = core.Model("biz_att_batch").Ctx(ctx).Where("id", batch["id"]).Update(g.Map{"request_json": attJSON(request)})
	return e
}
func attLoadPunches(ctx context.Context, id, start, end string) ([]eng.Punch, error) {
	rows, e := core.Model("biz_hrm_attendance_clock").Ctx(ctx).Where("employee_id", id).WhereGTE("clock_time", start).WhereLT("clock_time", end).All()
	if e != nil {
		return nil, e
	}
	out := []eng.Punch{}
	refs := map[string]bool{}
	for _, r := range rows {
		t, e := core.ParseTime(r["clock_time"].String())
		if e != nil {
			continue
		}
		p := eng.Punch{ID: r["id"].String(), At: t.Unix(), Type: r["clock_type"].String(), Status: r["status"].String()}
		var raw struct {
			Actual    *int64 `json:"checkin_time"`
			Scheduled int64  `json:"sch_checkin_time"`
			Group     int64  `json:"groupid"`
			Schedule  int64  `json:"schedule_id"`
		}
		_ = json.Unmarshal([]byte(r["raw_json"].String()), &raw)
		p.Placeholder = raw.Actual != nil && *raw.Actual == 0
		p.ScheduledAt = raw.Scheduled
		if raw.Group > 0 {
			p.SourceGroupID = fmt.Sprint(raw.Group)
		}
		if raw.Schedule > 0 {
			p.SourceScheduleID = fmt.Sprint(raw.Schedule)
		}
		out = append(out, p)
		refs[r["external_ref"].String()] = true
	}
	person, e := core.Model("biz_hrm_employee").Ctx(ctx).Where("id", id).One()
	if e != nil {
		return nil, e
	}
	if person["wecom_userid"].String() != "" {
		raw, e := core.Model("wecom_checkin_raw").Ctx(ctx).Where("wecom_userid", person["wecom_userid"]).WhereGTE("checkin_time", start).WhereLT("checkin_time", end).All()
		if e != nil {
			return nil, e
		}
		for _, r := range raw {
			if refs[r["external_ref"].String()] {
				continue
			}
			t, e := core.ParseTime(r["checkin_time"].String())
			if e == nil {
				var actual struct {
					At *int64 `json:"checkin_time"`
				}
				_ = json.Unmarshal([]byte(r["raw_json"].String()), &actual)
				out = append(out, eng.Punch{ID: "raw:" + r["id"].String(), At: t.Unix(), Type: "unknown", Status: r["exception_type"].String(), Placeholder: actual.At != nil && *actual.At == 0})
			}
		}
	}
	return out, nil
}
func attLoadAbsences(ctx context.Context, id, start, end string) ([]eng.Absence, error) {
	out := []eng.Absence{}
	for _, kind := range []string{"leave", "out", "trip", "overtime"} {
		rows, e := core.Model("biz_hrm_attendance_"+kind).Ctx(ctx).Where("employee_id", id).WhereLT("start_time", end).WhereGT("end_time", start).All()
		if e != nil {
			return nil, e
		}
		for _, r := range rows {
			a, e := core.ParseTime(r["start_time"].String())
			if e != nil {
				continue
			}
			b, e := core.ParseTime(r["end_time"].String())
			if e != nil {
				continue
			}
			leave := ""
			if x := r["leave_type"]; x != nil {
				leave = absenceLeaveLabel(r["raw_json"].String(), x.String(), r["source"].String())
			}
			out = append(out, eng.Absence{ID: kind + ":" + r["id"].String(), Kind: kind, LeaveType: leave, Status: r["approval_status"].String(), StartAt: a.Unix(), EndAt: b.Unix(), Unit: attApprovalUnit(r["raw_json"].String(), a.Unix(), b.Unix())})
		}
	}
	return out, nil
}

// The job loads a person's whole period once, but a calculation snapshot keeps
// only evidence in that date's configured windows (including overnight ranges).
func attDailyEvidence(in eng.Input) eng.Input {
	day, e := attDate(in.Date)
	if e != nil {
		return in
	}
	start, end := day.Unix(), day.AddDate(0, 0, 1).Unix()
	shifts := []eng.Shift{in.Shift}
	if in.Shift.OvertimeVariant != nil {
		shifts = append(shifts, in.Shift.OvertimeVariant.Shift)
	}
	for _, shift := range shifts {
		for _, period := range shift.EvidencePeriods {
			if a, b, ok := period.LocalWindows.Bounds(); ok {
				start = min(start, day.Unix()+int64(a))
				end = max(end, day.Unix()+int64(b))
			}
		}
		for _, s := range shift.Segments {
			if s.Kind == "break" {
				continue
			}
			if in.Rule.EvidenceMode == "local" {
				if a, b, ok := s.LocalWindows.Bounds(); ok {
					start = min(start, day.Unix()+int64(a))
					end = max(end, day.Unix()+int64(b))
				}
				continue
			}
			w := in.Rule.Window
			if s.Window != nil {
				w = *s.Window
			}
			start = min(start, day.Unix()+int64(s.StartMinute-w.InBeforeMinutes)*60)
			end = max(end, day.Unix()+int64(s.EndMinute+w.OutAfterMinutes+in.Rule.FlexMinutes)*60+1)
		}
	}
	punches := []eng.Punch{}
	for _, p := range in.Punches {
		if p.At >= start && p.At < end {
			punches = append(punches, p)
		}
	}
	absences := []eng.Absence{}
	for _, a := range in.Absences {
		if a.StartAt < end && a.EndAt > start {
			absences = append(absences, a)
		}
	}
	in.Punches, in.Absences = punches, absences
	return in
}

func attCompIssuesForDate(date string, shift eng.Shift, issues []CompLeaveIssue) []string {
	day, e := attDate(date)
	if e != nil {
		return nil
	}
	spans := [][2]int64{}
	shifts := []eng.Shift{shift}
	if shift.OvertimeVariant != nil {
		shifts = append(shifts, shift.OvertimeVariant.Shift)
	}
	for _, s := range shifts {
		for _, part := range s.Segments {
			if part.Kind == "work" {
				spans = append(spans, [2]int64{day.Unix() + int64(part.StartMinute)*60, day.Unix() + int64(part.EndMinute)*60})
			}
		}
	}
	if len(spans) == 0 {
		spans = append(spans, [2]int64{day.Unix(), day.AddDate(0, 0, 1).Unix()})
	}
	out := []string{}
	seen := map[string]bool{}
	for _, issue := range issues {
		if issue.Start.IsZero() || issue.End.IsZero() {
			continue
		}
		relevant := false
		for _, span := range spans {
			relevant = relevant || issue.Start.Unix() < span[1] && issue.End.Unix() > span[0]
		}
		if relevant && !seen[issue.Message] {
			seen[issue.Message] = true
			out = append(out, issue.Message)
		}
	}
	return out
}

func attAttachAdjustments(ctx context.Context, employee, date string, input *eng.Input) (gdb.Result, []AttAdjustmentRequest, error) {
	rows, e := core.Model("biz_att_adjustment").Ctx(ctx).Where("employee_id", employee).Where("work_date", date).Order("created_at,id").All()
	if e != nil {
		return nil, nil, e
	}
	overrides := []AttAdjustmentRequest{}
	for _, row := range rows {
		var a AttAdjustmentRequest
		if e = json.Unmarshal([]byte(row["adjustment_json"].String()), &a); e != nil {
			return nil, nil, e
		}
		a.ID = row["id"].String()
		a.EmployeeID = employee
		a.WorkDate = date
		if e = attValidateAdjustment(a); e != nil {
			return nil, nil, fmt.Errorf("人工调整%s无效：%w", a.ID, e)
		}
		if a.Kind == "punch" {
			input.Punches = append(input.Punches, eng.Punch{ID: "adjustment:" + a.ID, At: a.At, Type: a.PunchType, Status: "normal"})
		} else if a.Kind == "override" {
			overrides = append(overrides, a)
		} else {
			input.Adjustments = append(input.Adjustments, eng.Adjustment{ID: a.ID, Kind: a.Kind, Reason: a.Reason + "；依据：" + a.Evidence, Status: "approved", SegmentID: a.SegmentID, PunchID: a.PunchID, StartAt: a.StartAt, EndAt: a.EndAt})
		}
	}
	return rows, overrides, nil
}

// Preserve an explicit source unit. Day/half-day values without verified slices
// must reach the engine as unresolved rather than silently becoming hours.
func attApprovalUnit(raw string, start, end int64) string {
	type dateRange struct {
		Begin int64  `json:"new_begin"`
		End   int64  `json:"new_end"`
		Type  string `json:"type"`
	}
	type controlValue struct {
		Range      dateRange `json:"date_range"`
		Attendance struct {
			Range dateRange `json:"date_range"`
		} `json:"attendance"`
		Vacation struct {
			Attendance struct {
				Range dateRange `json:"date_range"`
			} `json:"attendance"`
		} `json:"vacation"`
	}
	var root struct {
		Info      json.RawMessage `json:"info"`
		ApplyData struct {
			Contents []struct {
				Value controlValue `json:"value"`
			} `json:"contents"`
		} `json:"apply_data"`
	}
	if json.Unmarshal([]byte(raw), &root) != nil {
		return ""
	}
	if len(root.Info) > 0 {
		if json.Unmarshal(root.Info, &root) != nil {
			return ""
		}
	}
	unit := ""
	for _, c := range root.ApplyData.Contents {
		for _, dr := range []dateRange{c.Value.Range, c.Value.Attendance.Range, c.Value.Vacation.Attendance.Range} {
			if dr.Begin == start && dr.End == end && dr.Type != "" {
				if unit != "" && unit != dr.Type {
					return "unknown"
				}
				unit = dr.Type
			}
		}
	}
	return unit
}

func attResultItem(row gdb.Record, detail bool) g.Map {
	var person AttEmployee
	var result eng.Result
	_ = json.Unmarshal([]byte(row["employee_json"].String()), &person)
	_ = json.Unmarshal([]byte(row["result_json"].String()), &result)
	out := g.Map{"id": row["id"], "batch_id": row["batch_id"], "employee_id": row["employee_id"], "work_date": row["work_date"], "rule_id": row["rule_id"], "status": row["status"], "employee": person, "result": result}
	if detail {
		out["evidence"] = json.RawMessage(row["input_json"].String())
	}
	return out
}
func attResults(r *ghttp.Request) {
	q := queryScope(r.Context()).apply(core.Model("biz_att_result").Ctx(r.Context()), "employee_id")
	batch := r.GetQuery("batch_id").String()
	if batch == "" {
		q = q.Where("batch_id IN (?)", core.Model("biz_att_batch").Ctx(r.Context()).Fields("id").Where("status", "completed"))
	} else {
		q = q.Where("batch_id", batch)
	}
	for _, key := range []string{"employee_id", "status"} {
		if key == "status" && batch == "" {
			continue
		}
		if s := r.GetQuery(key).String(); s != "" {
			q = q.Where(key, s)
		}
	}
	if s := r.GetQuery("start_date").String(); s != "" {
		q = q.WhereGTE("work_date", s)
	}
	if s := r.GetQuery("end_date").String(); s != "" {
		q = q.WhereLTE("work_date", s)
	}
	rows, e := q.Order("created_at DESC,id DESC").Limit(100000).All()
	if e != nil {
		attError(r, e)
		return
	}
	if batch == "" {
		sequences, err := attCompletedBatchSequences(r.Context())
		if err != nil {
			attError(r, err)
			return
		}
		sort.SliceStable(rows, func(i, j int) bool {
			return sequences[rows[i]["batch_id"].String()] > sequences[rows[j]["batch_id"].String()]
		})
	}
	out := []g.Map{}
	seen := map[string]bool{}
	departments := map[int64]bool{}
	if dept := r.GetQuery("dept_id").Int64(); dept > 0 {
		departments[dept] = true
		if r.GetQuery("include_children").String() != "false" && r.GetQuery("include_children").String() != "0" {
			rows, err := core.HostModel("auth_dept").Ctx(r.Context()).Fields("id,pid").All()
			if err != nil {
				attError(r, err)
				return
			}
			for changed := true; changed; {
				changed = false
				for _, d := range rows {
					if departments[d["pid"].Int64()] && !departments[d["id"].Int64()] {
						departments[d["id"].Int64()] = true
						changed = true
					}
				}
			}
		}
	}
	search := strings.ToLower(strings.TrimSpace(r.GetQuery("q").String()))
	frozen := map[string]string{}
	if batch == "" {
		months, err := core.Model("biz_att_month").Ctx(r.Context()).WhereIn("status", []string{"published", "locked"}).Fields("month,batch_id").All()
		if err != nil {
			attError(r, err)
			return
		}
		for _, month := range months {
			frozen[month["month"].String()] = month["batch_id"].String()
		}
	}
	for _, row := range rows {
		if batch == "" {
			if id, ok := frozen[row["work_date"].String()[:7]]; ok && row["batch_id"].String() != id {
				continue
			}
		}
		key := row["employee_id"].String() + ":" + row["work_date"].String()
		if batch == "" && seen[key] {
			continue
		}
		seen[key] = true
		if batch == "" && r.GetQuery("status").String() != "" && row["status"].String() != r.GetQuery("status").String() {
			continue
		}
		var person AttEmployee
		_ = json.Unmarshal([]byte(row["employee_json"].String()), &person)
		if len(departments) > 0 && !departments[person.DeptID] {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(person.Name), search) && !strings.Contains(strings.ToLower(person.EmployeeNo), search) {
			continue
		}
		out = append(out, attResultItem(row, false))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return fmt.Sprint(out[i]["work_date"])+fmt.Sprint(out[i]["employee_id"]) < fmt.Sprint(out[j]["work_date"])+fmt.Sprint(out[j]["employee_id"])
	})
	attOK(r, g.Map{"items": out})
}

func attCompletedBatchSequences(ctx context.Context) (map[string]int64, error) {
	rows, err := core.Model("biz_att_batch").Ctx(ctx).Where("status", "completed").Fields("id,request_json,created_at").All()
	if err != nil {
		return nil, err
	}
	result := map[string]int64{}
	for _, row := range rows {
		var req AttBatchRequest
		_ = json.Unmarshal([]byte(row["request_json"].String()), &req)
		seq := req.QueuedAtUnixNano
		if seq == 0 {
			if at, e := core.ParseTime(row["created_at"].String()); e == nil {
				seq = at.UnixNano()
			}
		}
		result[row["id"].String()] = seq
	}
	return result, nil
}
func attResultDetail(r *ghttp.Request) {
	row, e := queryScope(r.Context()).apply(core.Model("biz_att_result").Ctx(r.Context()), "employee_id").Where("id", r.GetRouter("id").String()).One()
	if e != nil {
		attError(r, e)
		return
	}
	if len(row) == 0 {
		r.Response.WriteStatus(404)
		return
	}
	attOK(r, attResultItem(row, true))
}
func attMonths(r *ghttp.Request) {
	rows, e := core.Model("biz_att_month").Ctx(r.Context()).OrderDesc("month").All()
	if e != nil {
		attError(r, e)
		return
	}
	attOK(r, g.Map{"items": rows})
}
func attMonthAction(r *ghttp.Request) {
	var in struct {
		Action  string `json:"action"`
		BatchID string `json:"batch_id"`
		Reason  string `json:"reason"`
	}
	if e := r.Parse(&in); e != nil {
		attError(r, e)
		return
	}
	if !queryScope(r.Context()).All {
		r.Response.WriteStatus(http.StatusForbidden)
		return
	}
	if strings.TrimSpace(in.Reason) == "" {
		attError(r, fmt.Errorf("请填写操作原因"))
		return
	}
	attMu.Lock()
	defer attMu.Unlock()
	if e := attMonthChange(r.Context(), r.GetRouter("month").String(), in.Action, in.BatchID, in.Reason); e != nil {
		attError(r, e)
		return
	}
	attOK(r, g.Map{"month": r.GetRouter("month").String(), "action": in.Action})
}

func attMonthChange(ctx context.Context, month, action, requestedBatch, reason string) error {
	start, e := attDate(month + "-01")
	if e != nil {
		return e
	}
	end := start.AddDate(0, 1, -1).Format("2006-01-02")
	return core.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		old, e := core.Model("biz_att_month").Ctx(ctx).Where("month", month).One()
		if e != nil {
			return e
		}
		status := old["status"].String()
		batchID := old["batch_id"].String()
		next := ""
		switch action {
		case "review":
			if status == "locked" {
				return fmt.Errorf("月份已锁定，请先重开")
			}
			batch, e := core.Model("biz_att_batch").Ctx(ctx).Where("id", requestedBatch).One()
			if e != nil {
				return e
			}
			if len(batch) == 0 || batch["status"].String() != "completed" || batch["start_date"].String() != month+"-01" || batch["end_date"].String() != end {
				return fmt.Errorf("必须选择完整月份且计算成功的批次")
			}
			if e = attRequireFreshBatch(ctx, requestedBatch); e != nil {
				return e
			}
			count, e := core.Model("biz_att_result").Ctx(ctx).Where("batch_id", requestedBatch).WhereNotIn("status", []string{"calculated", "rest"}).Count()
			if e != nil {
				return e
			}
			if count > 0 {
				return fmt.Errorf("仍有%d条待复核记录，请修正并重新试算", count)
			}
			people, e := attEmployees(ctx)
			if e != nil {
				return e
			}
			var req AttBatchRequest
			_ = json.Unmarshal([]byte(batch["request_json"].String()), &req)
			if len(req.EmployeeIDs) != len(people) || batch["completed_count"].Int() != len(people)*start.AddDate(0, 1, -1).Day() {
				return fmt.Errorf("正式月报必须包含全部适用人员和完整日期")
			}
			for _, id := range req.EmployeeIDs {
				if _, ok := people[id]; !ok {
					return fmt.Errorf("人员范围已变化，请重新计算")
				}
			}
			next = "reviewed"
			batchID = requestedBatch
		case "publish":
			if status != "reviewed" {
				return fmt.Errorf("请先复核完整月报")
			}
			if old["change_pending"].Int() != 0 {
				return fmt.Errorf("复核后证据有变化，请重新试算复核")
			}
			if e = attRequireFreshBatch(ctx, batchID); e != nil {
				return e
			}
			next = "published"
			if e := attProjectClockStates(ctx, batchID, "published"); e != nil {
				return e
			}
		case "lock":
			if status != "published" {
				return fmt.Errorf("请先发布月报")
			}
			if old["change_pending"].Int() != 0 {
				return fmt.Errorf("证据有变化，请重新试算复核")
			}
			if e = attRequireFreshBatch(ctx, batchID); e != nil {
				return e
			}
			next = "locked"
		case "reopen":
			if status != "locked" {
				return fmt.Errorf("只有锁定月份可以重开")
			}
			next = "draft"
		default:
			return fmt.Errorf("操作无效")
		}
		revision := old["revision"].Int() + 1
		data := g.Map{"month": month, "status": next, "batch_id": batchID, "revision": revision, "change_pending": 0, "updated_by": core.UID(ctx), "updated_at": core.Now()}
		if len(old) == 0 {
			_, e = core.Model("biz_att_month").Ctx(ctx).Data(data).Insert()
		} else {
			_, e = core.Model("biz_att_month").Ctx(ctx).Where("month", month).Update(data)
		}
		if e != nil {
			return e
		}
		if _, e = core.Model("biz_att_publication").Ctx(ctx).Data(g.Map{"id": guid.S(), "month": month, "revision": revision, "batch_id": batchID, "action": action, "reason": reason, "created_by": core.UID(ctx), "created_at": core.Now()}).Insert(); e != nil {
			return e
		}
		return attAudit(ctx, "month", month, action, reason, old, data)
	})
}

type AttAdjustmentRequest struct {
	ID              string `json:"id"`
	EmployeeID      string `json:"employee_id"`
	WorkDate        string `json:"work_date"`
	Kind            string `json:"kind"`
	Reason          string `json:"reason"`
	Evidence        string `json:"evidence"`
	At              int64  `json:"at"`
	PunchType       string `json:"punch_type"`
	StartAt         int64  `json:"start_at"`
	EndAt           int64  `json:"end_at"`
	SegmentID       string `json:"segment_id"`
	PunchID         string `json:"punch_id"`
	CreditedMinutes *int64 `json:"credited_minutes"`
	ActualMinutes   *int64 `json:"actual_minutes"`
	ConfirmIssues   bool   `json:"confirm_issues"`
}

func attValidateAdjustment(a AttAdjustmentRequest) error {
	day, e := attDate(a.WorkDate)
	if e != nil {
		return e
	}
	if strings.TrimSpace(a.Reason) == "" || strings.TrimSpace(a.Evidence) == "" {
		return fmt.Errorf("请填写复核原因与依据")
	}
	allowed := map[string]bool{"punch": true, "override": true, "credit": true, "absence": true, "waive_late": true, "waive_early": true, "waive_missing": true, "exclude_punch": true}
	if !allowed[a.Kind] {
		return fmt.Errorf("调整类型无效")
	}
	from, to := day.AddDate(0, 0, -1).Unix(), day.AddDate(0, 0, 3).Unix()
	if a.Kind == "punch" && (a.At < from || a.At >= to || (a.PunchType != "in" && a.PunchType != "out")) {
		return fmt.Errorf("补签须为该工作日前后一夜范围内的有效上下班时间")
	}
	if (a.Kind == "credit" || a.Kind == "absence") && (a.StartAt < from || a.EndAt > to || a.EndAt <= a.StartAt) {
		return fmt.Errorf("人工核定或旷工确认须提供该工作日前后有效的起止区间")
	}
	if a.Kind == "exclude_punch" && strings.TrimSpace(a.PunchID) == "" {
		return fmt.Errorf("排除打卡须选择来源卡")
	}
	for _, v := range []*int64{a.CreditedMinutes, a.ActualMinutes} {
		if v != nil && (*v < 0 || *v > 2880) {
			return fmt.Errorf("核定分钟必须在0至2880之间")
		}
	}
	if a.Kind == "override" && a.CreditedMinutes == nil && a.ActualMinutes == nil && !a.ConfirmIssues {
		return fmt.Errorf("请选择核定内容")
	}
	return nil
}

func attAdjustment(r *ghttp.Request) {
	var a AttAdjustmentRequest
	if e := r.Parse(&a); e != nil {
		attError(r, e)
		return
	}
	if e := attValidateAdjustment(a); e != nil {
		attError(r, e)
		return
	}
	people, e := attEmployees(r.Context())
	if e != nil {
		attError(r, e)
		return
	}
	if _, ok := people[a.EmployeeID]; !ok {
		r.Response.WriteStatus(http.StatusForbidden)
		return
	}
	attMu.Lock()
	defer attMu.Unlock()
	if attLocked(r.Context(), a.WorkDate) {
		attError(r, fmt.Errorf("已锁定月份须先重开"))
		return
	}
	a.ID = guid.S()
	e = core.DB().Transaction(r.Context(), func(ctx context.Context, tx gdb.TX) error {
		_, e := core.Model("biz_att_adjustment").Ctx(ctx).Data(g.Map{"id": a.ID, "employee_id": a.EmployeeID, "work_date": a.WorkDate, "adjustment_json": attJSON(a), "reason": a.Reason, "evidence": a.Evidence, "created_by": core.UID(ctx), "created_at": core.Now()}).Insert()
		if e != nil {
			return e
		}
		return attAudit(ctx, "adjustment", a.ID, "confirm", a.Reason, nil, a)
	})
	if e != nil {
		attError(r, e)
		return
	}
	day, _ := attDate(a.WorkDate)
	_ = AttendanceChanged(context.Background(), day, day, []string{a.EmployeeID})
	attOK(r, a)
}
func attAdjustments(r *ghttp.Request) {
	q := queryScope(r.Context()).apply(core.Model("biz_att_adjustment").Ctx(r.Context()), "employee_id")
	for _, key := range []string{"employee_id", "work_date"} {
		if s := r.GetQuery(key).String(); s != "" {
			q = q.Where(key, s)
		}
	}
	rows, e := q.OrderDesc("created_at").Limit(1000).All()
	if e != nil {
		attError(r, e)
		return
	}
	attOK(r, g.Map{"items": rows})
}
