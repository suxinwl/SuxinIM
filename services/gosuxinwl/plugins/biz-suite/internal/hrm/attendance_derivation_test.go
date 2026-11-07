package hrm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

// Hold the first employee at a known point, queue a real writer behind its
// lock, and keep the final employee paused. The writer must finish while the
// batch is still in progress, not merely after all employee chunks complete.
// Several intermediate chunks may run first: sync.Mutex is not a FIFO lock.
func attConcurrentDerivation(t *testing.T, ctx context.Context, request AttBatchRequest, write func() error) (string, error) {
	t.Helper()
	first, releaseFirst, releaseSecond := make(chan struct{}), make(chan struct{}), make(chan struct{})
	type outcome struct {
		fingerprint string
		err         error
	}
	finished := make(chan outcome, 1)
	go func() {
		calls := 0
		fingerprint, err := attDeriveRosterChunks(ctx, request, func(c context.Context, chunk AttBatchRequest) error {
			calls++
			if calls == len(request.EmployeeIDs) {
				<-releaseSecond
			}
			err := attEnsureDerivedRostersLocked(c, chunk)
			if calls == 1 {
				close(first)
				<-releaseFirst
			}
			return err
		})
		finished <- outcome{fingerprint, err}
	}()
	select {
	case <-first:
	case result := <-finished:
		t.Fatalf("worker did not reach first employee: %v", result.err)
	case <-time.After(3 * time.Second):
		t.Fatal("worker did not reach first employee")
	}
	attempted, written := make(chan struct{}), make(chan error, 1)
	go func() {
		close(attempted)
		attMu.Lock()
		defer attMu.Unlock()
		written <- write()
	}()
	<-attempted
	// Give the queued writer a turn before releasing the controlled chunk.
	// This tests mutex hand-off, not a wall-clock performance threshold.
	time.Sleep(20 * time.Millisecond)
	close(releaseFirst)
	var writeErr error
	select {
	case writeErr = <-written:
	case <-time.After(3 * time.Second):
		close(releaseSecond)
		<-finished
		<-written
		t.Fatal("a policy/month writer remained blocked by the whole batch")
	}
	close(releaseSecond)
	result := <-finished
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	return result.fingerprint, result.err
}

func attConcurrentFixture(t *testing.T) (context.Context, AttBatchRequest, AttRule) {
	t.Helper()
	ctx := context.WithValue(setup(t), "uid", int64(1))
	employees := []string{}
	for i := 1; i <= 6; i++ {
		employees = append(employees, attDirtyEmployee(t, ctx, fmt.Sprintf("第%d位员工", i)))
	}
	rule := attAutoFixtureRule()
	rule.Status = "published"
	attAutoStoreRule(t, ctx, rule)
	return ctx, AttBatchRequest{StartDate: "2026-10-12", EndDate: "2026-10-12", EmployeeIDs: employees}, rule
}

func TestDerivedRosterChunksAllowDraftAndPreserveManualEdits(t *testing.T) {
	ctx, request, rule := attConcurrentFixture(t)
	lastEmployee := request.EmployeeIDs[len(request.EmployeeIDs)-1]
	_, err := attConcurrentDerivation(t, ctx, request, func() error {
		draft := rule
		draft.ID, draft.Status, draft.Version = "concurrent-draft", "draft", 2
		_, err := core.Model("biz_att_rule").Ctx(ctx).Data(g.Map{"id": draft.ID, "family_id": draft.FamilyID, "version": draft.Version, "name": "后台试算期间保存的草稿", "status": "draft", "effective_from": draft.EffectiveFrom, "config_json": attJSON(draft.Config), "scope_json": attJSON(draft.Scope), "conflicts_json": "[]", "source_json": "{}", "created_by": 1, "created_at": core.Now(), "updated_at": core.Now()}).Insert()
		if err != nil {
			return err
		}
		return attPutRosters(ctx, []AttRoster{{EmployeeID: lastEmployee, WorkDate: request.StartDate, RuleID: rule.ID, Category: eng.Restday, Source: "manual"}}, "员工之间人工确认休息日")
	})
	if err != nil {
		t.Fatal("an unrelated draft/manual edit invalidated the worker", err)
	}
	row, err := core.Model("biz_att_roster").Ctx(ctx).Where("employee_id", lastEmployee).Where("work_date", request.StartDate).One()
	roster := attDecodeRoster(row)
	if err != nil || roster.Source != "manual" || roster.Category != eng.Restday || roster.ShiftID != "" {
		t.Fatalf("worker overwrote a concurrent manual assignment: %+v %v", roster, err)
	}
}

