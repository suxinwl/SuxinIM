package wecom

import (
	"context"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/os/gcron"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

var ErrSyncRunning = errors.New("企业微信同步或连接诊断正在运行，请等待完成后重试")

func (c *Client) claim() bool {
	c.runMu.Lock()
	defer c.runMu.Unlock()
	if c.running {
		return false
	}
	c.running = true
	return true
}
func (c *Client) release() { c.runMu.Lock(); c.running = false; c.runMu.Unlock() }
func (c *Client) Sync(ctx context.Context, kind string) (int, error) {
	_, count, err := c.Run(ctx, kind, time.Time{}, time.Time{})
	return count, err
}

// Run and Start are the logged/cursor-tracked entrypoints. Direct SyncContact,
// SyncCheckin and SyncApprovals methods are used by import tools and tests.
func (c *Client) Run(ctx context.Context, kind string, start, end time.Time) (string, int, error) {
	if !c.claim() {
		return "", 0, ErrSyncRunning
	}
	defer c.release()
	runID, start, end, err := c.begin(ctx, kind, start, end)
	if err != nil {
		return "", 0, err
	}
	count, err := c.execute(ctx, runID, kind, start, end)
	return runID, count, err
}
func (c *Client) Start(ctx context.Context, kind string, start, end time.Time) (string, error) {
	if !c.claim() {
		return "", ErrSyncRunning
	}
	id, start, end, err := c.begin(ctx, kind, start, end)
	if err != nil {
		c.release()
		return "", err
	}
	go func() {
		defer c.release()
		jobctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()
		_, _ = c.execute(jobctx, id, kind, start, end)
	}()
	return id, nil
}
func (c *Client) begin(ctx context.Context, kind string, start, end time.Time) (string, time.Time, time.Time, error) {
	if kind != "contact" && kind != "checkin" && kind != "approval" && !validApprovalKind(kind) && kind != "unmapped" {
		return "", start, end, errors.New("unknown sync type")
	}
	if c.CorpID == "" || c.secret(kind) == "" {
		return "", start, end, errors.New("enterprise WeChat credentials are not configured for this sync type")
	}
	if end.IsZero() {
		end = time.Now()
	}
	if end.After(time.Now().Add(5 * time.Minute)) {
		return "", start, end, errors.New("end_time cannot be in the future")
	}
	if start.IsZero() {
		lookback := 24 * time.Hour
		if kind == "approval" || validApprovalKind(kind) || kind == "unmapped" {
			lookback = 31 * 24 * time.Hour
		}
		start = end.Add(-lookback)
		if kind != "contact" {
			row, err := core.Model("wecom_sync_cursor").Ctx(ctx).Where("sync_type", kind).One()
			if err != nil {
				return "", start, end, dbError("sync cursor")
			}
			if len(row) > 0 {
				stamp, e := strconv.ParseInt(row["cursor_value"].String(), 10, 64)
				if e != nil {
					return "", start, end, errors.New("invalid stored sync cursor")
				}
				candidate := time.Unix(stamp, 0).Add(-24 * time.Hour)
				if candidate.Before(end) {
					start = candidate
				}
			}
		}
	}
	var floorErr error
	start, floorErr = applySyncFloor(kind, start, end)
	if floorErr != nil {
		return "", start, end, floorErr
	}
	if _, err := windows(start, end, 30*24*time.Hour); err != nil {
		return "", start, end, err
	}
	id := guid.S()
	if _, err := core.Model("wecom_sync_run").Ctx(ctx).Data(g.Map{"id": id, "sync_type": kind, "status": "running", "started_at": now(), "error_message": ""}).Insert(); err != nil {
		return "", start, end, dbError("sync log")
	}
	return id, start, end, nil
}
func (c *Client) execute(ctx context.Context, id, kind string, start, end time.Time) (count int, result error) {
	defer func() {
		if recover() != nil {
			result = errors.New("wecom synchronization stopped unexpectedly")
		}
		// Finish the audit row even when a request is canceled or the job times out.
		finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		status, msg, failed := "success", "", 0
		if result != nil {
			status = "failed"
			msg = result.Error()
			failed = 1
			var pe *PartialError
			if errors.As(result, &pe) {
				status = "partial"
				failed = pe.Failed
			}
		}
		_, err := core.Model("wecom_sync_run").Ctx(finish).Where("id", id).Update(g.Map{"status": status, "finished_at": now(), "success_count": count, "failure_count": failed, "error_message": msg})
		if err != nil && result == nil {
			result = dbError("sync completion log")
		}
	}()
	switch kind {
	case "contact":
		count, result = c.SyncContact(ctx)
		// Persist account/department ownership only after the directory snapshot
		// completes. Partial employee errors still leave useful, verifiable rows.
		var pe *PartialError
		if result == nil || errors.As(result, &pe) {
			report, syncErr := c.syncOrganization(ctx)
			if syncErr != nil {
				result = syncErr
			} else if report.ConflictCount > 0 && result == nil {
				result = partial(report.ConflictCount)
			}
		}
	case "checkin":
		count, result = c.SyncCheckin(ctx, start, end)
	case "approval":
		count, result = c.SyncApprovals(ctx, start, end)
	case "leave", "out", "trip", "overtime", "unmapped":
		count, result = c.SyncApprovalType(ctx, start, end, kind)
	}
	if result == nil {
		// Failed/partial jobs retain their previous high-water mark. A successful
		// historical backfill must not move an existing cursor backwards.
		row, err := core.Model("wecom_sync_cursor").Ctx(ctx).Where("sync_type", kind).One()
		if err != nil {
			return count, dbError("cursor lookup")
		}
		old := int64(0)
		if len(row) > 0 {
			old, _ = strconv.ParseInt(row["cursor_value"].String(), 10, 64)
		}
		if end.Unix() > old {
			result = upsert(ctx, "wecom_sync_cursor", "sync_type", kind, g.Map{"sync_type": kind, "cursor_value": strconv.FormatInt(end.Unix(), 10), "updated_at": now()})
		}
	}
	return count, result
}

const automaticCronPattern = "0 * * * * *"

var automaticSyncTimes = [...]string{"08:30", "10:00", "14:30", "23:59"}

// automaticSlot matches only this minute in Shanghai. It never looks backwards
// for a missed run, even when the process starts inside a scheduled minute.
func automaticSlot(at time.Time) (time.Time, bool) {
	local := at.In(core.Location)
	for _, clock := range automaticSyncTimes {
		if local.Format("15:04") == clock {
			return time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), 0, 0, core.Location), true
		}
	}
	return time.Time{}, false
}

