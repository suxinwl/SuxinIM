package hrm

import (
	"context"
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func ledgerEmployee(t *testing.T, ctx context.Context) string {
	t.Helper()
	id, e := UpsertEmployee(ctx, Employee{Name: "调休测试", EmployeeNo: "L001", DeptID: 10})
	if e != nil {
		t.Fatal(e)
	}
	return id
}
func ledgerCount(t *testing.T, ctx context.Context) int {
	t.Helper()
	n, e := core.Model("biz_att_ledger").Ctx(ctx).Count()
	if e != nil {
		t.Fatal(e)
	}
	return n
}
func ledgerOpening(t *testing.T, ctx context.Context, employee string, minutes int64) {
	t.Helper()
	_, e := attPostLedger(ctx, AttLedgerRequest{EmployeeID: employee, EntryType: "opening", Minutes: minutes, EffectiveDate: "2026-08-01", SourceRef: "opening", Reason: "期初确认", Evidence: "人事签字余额表"})
	if e != nil {
		t.Fatal(e)
	}
}
func ledgerResult(t *testing.T, ctx context.Context, employee, id, batch string, minutes int) {
	t.Helper()
	_, e := core.Model("biz_att_result").Ctx(ctx).Data(g.Map{"id": id, "batch_id": batch, "employee_id": employee, "work_date": "2026-08-02", "rule_id": "rule", "status": "calculated", "employee_json": "{}", "result_json": attJSON(g.Map{"overtime_seconds": minutes * 60}), "input_json": "{}", "created_at": core.Now()}).Insert()
	if e != nil {
		t.Fatal(e)
	}
}
func ledgerMonth(t *testing.T, ctx context.Context, batch, status string) {
	t.Helper()
	n, e := core.Model("biz_att_month").Ctx(ctx).Count()
	if e != nil {
		t.Fatal(e)
	}
	data := g.Map{"month": "2026-08", "status": status, "batch_id": batch, "revision": 1, "change_pending": 0, "updated_by": 1, "updated_at": core.Now()}
	if n == 0 {
		_, e = core.Model("biz_att_month").Ctx(ctx).Data(data).Insert()
	} else {
		_, e = core.Model("biz_att_month").Ctx(ctx).Where("month", "2026-08").Update(data)
	}
	if e != nil {
		t.Fatal(e)
	}
}

func TestAttendanceLedgerPublishedBudgetAcrossVersionsAndNoOverdraft(t *testing.T) {
	ctx := setup(t)
	employee := ledgerEmployee(t, ctx)
	ledgerResult(t, ctx, employee, "result1", "batch1", 120)
	ledgerMonth(t, ctx, "batch1", "reviewed")
	in := AttLedgerRequest{EmployeeID: employee, EntryType: "convert", Minutes: 90, EffectiveDate: "2026-08-02", SourceRef: "convert1", Reason: "人事确认转调休", ResultID: "result1"}
	if _, e := attPostLedger(ctx, in); e == nil {
		t.Fatal("unpublished overtime converted")
	}
	ledgerMonth(t, ctx, "batch1", "published")
	first, e := attPostLedger(ctx, in)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = attPostLedger(ctx, in); e != nil || ledgerCount(t, ctx) != 1 {
		t.Fatal("posting is not idempotent", e)
	}
	ledgerResult(t, ctx, employee, "result2", "batch2", 120)
	ledgerMonth(t, ctx, "batch2", "locked")
	in.SourceRef = "convert2"
	in.ResultID = "result2"
	in.Minutes = 31
	if _, e = attPostLedger(ctx, in); e == nil {
		t.Fatal("new version allowed duplicate overtime quota")
	}
	in.Minutes = 30
	if _, e = attPostLedger(ctx, in); e != nil {
		t.Fatal(e)
	}
	if _, e = attPostLedger(ctx, AttLedgerRequest{EmployeeID: employee, EntryType: "use", Minutes: 120, EffectiveDate: "2026-08-03", SourceRef: "used", Reason: "人事核定调休"}); e != nil {
		t.Fatal(e)
	}
	if _, e = attPostLedger(ctx, AttLedgerRequest{EmployeeID: employee, EntryType: "reverse", EffectiveDate: "2026-08-02", SourceRef: "reversal", ReverseEntryID: first["id"].String(), Reason: "审批撤销需冲销"}); e == nil || !strings.Contains(e.Error(), "余额不足") {
		t.Fatal("consumed credit silently overdrawn", e)
	}
	if ledgerCount(t, ctx) != 3 {
		t.Fatal("rejected reversal changed history")
	}
	ledgerResult(t, ctx, employee, "result3", "batch3", 60)
	ledgerMonth(t, ctx, "batch3", "published")
	in.ResultID = "result3"
	in.SourceRef = "convert3"
	in.Minutes = 1
	if _, e = attPostLedger(ctx, in); e == nil {
		t.Fatal("reduced new-version quota ignored")
	}
}

func TestAttendanceLedgerOpeningEvidenceScopeReversalAndChronology(t *testing.T) {
	ctx := setup(t)
	employee := ledgerEmployee(t, ctx)
	in := AttLedgerRequest{EmployeeID: employee, EntryType: "opening", Minutes: 60, EffectiveDate: "2026-08-02", SourceRef: "opening", Reason: "确认期初"}
	if _, e := attPostLedger(ctx, in); e == nil {
		t.Fatal("opening without evidence accepted")
	}
	in.Evidence = "人事签字表"
	limited := context.WithValue(ctx, hrmScopeKey{}, &hrmScope{employees: map[string]bool{}})
	if _, e := attPostLedger(limited, in); e == nil {
		t.Fatal("out of scope posting accepted")
	}
	first, e := attPostLedger(ctx, in)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = attPostLedger(ctx, AttLedgerRequest{EmployeeID: employee, EntryType: "use", Minutes: 10, EffectiveDate: "2026-08-01", SourceRef: "early", Reason: "提前调休"}); e == nil {
		t.Fatal("future credits funded earlier consumption")
	}
	reverse := AttLedgerRequest{EmployeeID: employee, EntryType: "reverse", EffectiveDate: "2026-08-02", SourceRef: "reverse", Reason: "期初核对冲销", ReverseEntryID: first["id"].String()}
	if _, e = attPostLedger(ctx, reverse); e != nil {
		t.Fatal(e)
	}
	if _, e = attPostLedger(ctx, reverse); e != nil {
		t.Fatal("reversal replay failed", e)
	}
	reverse.SourceRef = "reverse-again"
	if _, e = attPostLedger(ctx, reverse); e == nil {
		t.Fatal("same entry reversed twice")
	}
	old, e := core.Model("biz_att_ledger").Ctx(ctx).Where("id", first["id"]).One()
	if e != nil || old["minutes"].Int() != 60 || ledgerCount(t, ctx) != 2 {
		t.Fatal("source ledger was edited", old, e)
	}
}

func TestReconcileCompLeaveIdempotenceRefundReapprovalAndInsufficiency(t *testing.T) {
	ctx := setup(t)
	employee := ledgerEmployee(t, ctx)
	ledgerOpening(t, ctx, employee, 120)
	in := Leave{EmployeeID: employee, Source: "wecom", ExternalRef: "SP-COMP", ApprovalStatus: "approved", LeaveType: "调休", StartTime: "2026-08-03 09:00:00", EndTime: "2026-08-03 10:00:00", Duration: 1, Reason: "调休", RawJSON: "{}"}
	id, e := UpsertLeave(ctx, in)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		issues, e := ReconcileCompLeave(ctx, employee)
		if e != nil || len(issues) > 0 {
			t.Fatal(issues, e)
		}
	}
	if ledgerCount(t, ctx) != 2 {
		t.Fatal("repeated approved leave duplicated debit")
	}
	for _, state := range []string{"canceled", "rejected"} {
		if _, e = core.Model("biz_hrm_attendance_leave").Ctx(ctx).Where("id", id).Update(g.Map{"approval_status": state}); e != nil {
			t.Fatal(e)
		}
		issues, e := ReconcileCompLeave(ctx, employee)
		if e != nil || len(issues) > 0 {
			t.Fatal(issues, e)
		}
	}
	if ledgerCount(t, ctx) != 3 {
		t.Fatal("cancellation not refunded exactly once")
	}
	if _, e = UpsertLeave(ctx, in); e != nil {
		t.Fatal(e)
	}
	if issues, e := ReconcileCompLeave(ctx, employee); e != nil || len(issues) > 0 {
		t.Fatal(issues, e)
	}
	if ledgerCount(t, ctx) != 4 {
		t.Fatal("reapproval did not consume again")
	}
	in.ExternalRef = "SP-NO-BALANCE"
	in.StartTime = "2026-08-04 09:00:00"
	in.EndTime = "2026-08-04 11:00:00"
	in.Duration = 2
	if _, e = UpsertLeave(ctx, in); e != nil {
		t.Fatal(e)
	}
	issues, e := ReconcileCompLeave(ctx, employee)
	if e != nil || len(issues) != 1 || !strings.Contains(issues[0], "余额不足") || ledgerCount(t, ctx) != 4 {
		t.Fatal("insufficient comp leave was not a review issue", issues, e)
	}
}