func TestDerivedRosterChunksRejectConcurrentPublication(t *testing.T) {
	ctx, request, rule := attConcurrentFixture(t)
	_, err := attConcurrentDerivation(t, ctx, request, func() error {
		// Same-family new publication is included even while the old immutable
		// published version is retained for historical evidence.
		_, err := core.Model("biz_att_rule").Ctx(ctx).Data(g.Map{"id": "concurrent-published", "family_id": rule.FamilyID, "version": 2, "name": rule.Name, "status": "published", "effective_from": rule.EffectiveFrom, "config_json": attJSON(rule.Config), "scope_json": attJSON(rule.Scope), "conflicts_json": "[]", "source_json": "{}", "created_by": 1, "created_at": core.Now(), "updated_at": core.Now()}).Insert()
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "发生变化") {
		t.Fatalf("mixed policy versions were accepted: %v", err)
	}
	if count, _ := core.Model("biz_att_roster").Ctx(ctx).Where("employee_id", request.EmployeeIDs[len(request.EmployeeIDs)-1]).Count(); count != 0 {
		t.Fatal("worker continued deriving with stale policy inputs")
	}
}

func TestDerivedRosterChunksSerializeWithAtomicMonthClose(t *testing.T) {
	ctx, request, rule := attConcurrentFixture(t)
	request.StartDate, request.EndDate = "2026-10-01", "2026-10-31"
	for i := range rule.Config.Cycle.Weekdays {
		rule.Config.Cycle.Weekdays[i] = AttCalendarDay{Category: eng.Restday}
	}
	if _, err := core.Model("biz_att_rule").Ctx(ctx).Where("id", rule.ID).Update(g.Map{"config_json": attJSON(rule.Config)}); err != nil {
		t.Fatal(err)
	}
	batch, err := attQueueBatch(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	attProcessQueue(ctx)
	row, _ := core.Model("biz_att_batch").Ctx(ctx).Where("id", batch).One()
	if row["status"].String() != "completed" {
		t.Fatalf("month setup failed: %v", row)
	}
	audits, _ := core.Model("biz_att_audit").Ctx(ctx).Where("entity", "roster").Count()
	_, err = attConcurrentDerivation(t, ctx, request, func() error {
		return core.DB().Transaction(ctx, func(c context.Context, _ gdb.TX) error {
			for _, action := range []string{"review", "publish", "lock"} {
				if err := attMonthChange(c, "2026-10", action, batch, "并发月结核对"); err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	month, _ := core.Model("biz_att_month").Ctx(ctx).Where("month", "2026-10").One()
	if month["status"].String() != "locked" || month["revision"].Int() != 3 || month["change_pending"].Int() != 0 {
		t.Fatalf("worker crossed the atomic month close: %v", month)
	}
	if count, _ := core.Model("biz_att_publication").Ctx(ctx).Count(); count != 3 {
		t.Fatalf("month close audit steps incomplete: %d", count)
	}
	if after, _ := core.Model("biz_att_audit").Ctx(ctx).Where("entity", "roster").Count(); after != audits {
		t.Fatalf("locked rosters were mutated: %d -> %d", audits, after)
	}
}

func TestDerivedInputsChangeBeforeInitialFingerprintCannotBeCertified(t *testing.T) {
	ctx, request, rule := attConcurrentFixture(t)
	baseline, err := attPrepareDerivedRosters(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	rule.Config.Rule.LateGraceMinutes++
	if _, err = core.Model("biz_att_rule").Ctx(ctx).Where("id", rule.ID).Update(g.Map{"config_json": attJSON(rule.Config)}); err != nil {
		t.Fatal(err)
	}
	if _, err := attInputFingerprint(ctx, request); err != nil {
		t.Fatal(err)
	}
	if err := attVerifyDerivedInputs(ctx, request, baseline); err == nil {
		t.Fatal("a new initial fingerprint certified rosters derived before the publication")
	}
}

func TestDerivedRosterSourceChangesCannotCertifyOldSelections(t *testing.T) {
	for _, kind := range []string{"clock", "approval", "adjustment"} {
		t.Run(kind, func(t *testing.T) {
			ctx, request, rule := attConcurrentFixture(t)
			rule.Config.ShiftSelection = "auto"
			if _, err := core.Model("biz_att_rule").Ctx(ctx).Where("id", rule.ID).Update(g.Map{"config_json": attJSON(rule.Config)}); err != nil {
				t.Fatal(err)
			}
			before, err := attSourceFingerprint(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			employee := request.EmployeeIDs[0]
			_, err = attConcurrentDerivation(t, ctx, request, func() error {
				switch kind {
				case "clock":
					_, err := UpsertClock(ctx, Clock{EmployeeID: employee, Source: "wecom", ExternalRef: "concurrent-clock", ClockTime: "2026-10-12 08:00:00", ClockType: "in"})
					return err
				case "approval":
					_, err := UpsertLeave(ctx, Leave{EmployeeID: employee, Source: "wecom", ExternalRef: "concurrent-approval", ApprovalStatus: "approved", LeaveType: "事假", StartTime: "2026-10-12 08:00:00", EndTime: "2026-10-12 12:00:00", Duration: 4})
					return err
				default:
					a := AttAdjustmentRequest{ID: "concurrent-adjustment", EmployeeID: employee, WorkDate: request.StartDate, Kind: "punch", At: tm("2026-10-12 08:00:00").Unix(), PunchType: "in", Reason: "同时补签", Evidence: "核对记录"}
					_, err := core.Model("biz_att_adjustment").Ctx(ctx).Data(g.Map{"id": a.ID, "employee_id": employee, "work_date": a.WorkDate, "adjustment_json": attJSON(a), "reason": a.Reason, "evidence": a.Evidence, "created_by": 1, "created_at": core.Now()}).Insert()
					return err
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			if fingerprint, err := attFinishDerivationSnapshot(ctx, request, before); err == nil || fingerprint != "" || !strings.Contains(err.Error(), "业务证据发生变化") {
				t.Fatalf("%s changed after the first shift selection but received a certificate: %q %v", kind, fingerprint, err)
			}
		})
	}
}

func TestDerivedRosterSourceFingerprintIgnoresOnlyGeneratedRosters(t *testing.T) {
	ctx, request, rule := attConcurrentFixture(t)
	before, err := attSourceFingerprint(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if err = attEnsureDerivedRosters(ctx, request); err != nil {
		t.Fatal(err)
	}
	if fingerprint, err := attFinishDerivationSnapshot(ctx, request, before); err != nil || fingerprint == "" {
		t.Fatalf("ordinary generated rosters invalidated their own input: %q %v", fingerprint, err)
	}
	if err = attPutRosters(ctx, []AttRoster{{EmployeeID: request.EmployeeIDs[0], WorkDate: request.StartDate, RuleID: rule.ID, Category: eng.Restday, Source: "manual"}}, "人工修改前一日可改变下一日自动选班"); err != nil {
		t.Fatal(err)
	}
	if _, err := attFinishDerivationSnapshot(ctx, request, before); err == nil {
		t.Fatal("manual roster changes were excluded with generated rosters")
	}
}

func TestMissingAttendanceConfigurationRemainsPrimaryWithCompLeaveIssue(t *testing.T) {
	ctx := setup(t)
	employee := attDirtyEmployee(t, ctx, "缺少制度且调休不足")
	if _, err := UpsertLeave(ctx, Leave{EmployeeID: employee, Source: "wecom", ExternalRef: "insufficient-comp", ApprovalStatus: "approved", LeaveType: "调休", StartTime: "2026-10-12 09:00:00", EndTime: "2026-10-12 10:00:00", Duration: 1}); err != nil {
		t.Fatal(err)
	}
	batch, err := attQueueBatch(ctx, AttBatchRequest{StartDate: "2026-10-12", EndDate: "2026-10-12", EmployeeIDs: []string{employee}})
	if err != nil {
		t.Fatal(err)
	}
	attProcessQueue(ctx)
	row, err := core.Model("biz_att_result").Ctx(ctx).Where("batch_id", batch).One()
	if err != nil {
		t.Fatal(err)
	}
	var result eng.Result
	if err = json.Unmarshal([]byte(row["result_json"].String()), &result); err != nil {
		t.Fatal(err)
	}
	if row["status"].String() != "configuration_pending" || result.State != "configuration_pending" || result.FullAttendanceEligible || !strings.Contains(fmt.Sprint(result.Issues), "余额不足") || !strings.Contains(fmt.Sprint(result.Issues), "待配置") {
		t.Fatalf("ledger issue masked missing configuration: %+v", result)
	}
}