func automaticSchedule(at time.Time) g.Map {
	local := at.In(core.Location)
	var next time.Time
	for offset := 0; offset <= 1 && next.IsZero(); offset++ {
		day := local.AddDate(0, 0, offset)
		for _, clock := range automaticSyncTimes {
			hour, _ := strconv.Atoi(clock[:2])
			minute, _ := strconv.Atoi(clock[3:])
			candidate := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, core.Location)
			if candidate.After(local) {
				next = candidate
				break
			}
		}
	}
	return g.Map{"timezone": "Asia/Shanghai", "times": append([]string(nil), automaticSyncTimes[:]...), "next_run_at": next.Format("2006-01-02 15:04:05"), "startup_sync": false, "catch_up": false}
}

// The existing cursor primary key is the durable at-most-once claim, rather
// than a successful-run watermark. Failures and restarts must not replay it.
func claimAutomaticSlot(ctx context.Context, slot time.Time) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	matched, ok := automaticSlot(slot)
	if !ok || !slot.Equal(matched) {
		return false, errors.New("自动同步时点无效")
	}
	result, err := core.Model("wecom_sync_cursor").Ctx(ctx).Data(g.Map{
		"sync_type": "auto:" + matched.Format("20060102:1504"), "cursor_value": matched.Format("2006-01-02 15:04:05"), "updated_at": now(),
	}).InsertIgnore()
	if err != nil {
		return false, dbError("automatic schedule claim")
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, dbError("automatic schedule claim result")
	}
	return n == 1, nil
}