func TestReconcileCompLeaveReadsSelectedChineseLabel(t *testing.T) {
	ctx := setup(t)
	employee := ledgerEmployee(t, ctx)
	ledgerOpening(t, ctx, employee, 60)
	raw := `{"apply_data":{"contents":[{"control":"Vacation","value":{"vacation":{"selector":{"options":[{"key":"encoded-id","value":[{"lang":"zh_CN","text":"调休"}]}]}}}}]}}`
	_, e := UpsertLeave(ctx, Leave{EmployeeID: employee, Source: "wecom", ExternalRef: "SP-LABEL", ApprovalStatus: "approved", LeaveType: "encoded-id", StartTime: "2026-08-05 09:00:00", EndTime: "2026-08-05 10:00:00", Duration: 1, RawJSON: raw})
	if e != nil {
		t.Fatal(e)
	}
	issues, e := ReconcileCompLeave(ctx, employee)
	if e != nil || len(issues) > 0 || ledgerCount(t, ctx) != 2 {
		t.Fatal("selected Chinese comp label not reconciled", issues, e)
	}
}

func TestReconcileCompLeaveDateEditPreservesHistoryAndChronologicalBalance(t *testing.T) {
	ctx := setup(t)
	employee := ledgerEmployee(t, ctx)
	_, e := attPostLedger(ctx, AttLedgerRequest{EmployeeID: employee, EntryType: "opening", Minutes: 60, EffectiveDate: "2026-08-04", SourceRef: "late-opening", Reason: "确认余额", Evidence: "签字单"})
	if e != nil {
		t.Fatal(e)
	}
	in := Leave{EmployeeID: employee, Source: "wecom", ExternalRef: "SP-DATE", ApprovalStatus: "approved", LeaveType: "调休", StartTime: "2026-08-05 09:00:00", EndTime: "2026-08-05 10:00:00", Duration: 1}
	if _, e = UpsertLeave(ctx, in); e != nil {
		t.Fatal(e)
	}
	if issues, e := ReconcileCompLeaveDetailed(ctx, employee); e != nil || len(issues) > 0 {
		t.Fatal(issues, e)
	}
	in.StartTime, in.EndTime = "2026-08-03 09:00:00", "2026-08-03 10:00:00"
	if _, e = UpsertLeave(ctx, in); e != nil {
		t.Fatal(e)
	}
	issues, e := ReconcileCompLeaveDetailed(ctx, employee)
	if e != nil || len(issues) != 1 || !strings.Contains(issues[0].Message, "余额不足") || issues[0].Start.Format("2006-01-02") != "2026-08-03" || ledgerCount(t, ctx) != 2 {
		t.Fatal("invalid earlier date silently moved debit", issues, e)
	}
	in.StartTime, in.EndTime = "2026-08-06 09:00:00", "2026-08-06 10:00:00"
	if _, e = UpsertLeave(ctx, in); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if issues, e := ReconcileCompLeaveDetailed(ctx, employee); e != nil || len(issues) > 0 {
			t.Fatal(issues, e)
		}
	}
	if ledgerCount(t, ctx) != 4 {
		t.Fatal("date edit did not append one refund and one debit exactly once")
	}
	old, _ := core.Model("biz_att_ledger").Ctx(ctx).Where("entry_type", "use").Where("effective_date", "2026-08-05").Count()
	if old != 1 {
		t.Fatal("original debit was removed")
	}
	for _, date := range []string{"2026-08-05", "2026-08-06"} {
		sum, e := core.Model("biz_att_ledger").Ctx(ctx).Where("effective_date", date).Sum("minutes")
		want := float64(0)
		if date == "2026-08-06" {
			want = -60
		}
		if e != nil || sum != want {
			t.Fatalf("date %s got %v want %v: %v", date, sum, want, e)
		}
	}
}

