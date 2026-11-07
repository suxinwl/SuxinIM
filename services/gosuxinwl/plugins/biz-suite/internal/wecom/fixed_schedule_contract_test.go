package wecom

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func fixedScheduleTime(t *testing.T, value string) time.Time {
	t.Helper()
	at, err := time.ParseInLocation("2006-01-02 15:04:05", value, core.Location)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func TestFixedScheduleContractOnlyFourShanghaiMinuteWindows(t *testing.T) {
	day := fixedScheduleTime(t, "2026-10-29 00:00:00")
	want := map[string]bool{"08:30": true, "10:00": true, "14:30": true, "23:59": true}
	seen := map[string]int{}
	for second := 0; second < 86400; second++ {
		at := day.Add(time.Duration(second) * time.Second)
		slot, ok := automaticSlot(at)
		if ok != want[at.Format("15:04")] {
			t.Fatalf("unexpected scheduling combination at %s: %v", at, ok)
		}
		if ok {
			if slot.Location().String() != "Asia/Shanghai" || !slot.Equal(at.Truncate(time.Minute)) {
				t.Fatalf("slot is not the canonical Shanghai minute: %v", slot)
			}
			seen[slot.Format("15:04")]++
		}
	}
	if !reflect.DeepEqual(seen, map[string]int{"08:30": 60, "10:00": 60, "14:30": 60, "23:59": 60}) {
		t.Fatal("missing or Cartesian-product slots", seen)
	}
	for _, clock := range []string{"08:30:19", "10:00:42", "14:30:59", "23:59:00"} {
		at := fixedScheduleTime(t, "2026-10-29 "+clock)
		for _, location := range []*time.Location{time.UTC, time.FixedZone("other-local", -7*3600), time.FixedZone("Tokyo", 9*3600)} {
			slot, ok := automaticSlot(at.In(location))
			if !ok || !slot.Equal(at.Truncate(time.Minute)) {
				t.Fatal("process/input timezone changed the Shanghai schedule", at.In(location), slot)
			}
		}
	}
	if _, ok := automaticSlot(time.Date(2026, 10, 29, 23, 59, 0, 0, time.UTC)); ok {
		t.Fatal("23:59 UTC was mistaken for 23:59 Shanghai")
	}
}

func TestFixedScheduleContractNextRunRollsOverShanghaiDateWithoutCatchup(t *testing.T) {
	for _, tc := range []struct{ at, next string }{
		{"2026-10-29 08:29:59", "2026-10-29 08:30:00"},
		{"2026-10-29 08:31:00", "2026-10-29 10:00:00"},
		{"2026-10-29 10:01:00", "2026-10-29 14:30:00"},
		{"2026-10-29 14:31:00", "2026-10-29 23:59:00"},
		{"2026-10-29 23:59:01", "2026-10-30 08:30:00"},
		{"2026-12-31 23:59:59", "2027-01-01 08:30:00"},
	} {
		at := fixedScheduleTime(t, tc.at)
		status := automaticSchedule(at.UTC())
		next, err := time.ParseInLocation("2006-01-02 15:04:05", gconv.String(status["next_run_at"]), core.Location)
		if err != nil || !next.Equal(fixedScheduleTime(t, tc.next)) || gconv.String(status["timezone"]) != "Asia/Shanghai" || gconv.Bool(status["startup_sync"]) || gconv.Bool(status["catch_up"]) {
			t.Fatalf("incorrect schedule status for %v: %v %v", at, status, err)
		}
		if !reflect.DeepEqual(gconv.Strings(status["times"]), []string{"08:30", "10:00", "14:30", "23:59"}) {
			t.Fatal("status does not expose exact allowed times", status)
		}
	}
}

type fixedScheduleBatchCall struct {
	kind       string
	start, end time.Time
}

type fixedScheduleBatchFake struct {
	calls         []fixedScheduleBatchCall
	fail          map[string]error
	notReady      map[string]bool
	snapshotScope string
}

func (f *fixedScheduleBatchFake) ready(kind string) bool { return !f.notReady[kind] }
func (f *fixedScheduleBatchFake) Run(_ context.Context, kind string, start, end time.Time) (string, int, error) {
	f.calls = append(f.calls, fixedScheduleBatchCall{kind, start, end})
	return kind, 0, f.fail[kind]
}
func (f *fixedScheduleBatchFake) CaptureCheckinSnapshot(_ context.Context, in CheckinSnapshotRequest) (CheckinSnapshot, error) {
	f.calls = append(f.calls, fixedScheduleBatchCall{kind: "snapshot"})
	f.snapshotScope = in.Scope
	return CheckinSnapshot{}, f.fail["snapshot"]
}

func TestFixedScheduleContractBatchesStayInsideOneSlotAndRespectTemplates(t *testing.T) {
	ctx := setupDB(t)
	SetAttendanceChangeHook(nil)
	for _, kind := range []string{"leave", "out", "trip", "overtime"} {
		enabled := 1
		if kind == "out" {
			enabled = 0
		}
		if err := SaveTemplate(ctx, Template{TemplateID: "schedule-" + kind, ApprovalType: kind, Enabled: enabled, Mapping: Mapping{AttendanceControlID: "time"}}); err != nil {
			t.Fatal(err)
		}
	}
	wantKinds := []string{"contact", "checkin", "leave", "trip", "overtime", "unmapped"}
	for _, clock := range []string{"08:30:00", "10:00:00", "14:30:00", "23:59:00"} {
		slot := fixedScheduleTime(t, "2026-10-29 "+clock)
		fake := &fixedScheduleBatchFake{}
		if err := runAutomaticBatch(ctx, fake, slot); err != nil {
			t.Fatal(err)
		}
		kinds := []string{}
		for _, call := range fake.calls {
			kinds = append(kinds, call.kind)
			if call.kind == "snapshot" {
				continue
			}
			if !call.end.Equal(slot) {
				t.Fatal("children drifted to a new wall-clock range", clock, call)
			}
			if call.kind == "checkin" && clock == "23:59:00" {
				if !call.start.Equal(slot.Add(-30 * 24 * time.Hour)) {
					t.Fatal("23:59 calibration lost 30-day window", call)
				}
			} else if !call.start.IsZero() {
				t.Fatal("incremental job was silently replaced by full calibration", clock, call)
			}
		}
		if !reflect.DeepEqual(kinds, wantKinds) || fake.snapshotScope != "" {
			t.Fatal("disabled template, separate snapshot job, or missing batch step", kinds, fake.snapshotScope)
		}
	}
	ordinary := errors.New("ordinary test upstream failure")
	fake := &fixedScheduleBatchFake{fail: map[string]error{"contact": ordinary}}
	if err := runAutomaticBatch(ctx, fake, fixedScheduleTime(t, "2026-10-29 10:00:00")); !errors.Is(err, ordinary) || len(fake.calls) != len(wantKinds) {
		t.Fatal("ordinary failure lost remaining same-batch work or aggregate error", err, fake.calls)
	}
	fake = &fixedScheduleBatchFake{fail: map[string]error{"checkin": ErrSyncRunning}}
	if err := runAutomaticBatch(ctx, fake, fixedScheduleTime(t, "2026-10-29 14:30:00")); !errors.Is(err, ErrSyncRunning) || len(fake.calls) != 2 {
		t.Fatal("busy manual owner caused delayed/supplemental jobs", err, fake.calls)
	}
	fake = &fixedScheduleBatchFake{notReady: map[string]bool{"contact": true, "checkin": true, "approval": true}}
	if err := runAutomaticBatch(ctx, fake, fixedScheduleTime(t, "2026-10-29 23:59:00")); err != nil || len(fake.calls) != 0 {
		t.Fatal("unconfigured credentials still made upstream calls", err, fake.calls)
	}
}

func TestFixedScheduleContractPersistedClaimsPreventRestartAndFailureReruns(t *testing.T) {
	ctx := setupDB(t)
	slot := fixedScheduleTime(t, "2026-10-29 08:30:00")
	calls := 0
	failure := errors.New("test batch failure")
	run := func(job context.Context, got time.Time) error {
		calls++
		if !got.Equal(slot) {
			t.Fatal("execution received tick time instead of canonical slot", got)
		}
		deadline, ok := job.Deadline()
		if !ok || time.Until(deadline) > 45*time.Minute || time.Until(deadline) <= 0 {
			t.Fatal("automatic batch missing bounded deadline", deadline)
		}
		return failure
	}
	executed, err := runScheduledSlot(ctx, slot.Add(7*time.Second), run)
	if !executed || !errors.Is(err, failure) || calls != 1 {
		t.Fatal(executed, err, calls)
	}
	// No scheduler/client instance state is reused: a new callback sees the
	// database claim left by the first invocation, including a failed run.
	for _, delay := range []time.Duration{20 * time.Second, 59 * time.Second, 0} {
		executed, err = runScheduledSlot(ctx, slot.Add(delay), func(context.Context, time.Time) error {
			t.Fatal("persisted slot was retried after a restart/failure")
			return nil
		})
		if executed || err != nil {
			t.Fatal(executed, err)
		}
	}
	n, err := core.Model("wecom_sync_cursor").Ctx(ctx).Where("sync_type", "auto:20261029:0830").Count()
	if err != nil || n != 1 {
		t.Fatal("claim was not durably stored", n, err)
	}
	for _, at := range []string{"2026-10-29 08:31:00", "2026-10-29 09:45:00", "2026-10-29 23:58:59", "2026-10-30 00:00:01"} {
		executed, err = runScheduledSlot(ctx, fixedScheduleTime(t, at), func(context.Context, time.Time) error {
			t.Fatal("missed or old fifteen-minute slot was caught up", at)
			return nil
		})
		if executed || err != nil {
			t.Fatal(at, executed, err)
		}
	}
	for _, next := range []string{"2026-10-29 10:00:00", "2026-10-29 23:59:00", "2026-10-30 08:30:00"} {
		executed, err = runScheduledSlot(ctx, fixedScheduleTime(t, next), func(context.Context, time.Time) error { calls++; return nil })
		if !executed || err != nil {
			t.Fatal("distinct current slot suppressed", next, executed, err)
		}
	}
	if calls != 4 {
		t.Fatal(calls)
	}
}

func TestFixedScheduleContractDoesNotRestrictManualRun(t *testing.T) {
	ctx := setupDB(t)
	seedRawUser(t, ctx, "manual-alice")
	var requests atomic.Int32
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/checkin/getcheckindata" {
			t.Error("unexpected endpoint", r.URL.Path)
		}
		requests.Add(1)
		respond(w, g.Map{"errcode": 0, "checkindata": []any{}})
	})
	day := time.Now().In(core.Location).AddDate(0, 0, -2)
	end := time.Date(day.Year(), day.Month(), day.Day(), 13, 12, 0, 0, core.Location)
	if _, ok := automaticSlot(end); ok {
		t.Fatal("test selected an automatic slot")
	}
	for i := 0; i < 2; i++ {
		id, count, err := c.Run(ctx, "checkin", end.Add(-time.Hour), end)
		if err != nil || id == "" || count != 0 {
			t.Fatal("manual run was limited by automatic scheduling", id, count, err)
		}
	}
	if requests.Load() != 2 || countRows(t, ctx, "wecom_sync_run") != 2 {
		t.Fatal("manual run was de-duplicated by automatic slot policy", requests.Load())
	}
}