// runScheduledSlot is synchronous: once a slot starts, its children may finish
// later, but there is no delayed retry goroutine or catch-up queue.
func runScheduledSlot(ctx context.Context, at time.Time, run func(context.Context, time.Time) error) (bool, error) {
	slot, ok := automaticSlot(at)
	if !ok {
		return false, nil
	}
	claimed, err := claimAutomaticSlot(ctx, slot)
	if err != nil || !claimed {
		return false, err
	}
	job, cancel := context.WithTimeout(ctx, 45*time.Minute)
	defer cancel()
	return true, run(job, slot)
}

// Narrow interface keeps the batch's fixed range and ordering testable without
// an upstream service. Attendance rules are entirely local; no rule snapshots
// are requested by an automatic batch.
type automaticBatch interface {
	ready(string) bool
	Run(context.Context, string, time.Time, time.Time) (string, int, error)
}

func runAutomaticBatch(ctx context.Context, c automaticBatch, slot time.Time) error {
	var failures []error
	if err := RetryAttendanceChanges(ctx); err != nil {
		failures = append(failures, err)
	}
	run := func(kind string, start time.Time) bool {
		if err := ctx.Err(); err != nil {
			failures = append(failures, err)
			return false
		}
		_, _, err := c.Run(ctx, kind, start, slot)
		if err != nil {
			failures = append(failures, err)
		}
		// A manual job or diagnostic already owns the client. Skip this slot's
		// remaining work rather than wait and create another automatic start.
		return !errors.Is(err, ErrSyncRunning)
	}
	if c.ready("contact") && !run("contact", time.Time{}) {
		return errors.Join(failures...)
	}
	if c.ready("checkin") {
		start := time.Time{}
		if slot.In(core.Location).Format("15:04") == "23:59" {
			start = slot.Add(-30 * 24 * time.Hour)
		}
		if !run("checkin", start) {
			return errors.Join(failures...)
		}
	}
	if c.ready("approval") {
		for _, kind := range approvalKinds {
			enabled, err := core.Model("wecom_approval_template").Ctx(ctx).Where("approval_type", kind).Where("enabled", 1).Exist()
			if err != nil {
				failures = append(failures, dbError("automatic approval templates"))
				continue
			}
			if enabled && !run(kind, time.Time{}) {
				return errors.Join(failures...)
			}
		}
		if !run("unmapped", time.Time{}) {
			return errors.Join(failures...)
		}
	}
	return errors.Join(failures...)
}

func RegisterScheduler(ctx context.Context, c *Client) error {
	if os.Getenv("GOSUXIN_REHEARSAL") == "true" {
		gcron.Remove("biz-suite-wecom-rule-snapshot")
		gcron.Remove("biz-suite-wecom-sync")
		return nil
	}
	// Startup recovers local notifications/logs only. It must never perform an
	// upstream read or recover a missed automatic slot.
	recovery, cancelRecovery := context.WithTimeout(ctx, 15*time.Second)
	_ = RetryAttendanceChanges(recovery)
	cancelRecovery()
	if _, err := core.Model("wecom_sync_run").Ctx(ctx).Where("status", "running").Update(g.Map{"status": "failed", "finished_at": now(), "failure_count": 1, "error_message": "worker restarted before synchronization completed; retry the batch"}); err != nil {
		return dbError("recover sync log")
	}
	gcron.Remove("biz-suite-wecom-rule-snapshot")
	gcron.Remove("biz-suite-wecom-sync")
	_, err := gcron.AddSingleton(ctx, automaticCronPattern, func(_ context.Context) {
		_, _ = runScheduledSlot(ctx, time.Now(), func(job context.Context, slot time.Time) error {
			return runAutomaticBatch(job, c, slot)
		})
	}, "biz-suite-wecom-sync")
	return err
}

func (c *Client) ready(kind string) bool {
	c.runMu.Lock()
	defer c.runMu.Unlock()
	return c.CorpID != "" && c.secret(kind) != ""
}
