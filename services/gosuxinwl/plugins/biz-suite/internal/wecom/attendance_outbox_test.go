package wecom

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func TestCheckinNotificationOnlyOnChangeAndDurableRetry(t *testing.T) {
	ctx := setupDB(t)
	seedEmployee(t, ctx, "alice")
	seedRawUser(t, ctx, "alice")
	item := Checkin{UserID: "alice", Type: "上班打卡", Time: 1700000000, ScheduledTime: 1700000000}
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		respond(w, g.Map{"errcode": 0, "checkindata": []Checkin{item}})
	})
	calls := 0
	failing := true
	SetAttendanceChangeHook(func(context.Context, time.Time, time.Time, []string) error {
		calls++
		if failing {
			return errors.New("offline")
		}
		return nil
	})
	t.Cleanup(func() { SetAttendanceChangeHook(nil) })
	start, end := time.Unix(1699990000, 0), time.Unix(1700010000, 0)
	if _, _, err := c.Run(ctx, "checkin", start, end); err == nil {
		t.Fatal("failed callback claimed success")
	}
	if countRows(t, ctx, "wecom_attendance_outbox") != 1 || countRows(t, ctx, "wecom_sync_cursor") != 0 {
		t.Fatal("pending event/cursor missing")
	}
	failing = false
	if _, _, err := c.Run(ctx, "checkin", start, end); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || countRows(t, ctx, "wecom_attendance_outbox") != 1 {
		t.Fatal("unchanged retry lost or duplicated durable event", calls)
	}
	if _, _, err := c.Run(ctx, "checkin", start, end); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("idempotent checkin notified again", calls)
	}
	item.Time += 600
	item.Exception = "时间异常"
	if _, _, err := c.Run(ctx, "checkin", start, end); err != nil {
		t.Fatal(err)
	}
	if calls != 3 || countRows(t, ctx, "biz_hrm_attendance_clock") != 1 {
		t.Fatal("corrected checkin not notified", calls)
	}
}

func TestUnknownCheckinRepeatedExceptionDoesNotDirtyHistory(t *testing.T) {
	ctx := setupDB(t)
	seedEmployee(t, ctx, "alice")
	seedRawUser(t, ctx, "alice")
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		respond(w, g.Map{"errcode": 0, "checkindata": []Checkin{{UserID: "alice", Time: 1700000000}}})
	})
	calls := 0
	SetAttendanceChangeHook(func(context.Context, time.Time, time.Time, []string) error { calls++; return nil })
	t.Cleanup(func() { SetAttendanceChangeHook(nil) })
	for i := 0; i < 2; i++ {
		_, err := c.SyncCheckin(ctx, time.Unix(1699990000, 0), time.Unix(1700010000, 0))
		var partial *PartialError
		if !errors.As(err, &partial) {
			t.Fatal(err)
		}
	}
	if calls != 1 || countRows(t, ctx, "wecom_attendance_outbox") != 1 {
		t.Fatal("repeated unknown checkin dirtied history", calls)
	}
}

func TestSourceAndOutboxCommitAtomically(t *testing.T) {
	ctx := setupDB(t)
	seedEmployee(t, ctx, "alice")
	installTemplate(t, ctx)
	if _, err := core.DB().Exec(ctx, "DROP TABLE wecom_attendance_outbox"); err != nil {
		t.Fatal(err)
	}
	item := Checkin{UserID: "alice", Type: "上班打卡", Time: 1700000000}
	if err := saveCheckin(ctx, item, mustJSON(item)); err == nil {
		t.Fatal("missing outbox accepted")
	}
	if countRows(t, ctx, "wecom_checkin_raw") != 0 || countRows(t, ctx, "biz_hrm_attendance_clock") != 0 {
		t.Fatal("source committed without notification")
	}
	fixture := approvalFixture("ROLLBACK", "leave-tpl", 2)
	var d approvalDetail
	_ = json.Unmarshal([]byte(mustJSON(fixture)), &d)
	if err := saveApproval(ctx, d, mustJSON(fixture)); err == nil {
		t.Fatal("approval accepted missing outbox")
	}
	if countRows(t, ctx, "wecom_approval_raw") != 0 || countRows(t, ctx, "biz_hrm_attendance_leave") != 0 {
		t.Fatal("approval committed without notification")
	}
}
