package hrm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/approvalview"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

type AttLedgerRequest struct {
	EmployeeID     string `json:"employee_id"`
	EntryType      string `json:"entry_type"`
	Minutes        int64  `json:"minutes"`
	EffectiveDate  string `json:"effective_date"`
	SourceRef      string `json:"source_ref"`
	Reason         string `json:"reason"`
	ResultID       string `json:"result_id,omitempty"`
	ReverseEntryID string `json:"reverse_entry_id,omitempty"`
	Evidence       string `json:"evidence,omitempty"`
}
type ledgerSource struct {
	Origin          string            `json:"origin"`
	Request         *AttLedgerRequest `json:"request,omitempty"`
	WorkDate        string            `json:"work_date,omitempty"`
	ResultID        string            `json:"result_id,omitempty"`
	ReversesEntryID string            `json:"reverses_entry_id,omitempty"`
	LeaveID         string            `json:"leave_id,omitempty"`
	Revision        int               `json:"revision,omitempty"`
	Snapshot        json.RawMessage   `json:"snapshot,omitempty"`
}

type ledgerInsufficientError struct{ date string }

func (e *ledgerInsufficientError) Error() string {
	return fmt.Sprintf("调休余额不足（%s），该笔操作未入账；需人事复核，不能透支", e.date)
}

func decodeLedgerSource(row gdb.Record) ledgerSource {
	var s ledgerSource
	_ = json.Unmarshal([]byte(row["source_json"].String()), &s)
	return s
}

