package wecom

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func TestHistoryProbeSingleAllowedPersonReadOnlyAndSafeReport(t *testing.T) {
	ctx := setupDB(t)
	filtered := seedEmployee(t, ctx, "filtered-person")
	seedRawUser(t, ctx, "filtered-person")
	if _, e := core.Model("biz_hrm_employee").Ctx(ctx).Where("id", filtered).Update(g.Map{"status": "filtered"}); e != nil {
		t.Fatal(e)
	}
	seedEmployee(t, ctx, "current-person")
	seedRawUser(t, ctx, "current-person")
	requests := 0
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != "POST" || r.URL.Path != "/checkin/getcheckindata" {
			t.Fatal("unexpected endpoint", r.Method, r.URL.Path)
		}
		var body struct {
			IDs        []string `json:"useridlist"`
			Start, End int64
			StartTime  int64 `json:"starttime"`
			EndTime    int64 `json:"endtime"`
		}
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Fatal(e)
		}
		if len(body.IDs) != 1 || body.IDs[0] != "current-person" || body.EndTime-body.StartTime != 86399 {
			t.Fatalf("not a one-person single-day probe %+v", body)
		}
		respond(w, g.Map{"errcode": 0, "checkindata": []any{g.Map{"userid": "current-person", "checkin_time": body.StartTime + 9*3600, "checkin_type": "上班打卡", "private_field": "must-not-report"}}})
	})
	report, e := c.ProbeHistoricalCheckins(ctx, []string{"2020-08-03", "2020-08-10", "2020-09-01"})
	if e != nil {
		t.Fatal(e)
	}
	if requests != 3 || len(report) != 3 {
		t.Fatal(report, requests)
	}
	for _, r := range report {
		if !r.OK || r.Count != 1 {
			t.Fatal(r)
		}
	}
	raw, _ := json.Marshal(report)
	for _, forbidden := range []string{"current-person", "filtered-person", "must-not-report", "token-secret", "secret"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatal("report leaked source", string(raw))
		}
	}
	for _, table := range []string{"wecom_checkin_raw", "biz_hrm_attendance_clock", "wecom_sync_run", "wecom_sync_cursor", "wecom_checkin_rule_snapshot"} {
		if countRows(t, ctx, table) != 0 {
			t.Fatal("probe persisted data", table)
		}
	}
}

func TestHistoryProbeRejectsOutOfScopeResponsesAndRedactsErrors(t *testing.T) {
	ctx := setupDB(t)
	seedEmployee(t, ctx, "sample")
	seedRawUser(t, ctx, "sample")
	mode := "private"
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if mode == "private" {
			respond(w, g.Map{"errcode": 48009, "errmsg": "do-not-print-secret-credentials"})
		} else {
			respond(w, g.Map{"errcode": 0, "checkindata": []any{g.Map{"userid": "outsider", "checkin_time": 1}}})
		}
	})
	results, e := c.ProbeHistoricalCheckins(ctx, []string{"2020-08-03"})
	if e != nil || len(results) != 1 || results[0].ErrorCode != 48009 || results[0].OK {
		t.Fatal(results, e)
	}
	raw, _ := json.Marshal(results)
	if strings.Contains(string(raw), "credentials") || strings.Contains(string(raw), "secret") {
		t.Fatal("API error leaked")
	}
	mode = "outsider"
	results, e = c.ProbeHistoricalCheckins(ctx, []string{"2020-08-03"})
	if e != nil || results[0].OK || results[0].Count != 0 {
		t.Fatal(results, e)
	}
	if _, e = c.ProbeHistoricalCheckins(ctx, []string{time.Now().AddDate(0, 0, 2).Format("2006-01-02")}); e == nil {
		t.Fatal("future date accepted")
	}
	if !c.claim() {
		t.Fatal("claim")
	}
	defer c.release()
	if _, e = c.ProbeHistoricalCheckins(ctx, []string{"2020-08-03"}); e != ErrSyncRunning {
		t.Fatal(e)
	}
}
