package wecom

import (
	"errors"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"net/http"
	"testing"
	"time"
)

func TestCheckinRulesDisabledWithoutNetworkOrAuditMutation(t *testing.T) {
	ctx := setupDB(t)
	evidence := `{"source":"wecom","response":{"group":[{"groupid":1,"future_field":"preserved"}]}}`
	_, err := core.Model("wecom_checkin_rule_snapshot").Ctx(ctx).Data(g.Map{"id": "historic", "snapshot_type": "corp", "scope_key": "historic", "rule_date": "", "raw_json": evidence, "fetched_at": now()}).Insert()
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		t.Error("disabled rule operation made network request", r.URL.Path)
	})
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, core.Location)
	if _, err = c.GetCorpCheckinOption(ctx); !errors.Is(err, ErrLocalAttendanceRules) {
		t.Fatal(err)
	}
	if _, err = c.GetEmployeeCheckinOption(ctx, day, []string{"alice"}); !errors.Is(err, ErrLocalAttendanceRules) {
		t.Fatal(err)
	}
	if _, err = c.GetCheckinScheduleList(ctx, day, day.AddDate(0, 1, 0), []string{"alice"}); !errors.Is(err, ErrLocalAttendanceRules) {
		t.Fatal(err)
	}
	for _, scope := range []string{"corp", "day", "schedule"} {
		if _, err = c.CaptureCheckinSnapshot(ctx, CheckinSnapshotRequest{Scope: scope}); !errors.Is(err, ErrLocalAttendanceRules) {
			t.Fatal(err)
		}
	}
	row, err := core.Model("wecom_checkin_rule_snapshot").Ctx(ctx).Where("id", "historic").One()
	count, countErr := core.Model("wecom_checkin_rule_snapshot").Ctx(ctx).Count()
	if calls != 0 || err != nil || countErr != nil || count != 1 || row["raw_json"].String() != evidence {
		t.Fatal("legacy evidence changed or upstream contacted", calls, count, err, countErr)
	}
}
