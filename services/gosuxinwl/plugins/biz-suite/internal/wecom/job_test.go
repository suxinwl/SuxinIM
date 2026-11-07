package wecom

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func TestOutApprovalAndDisabledTemplateInvalidatePreviousAttendance(t *testing.T) {
	ctx := setupDB(t)
	seedEmployee(t, ctx, "alice")
	template := Template{TemplateID: "out", ApprovalType: "out", Enabled: 1, Mapping: Mapping{AttendanceControlID: "out-time", DestinationControlID: "place"}}
	if err := SaveTemplate(ctx, template); err != nil {
		t.Fatal(err)
	}
	fixture := g.Map{"sp_no": "OUT-1", "template_id": "out", "sp_status": 2, "applyer": g.Map{"userid": "alice"}, "apply_data": g.Map{"contents": []any{
		g.Map{"id": "out-time", "control": "Attendance", "value": g.Map{"attendance": g.Map{"type": 4, "date_range": g.Map{"new_begin": 1700000000, "new_end": 1700003600, "new_duration": 3600}}}},
		g.Map{"id": "place", "control": "Text", "value": g.Map{"text": "customer office"}},
	}}}
	var d approvalDetail
	_ = json.Unmarshal([]byte(mustJSON(fixture)), &d)
	if err := saveApproval(ctx, d, mustJSON(fixture)); err != nil {
		t.Fatal(err)
	}
	row, _ := core.Model("biz_hrm_attendance_out").Ctx(ctx).One()
	if row["duration"].Float64() != 1 || row["destination"].String() != "customer office" {
		t.Fatal(row)
	}
	template.Enabled = 0
	if err := SaveTemplate(ctx, template); err != nil {
		t.Fatal(err)
	}
	var pe *PartialError
	if err := saveApproval(ctx, d, mustJSON(fixture)); !errors.As(err, &pe) {
		t.Fatal(err)
	}
	row, _ = core.Model("biz_hrm_attendance_out").Ctx(ctx).One()
	if row["approval_status"].String() != "pending" {
		t.Fatal("disabled template left active attendance", row)
	}
}

func TestUnsupportedBatchApplicantsAreRetainedAsExceptions(t *testing.T) {
	ctx := setupDB(t)
	seedEmployee(t, ctx, "alice")
	installTemplate(t, ctx)
	fixture := approvalFixture("BATCH-1", "leave-tpl", 2)
	delete(fixture, "applyer")
	fixture["batch_applyer"] = []any{g.Map{"userid": "alice"}, g.Map{"userid": "bob"}}
	var d approvalDetail
	_ = json.Unmarshal([]byte(mustJSON(fixture)), &d)
	var pe *PartialError
	if err := saveApproval(ctx, d, mustJSON(fixture)); !errors.As(err, &pe) {
		t.Fatal(err)
	}
	if countRows(t, ctx, "biz_hrm_attendance_leave") != 0 {
		t.Fatal("batch assigned to a guessed applicant")
	}
	row, _ := core.Model("wecom_approval_raw").Ctx(ctx).One()
	if !strings.Contains(row["raw_json"].String(), "bob") || row["processing_error"].String() == "" {
		t.Fatal(row)
	}
}

func TestSuccessfulRunPersistsLogAndMonotonicCursor(t *testing.T) {
	ctx := setupDB(t)
	seedRawUser(t, ctx, "alice")
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) { respond(w, g.Map{"errcode": 0, "checkindata": []any{}}) })
	end := time.Now().Add(-time.Hour).Truncate(time.Second)
	id, n, err := c.Run(ctx, "checkin", end.Add(-time.Hour), end)
	if err != nil || n != 0 {
		t.Fatal(id, n, err)
	}
	row, _ := core.Model("wecom_sync_cursor").Ctx(ctx).Where("sync_type", "checkin").One()
	if row["cursor_value"].Int64() != end.Unix() {
		t.Fatal(row)
	}
	run, _ := core.Model("wecom_sync_run").Ctx(ctx).Where("id", id).One()
	if run["status"].String() != "success" {
		t.Fatal(run)
	}
	if _, _, err = c.Run(ctx, "checkin", end.Add(-72*time.Hour), end.Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	row, _ = core.Model("wecom_sync_cursor").Ctx(ctx).Where("sync_type", "checkin").One()
	if row["cursor_value"].Int64() != end.Unix() {
		t.Fatal("backfill moved cursor backwards", row)
	}
}

func TestBackgroundJobSingleFlightAndCancellationAudit(t *testing.T) {
	ctx := setupDB(t)
	seedRawUser(t, ctx, "alice")
	entered := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	job, cancel := context.WithCancel(ctx)
	defer cancel()
	id, err := c.Start(job, "checkin", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("job did not start")
	}
	if _, err = c.Start(job, "checkin", time.Time{}, time.Time{}); !errors.Is(err, ErrSyncRunning) {
		t.Fatal(err)
	}
	cancel()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		row, e := core.Model("wecom_sync_run").Ctx(ctx).Where("id", id).One()
		if e != nil {
			t.Fatal(e)
		}
		if row["status"].String() == "failed" {
			if row["failure_count"].Int() != 1 {
				t.Fatal(row)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("canceled background job did not finish its audit log")
}