func attLedger(r *ghttp.Request) {
	ctx := r.Context()
	query := queryScope(ctx).apply(core.Model("biz_att_ledger").Ctx(ctx), "employee_id")
	employee := strings.TrimSpace(r.GetQuery("employee_id").String())
	if employee != "" {
		query = query.Where("employee_id", employee)
	}
	rows, err := query.Order("effective_date,created_at,id").All()
	if err != nil {
		attError(r, err)
		return
	}
	balances := map[string]int64{}
	for _, row := range rows {
		balances[row["employee_id"].String()] += row["minutes"].Int64()
	}
	ids := make([]string, 0, len(balances))
	for id := range balances {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	list := []g.Map{}
	for _, id := range ids {
		list = append(list, g.Map{"employee_id": id, "balance_minutes": balances[id]})
	}
	// Balance includes all rows, while optional date filters affect the visible
	// ledger only, so paging/filtering cannot fabricate an account balance.
	items := gdb.Result{}
	from, to := r.GetQuery("start_date").String(), r.GetQuery("end_date").String()
	for i := len(rows) - 1; i >= 0; i-- {
		row := rows[i]
		d := row["effective_date"].String()
		if (from == "" || d >= from) && (to == "" || d <= to) {
			items = append(items, row)
		}
	}
	out := g.Map{"items": items, "balances": list}
	if employee != "" {
		out["balance_minutes"] = balances[employee]
	}
	attOK(r, out)
}
func attLedgerWrite(r *ghttp.Request) {
	var in AttLedgerRequest
	if err := r.Parse(&in); err != nil {
		attError(r, err)
		return
	}
	row, err := attPostLedger(r.Context(), in)
	if err != nil {
		attError(r, err)
		return
	}
	attOK(r, row)
}

// All postings append immutable entries. A reversal never edits its source.
func attPostLedger(ctx context.Context, in AttLedgerRequest) (gdb.Record, error) {
	in.SourceRef = strings.TrimSpace(in.SourceRef)
	in.Reason = strings.TrimSpace(in.Reason)
	if !queryScope(ctx).allowsEmployee(in.EmployeeID) {
		return nil, fmt.Errorf("无权维护该员工的调休台账")
	}
	if err := employeeExists(ctx, in.EmployeeID); err != nil {
		return nil, err
	}
	if _, err := attDate(in.EffectiveDate); err != nil {
		return nil, err
	}
	if in.SourceRef == "" || len(in.SourceRef) > 192 || strings.HasPrefix(in.SourceRef, "comp-leave:") {
		return nil, fmt.Errorf("请填写有效且独立的业务来源编号")
	}
	if in.Reason == "" || len(in.Reason) > 1000 {
		return nil, fmt.Errorf("请填写台账调整原因（不超过1000字节）")
	}
	if in.EntryType != "opening" && in.EntryType != "convert" && in.EntryType != "use" && in.EntryType != "reverse" {
		return nil, fmt.Errorf("台账类型无效")
	}
	if in.EntryType != "reverse" && (in.Minutes <= 0 || in.Minutes > 2147483647) {
		return nil, fmt.Errorf("分钟数必须为有效正整数")
	}
	if in.EntryType == "opening" && strings.TrimSpace(in.Evidence) == "" {
		return nil, fmt.Errorf("期初余额必须填写导入依据")
	}
	attMu.Lock()
	defer attMu.Unlock()
	var saved gdb.Record
	err := core.DB().Transaction(ctx, func(txctx context.Context, _ gdb.TX) error {
		old, e := core.Model("biz_att_ledger").Ctx(txctx).Where("source_ref", in.SourceRef).One()
		if e != nil {
			return e
		}
		if len(old) > 0 {
			source := decodeLedgerSource(old)
			if source.Request == nil || attJSON(*source.Request) != attJSON(in) {
				return fmt.Errorf("来源编号已用于其他台账内容")
			}
			saved = old
			return nil
		}
		source := ledgerSource{Origin: "manual", Request: &in}
		minutes := in.Minutes
		switch in.EntryType {
		case "convert":
			if in.ResultID == "" {
				return fmt.Errorf("转入调休须选择已发布的加班考勤结果")
			}
			result, e := core.Model("biz_att_result").Ctx(txctx).Where("id", in.ResultID).Where("employee_id", in.EmployeeID).One()
			if e != nil {
				return e
			}
			if len(result) == 0 {
				return fmt.Errorf("该员工的考勤结果不存在")
			}
			if result["status"].String() != "calculated" && result["status"].String() != "rest" {
				return fmt.Errorf("考勤结果尚未完成复核")
			}
			date := result["work_date"].String()
			if _, e = attDate(date); e != nil {
				return e
			}
			if in.EffectiveDate < date {
				return fmt.Errorf("加班转换入账日不得早于工作日期")
			}
			month, e := core.Model("biz_att_month").Ctx(txctx).Where("month", date[:7]).One()
			if e != nil {
				return e
			}
			if len(month) == 0 || (month["status"].String() != "published" && month["status"].String() != "locked") || month["batch_id"].String() != result["batch_id"].String() || month["change_pending"].Int() != 0 {
				return fmt.Errorf("仅可转换当前已发布或锁定且没有待处理变化的考勤结果")
			}
			var calculated eng.Result
			if json.Unmarshal([]byte(result["result_json"].String()), &calculated) != nil || calculated.OvertimeSeconds < 0 {
				return fmt.Errorf("考勤结果无法核验")
			}
			used, e := ledgerConvertedMinutes(txctx, in.EmployeeID, date)
			if e != nil {
				return e
			}
			budget := calculated.OvertimeSeconds / 60
			if used < 0 || used+minutes > budget {
				return fmt.Errorf("该员工当日已转换%d分钟，可确认加班共%d分钟；不足或旧版本额度变化，需人事复核", used, budget)
			}
			source.WorkDate = date
			source.ResultID = in.ResultID
			source.Snapshot = json.RawMessage(result["result_json"].String())
		case "use":
			minutes = -minutes
		case "reverse":
			if in.ReverseEntryID == "" {
				return fmt.Errorf("请选择需冲销的原流水")
			}
			original, e := core.Model("biz_att_ledger").Ctx(txctx).Where("id", in.ReverseEntryID).Where("employee_id", in.EmployeeID).One()
			if e != nil {
				return e
			}
			if len(original) == 0 || original["entry_type"].String() == "reverse" {
				return fmt.Errorf("原流水不存在或不能再次冲销")
			}
			if in.EffectiveDate < original["effective_date"].String() {
				return fmt.Errorf("冲销日期不得早于原流水生效日期")
			}
			origin := decodeLedgerSource(original)
			if origin.Origin == "comp_leave" {
				return fmt.Errorf("审批消费应通过原审批撤销或修改，不能人工冲销")
			}
			all, e := core.Model("biz_att_ledger").Ctx(txctx).Where("employee_id", in.EmployeeID).Where("entry_type", "reverse").All()
			if e != nil {
				return e
			}
			for _, row := range all {
				if decodeLedgerSource(row).ReversesEntryID == in.ReverseEntryID {
					return fmt.Errorf("该流水已冲销")
				}
			}
			minutes = -original["minutes"].Int64()
			if in.Minutes != 0 && in.Minutes != absLedger(minutes) {
				return fmt.Errorf("冲销必须覆盖原流水全部分钟数")
			}
			source.ReversesEntryID = in.ReverseEntryID
			source.WorkDate = origin.WorkDate
			source.ResultID = origin.ResultID
			source.Snapshot = json.RawMessage(attJSON(original))
		}
		if e := ledgerCanApply(txctx, in.EmployeeID, in.EffectiveDate, minutes); e != nil {
			return e
		}
		id, e := appendLedger(txctx, in.EmployeeID, in.EntryType, minutes, in.EffectiveDate, in.SourceRef, in.Reason, source)
		if e != nil {
			return e
		}
		saved, e = core.Model("biz_att_ledger").Ctx(txctx).Where("id", id).One()
		return e
	})
	return saved, err
}
func absLedger(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}
func ledgerConvertedMinutes(ctx context.Context, employee, date string) (int64, error) {
	rows, e := core.Model("biz_att_ledger").Ctx(ctx).Where("employee_id", employee).All()
	if e != nil {
		return 0, e
	}
	var total int64
	for _, row := range rows {
		source := decodeLedgerSource(row)
		if source.WorkDate == date && source.ResultID != "" && (row["entry_type"].String() == "convert" || row["entry_type"].String() == "reverse") {
			total += row["minutes"].Int64()
		}
	}
	return total, nil
}
func ledgerCanApply(ctx context.Context, employee, date string, minutes int64) error {
	return ledgerCanApplyDeltas(ctx, employee, map[string]int64{date: minutes})
}
func ledgerCanApplyDeltas(ctx context.Context, employee string, changes map[string]int64) error {
	rows, e := core.Model("biz_att_ledger").Ctx(ctx).Where("employee_id", employee).Fields("effective_date,minutes").All()
	if e != nil {
		return e
	}
	days := map[string]int64{}
	for date, minutes := range changes {
		days[date] += minutes
	}
	for _, row := range rows {
		days[row["effective_date"].String()] += row["minutes"].Int64()
	}
	keys := []string{}
	for day := range days {
		keys = append(keys, day)
	}
	sort.Strings(keys)
	var balance int64
	for _, day := range keys {
		balance += days[day]
		if balance < 0 {
			return &ledgerInsufficientError{date: day}
		}
	}
	return nil
}
func appendLedger(ctx context.Context, employee, kind string, minutes int64, date, ref, reason string, source ledgerSource) (string, error) {
	id := guid.S()
	data := g.Map{"id": id, "employee_id": employee, "entry_type": kind, "minutes": minutes, "effective_date": date, "source_ref": ref, "source_json": attJSON(source), "reason": reason, "created_by": core.UID(ctx), "created_at": core.Now()}
	if _, e := core.Model("biz_att_ledger").Ctx(ctx).Data(data).Insert(); e != nil {
		return "", e
	}
	return id, attAudit(ctx, "ledger", id, kind, reason, nil, data)
}

// ReconcileCompLeave consumes only an explicitly named 调休 leave. Overtime is
// never converted automatically. Repeated reconciliations append only the net
// change; withdrawal/refusal refunds the prior consumption. Missing source
// rows and insufficient balances remain review issues, never silent overdraft.
type CompLeaveIssue struct {
	LeaveID string    `json:"leave_id"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Message string    `json:"message"`
}

func compLeaveIssue(leave gdb.Record, message string) CompLeaveIssue {
	start, _ := core.ParseTime(leave["start_time"].String())
	end, _ := core.ParseTime(leave["end_time"].String())
	return CompLeaveIssue{LeaveID: leave["id"].String(), Start: start, End: end, Message: message}
}
func compLeaveTarget(leave gdb.Record) (isComp bool, desired int64, valid bool) {
	label := strings.TrimSpace(leave["leave_type"].String())
	if selected := approvalview.LeaveTypeLabel(leave["raw_json"].String(), ""); selected != "" {
		label = selected
	}
	isComp = strings.Contains(label, "调休")
	if !isComp || leave["approval_status"].String() != "approved" {
		return isComp, 0, true
	}
	v := leave["duration"].Float64() * 60
	if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > 2147483647 {
		return isComp, 0, false
	}
	return isComp, -int64(math.Round(v)), true
}
func ReconcileCompLeave(ctx context.Context, employeeID string) ([]string, error) {
	details, err := ReconcileCompLeaveDetailed(ctx, employeeID)
	issues := []string{}
	for _, issue := range details {
		issues = append(issues, issue.Message)
	}
	return issues, err
}
func ReconcileCompLeaveDetailed(ctx context.Context, employeeID string) (issues []CompLeaveIssue, result error) {
	issues = []CompLeaveIssue{}
	if !queryScope(ctx).allowsEmployee(employeeID) {
		return issues, fmt.Errorf("无权核对该员工的调休台账")
	}
	attMu.Lock()
	defer attMu.Unlock()
	result = core.DB().Transaction(ctx, func(txctx context.Context, _ gdb.TX) error {
		rows, e := core.Model("biz_hrm_attendance_leave").Ctx(txctx).Where("employee_id", employeeID).Order("start_time,id").All()
		if e != nil {
			return e
		}
		entries, e := core.Model("biz_att_ledger").Ctx(txctx).Where("employee_id", employeeID).All()
		if e != nil {
			return e
		}
		net := map[string]int64{}
		positions := map[string]map[string]int64{}
		previousSources := map[string]ledgerSource{}
		revisions := map[string]int{}
		seen := map[string]bool{}
		for _, row := range entries {
			source := decodeLedgerSource(row)
			if source.Origin == "comp_leave" && source.LeaveID != "" {
				net[source.LeaveID] += row["minutes"].Int64()
				if positions[source.LeaveID] == nil {
					positions[source.LeaveID] = map[string]int64{}
				}
				positions[source.LeaveID][row["effective_date"].String()] += row["minutes"].Int64()
				if source.Revision > revisions[source.LeaveID] {
					revisions[source.LeaveID] = source.Revision
					previousSources[source.LeaveID] = source
				}
			}
		}
		// Withdrawals and shortened approvals release their prior dated debit
		// before new approvals are checked. Otherwise one synchronization can
		// leave a false insufficient-balance issue until another calculation.
		priority := func(leave gdb.Record) int {
			_, desired, valid := compLeaveTarget(leave)
			if !valid {
				return 1
			}
			delta := desired - net[leave["id"].String()]
			if delta > 0 {
				return 0
			}
			if delta < 0 {
				return 2
			}
			return 1
		}
		sort.SliceStable(rows, func(i, j int) bool { return priority(rows[i]) < priority(rows[j]) })
		for _, leave := range rows {
			id := leave["id"].String()
			seen[id] = true
			isComp, desired, valid := compLeaveTarget(leave)
			if !isComp && len(positions[id]) == 0 {
				continue
			}
			ref := leave["external_ref"].String()
			if ref == "" {
				ref = "人工调休申请"
			}
			if !valid {
				issues = append(issues, compLeaveIssue(leave, fmt.Sprintf("调休审批%s时长无效，需人工复核", ref)))
				continue
			}
			// A date edit must relocate the debit even when duration is unchanged.
			// Refund each old date and consume on the current date as one atomic
			// posting set, preserving the original entries and chronological balance.
			changes := map[string]int64{}
			for date, minutes := range positions[id] {
				changes[date] -= minutes
			}
			if desired != 0 {
				start, err := core.ParseTime(leave["start_time"].String())
				if err != nil {
					issues = append(issues, compLeaveIssue(leave, fmt.Sprintf("调休审批%s日期无效，需人工复核", ref)))
					continue
				}
				changes[start.In(core.Location).Format("2006-01-02")] += desired
			}
			dates := []string{}
			for date, delta := range changes {
				if delta != 0 {
					dates = append(dates, date)
				}
			}
			if len(dates) == 0 {
				continue
			}
			if e = ledgerCanApplyDeltas(txctx, employeeID, changes); e != nil {
				var insufficient *ledgerInsufficientError
				if !errors.As(e, &insufficient) {
					return e
				}
				issues = append(issues, compLeaveIssue(leave, fmt.Sprintf("调休审批%s：%s", ref, e.Error())))
				continue
			}
			sort.Strings(dates)
			for _, date := range dates {
				delta := changes[date]
				kind := "use"
				reason := "已通过调休审批扣减"
				if delta > 0 {
					kind = "reverse"
					reason = "调休审批撤销、驳回或修改返还"
				}
				revision := revisions[id] + 1
				source := ledgerSource{Origin: "comp_leave", LeaveID: id, Revision: revision, Snapshot: json.RawMessage(attJSON(leave))}
				key := "comp-leave:" + externalID("comp-leave", fmt.Sprintf("%s:%d", id, revision))
				if _, e = appendLedger(txctx, employeeID, kind, delta, date, key, reason, source); e != nil {
					return e
				}
				revisions[id] = revision
			}
			net[id] = desired
		}
		for id, value := range net {
			if value != 0 && !seen[id] {
				var old struct {
					Start string `json:"start_time"`
					End   string `json:"end_time"`
				}
				_ = json.Unmarshal(previousSources[id].Snapshot, &old)
				start, _ := core.ParseTime(old.Start)
				end, _ := core.ParseTime(old.End)
				issues = append(issues, CompLeaveIssue{LeaveID: id, Start: start, End: end, Message: "已扣减调休的原审批记录缺失，需人工复核"})
			}
		}
		return nil
	})
	sort.Slice(issues, func(i, j int) bool { return issues[i].LeaveID < issues[j].LeaveID })
	return issues, result
}