func TestAttendanceLedgerCannotRefundBeforeOriginalDebit(t *testing.T) {
	ctx := setup(t)
	employee := ledgerEmployee(t, ctx)
	ledgerOpening(t, ctx, employee, 60)
	used, e := attPostLedger(ctx, AttLedgerRequest{EmployeeID: employee, EntryType: "use", Minutes: 60, EffectiveDate: "2026-08-03", SourceRef: "usage-date", Reason: "核对调休"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = attPostLedger(ctx, AttLedgerRequest{EmployeeID: employee, EntryType: "reverse", EffectiveDate: "2026-08-02", SourceRef: "refund-before-use", ReverseEntryID: used["id"].String(), Reason: "冲销"})
	if e == nil || !strings.Contains(e.Error(), "不得早于") || ledgerCount(t, ctx) != 2 {
		t.Fatal("refund fabricated credit before its original debit", e)
	}
}

func TestReconcileCompLeaveRefundFundsReplacementInOneRun(t *testing.T) {
	ctx := setup(t)
	employee := ledgerEmployee(t, ctx)
	ledgerOpening(t, ctx, employee, 60)
	old := Leave{EmployeeID: employee, Source: "wecom", ExternalRef: "SP-OLD", ApprovalStatus: "approved", LeaveType: "调休", StartTime: "2026-08-05 09:00:00", EndTime: "2026-08-05 10:00:00", Duration: 1}
	if _, e := UpsertLeave(ctx, old); e != nil {
		t.Fatal(e)
	}
	if issues, e := ReconcileCompLeaveDetailed(ctx, employee); e != nil || len(issues) > 0 {
		t.Fatal(issues, e)
	}
	old.ApprovalStatus = "canceled"
	if _, e := UpsertLeave(ctx, old); e != nil {
		t.Fatal(e)
	}
	replacement := old
	replacement.ExternalRef = "SP-NEW"
	replacement.ApprovalStatus = "approved"
	replacement.StartTime = "2026-08-03 09:00:00"
	replacement.EndTime = "2026-08-03 10:00:00"
	if _, e := UpsertLeave(ctx, replacement); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if issues, e := ReconcileCompLeaveDetailed(ctx, employee); e != nil || len(issues) > 0 {
			t.Fatal("replacement falsely lacks balance until second calculation", issues, e)
		}
	}
	if ledgerCount(t, ctx) != 4 {
		t.Fatal("refund/replacement must each append once")
	}
	sum, e := core.Model("biz_att_ledger").Ctx(ctx).Where("employee_id", employee).Sum("minutes")
	if e != nil || sum != 0 {
		t.Fatal("replacement ledger balance", sum, e)
	}
}
