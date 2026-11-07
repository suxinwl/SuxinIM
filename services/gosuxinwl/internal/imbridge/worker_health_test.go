package imbridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWorkerHealthPersistsStageAndSafeResult(t *testing.T) {
	ctx, store, _ := fixture(t)
	health := workerHealth{}
	must(t, health.record(ctx, store, "scanning", nil))
	var raw string
	must(t, store.DB.QueryRowContext(ctx, `SELECT v FROM biz_openim_state WHERE k='worker_health'`).Scan(&raw))
	var actual workerHealth
	must(t, json.Unmarshal([]byte(raw), &actual))
	if actual.Stage != "scanning" || actual.LastScanStarted == 0 || actual.LastScanCompleted != 0 {
		t.Fatal("initial scan cannot be distinguished from stopped worker")
	}
	must(t, health.record(ctx, store, "scan_completed", fault("source.employees.scan", errors.New("sql: Scan error converting NULL; password=private-value"))))
	must(t, store.DB.QueryRowContext(ctx, `SELECT v FROM biz_openim_state WHERE k='worker_health'`).Scan(&raw))
	if strings.Contains(raw, "private-value") || !strings.Contains(raw, "source.employees.scan class=null_projection") {
		t.Fatal("worker heartbeat contains unsafe or unhelpful diagnostic")
	}
	must(t, json.Unmarshal([]byte(raw), &actual))
	if actual.LastScanCompleted == 0 {
		t.Fatal("scan completion was not persisted")
	}
}

type slowUserRemote struct{ Remote }

func (r slowUserRemote) User(ctx context.Context, p Projection) error {
	if err := r.Remote.User(ctx, p); err != nil {
		return err
	}
	time.Sleep(20 * time.Millisecond)
	return nil
}

func TestOutboxBudgetRetainsWorkAndSavesLastCompletedTask(t *testing.T) {
	ctx, store, remote := fixture(t)
	must(t, store.ConfirmProvision(ctx))
	must(t, store.Reconcile(ctx))
	must(t, store.drain(ctx, slowUserRemote{remote}, 10*time.Millisecond))
	var done, pending int
	must(t, store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM biz_openim_outbox WHERE status='done'`).Scan(&done))
	must(t, store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM biz_openim_outbox WHERE status='pending'`).Scan(&pending))
	if done != 1 || pending == 0 {
		t.Fatalf("budget must save one completed operation and retain others, done=%d pending=%d", done, pending)
	}
	must(t, store.Drain(ctx, remote))
	must(t, store.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM biz_openim_outbox WHERE status='pending'`).Scan(&pending))
	if pending != 0 {
		t.Fatal("subsequent drain did not resume remaining durable work")
	}
}
