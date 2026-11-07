package wecom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/hrm"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/migration"
	"golang.org/x/time/rate"
)

func setupDB(t *testing.T) context.Context {
	t.Helper()
	ctx := context.Background()
	db, err := gdb.New(gdb.ConfigNode{Type: "sqlite", Link: fmt.Sprintf("sqlite::@file(%s)", filepath.ToSlash(filepath.Join(t.TempDir(), "wecom.db"))), Prefix: "gf_"})
	if err != nil {
		t.Fatal(err)
	}
	if err = core.Init(db); err != nil {
		t.Fatal(err)
	}
	business := core.DB()
	t.Cleanup(func() { _ = business.Close(ctx); _ = db.Close(ctx) })
	if err = migration.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = core.DB().Exec(ctx, "CREATE TABLE gf_admin (id INTEGER PRIMARY KEY,status INTEGER NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	if _, err = core.DB().Exec(ctx, "INSERT INTO gf_admin (id,status) VALUES (1,0)"); err != nil {
		t.Fatal(err)
	}
	return ctx
}
func mockClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/gettoken" {
			_, _ = w.Write([]byte(`{"errcode":0,"access_token":"token-secret","expires_in":7200}`))
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return &Client{HTTP: server.Client(), CorpID: "corp", AgentID: "agent", ContactSecret: "contact-secret", AppSecret: "secret", BaseURL: server.URL, limiter: rate.NewLimiter(rate.Inf, 1)}
}
func respond(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }
func seedEmployee(t *testing.T, ctx context.Context, uid string) string {
	t.Helper()
	id, err := hrm.UpsertEmployee(ctx, hrm.Employee{Name: uid, WeComUserID: uid, Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func seedRawUser(t *testing.T, ctx context.Context, uid string) {
	t.Helper()
	err := upsert(ctx, "wecom_user", "wecom_userid", uid, g.Map{"id": stableID(uid), "wecom_userid": uid, "department_ids": "[]", "raw_json": "{}", "synced_at": now()})
	if err != nil {
		t.Fatal(err)
	}
}
func countRows(t *testing.T, ctx context.Context, table string) int {
	t.Helper()
	n, err := core.Model(table).Ctx(ctx).Count()
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestUserStatus(t *testing.T) {
	for in, want := range map[int]string{1: "active", 2: "inactive", 4: "inactive", 5: "resigned", 0: "inactive"} {
		if userStatus(in) != want {
			t.Fatal(in)
		}
	}
}
func TestAccessTokenScopesCacheAndRefresh(t *testing.T) {
	var tokenCalls, apiCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gettoken" {
			n := tokenCalls.Add(1)
			respond(w, g.Map{"errcode": 0, "access_token": fmt.Sprintf("token-%d", n), "expires_in": 7200})
			return
		}
		if apiCalls.Add(1) == 1 {
			respond(w, g.Map{"errcode": 42001, "errmsg": "echo token secret"})
			return
		}
		respond(w, g.Map{"errcode": 0})
	}))
	defer server.Close()
	c := &Client{HTTP: server.Client(), CorpID: "corp", AgentID: "a", ContactSecret: "contact-secret", AppSecret: "app-secret", BaseURL: server.URL, limiter: rate.NewLimiter(rate.Inf, 1)}
	a, err := c.AccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.AccessToken(context.Background())
	if err != nil || a != b || tokenCalls.Load() != 1 {
		t.Fatal(a, b, err)
	}
	contact, err := c.accessToken(context.Background(), "contact")
	if err != nil || contact == a || tokenCalls.Load() != 2 {
		t.Fatal("contact token must be independently cached", err)
	}
	var out apiError
	if err = c.api(context.Background(), "checkin", http.MethodPost, "/probe", nil, g.Map{}, &out); err != nil {
		t.Fatal(err)
	}
	if tokenCalls.Load() != 3 || apiCalls.Load() != 2 {
		t.Fatal(tokenCalls.Load(), apiCalls.Load())
	}
	if _, err := c.accessToken(context.Background(), "approval"); err != nil || tokenCalls.Load() != 3 {
		t.Fatal("approval must reuse the refreshed application token", err)
	}
}

type brokenTransport struct{}

func (brokenTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("credentials leaked in transport: %s", r.URL.String())
}
func TestErrorsDoNotLeakSecretsAndCancellationStopsRetries(t *testing.T) {
	c := &Client{HTTP: &http.Client{Transport: brokenTransport{}}, CorpID: "corp", ContactSecret: "contact-private", AppSecret: "do-not-print-me", BaseURL: "https://example.invalid", limiter: rate.NewLimiter(rate.Inf, 1)}
	_, err := c.AccessToken(context.Background())
	if err == nil || strings.Contains(err.Error(), "do-not-print-me") || strings.Contains(err.Error(), "corpsecret") {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	var requests atomic.Int32
	c = mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		cancel()
		w.WriteHeader(500)
		_, _ = w.Write([]byte("token-secret do-not-print-me"))
	})
	var out apiError
	err = c.api(ctx, "contact", http.MethodGet, "/retry", nil, nil, &out)
	if !errors.Is(err, context.Canceled) || requests.Load() != 1 {
		t.Fatal(err, requests.Load())
	}
	c = mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		respond(w, g.Map{"errcode": 40003, "errmsg": "token-secret do-not-print-me"})
	})
	err = c.api(context.Background(), "contact", http.MethodGet, "/error", nil, nil, &out)
	if err == nil || strings.Contains(err.Error(), "token-secret") || strings.Contains(err.Error(), "do-not-print-me") {
		t.Fatal(err)
	}
}
func TestHTTPAndAPIRateLimitRetries(t *testing.T) {
	var calls atomic.Int32
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(429)
			return
		}
		if n == 2 {
			respond(w, g.Map{"errcode": 45009})
			return
		}
		respond(w, g.Map{"errcode": 0})
	})
	var out apiError
	if err := c.api(context.Background(), "contact", http.MethodGet, "/rate", nil, nil, &out); err != nil || calls.Load() != 3 {
		t.Fatal(err, calls.Load())
	}
}
func TestContactSyncDeduplicatesDepartmentsAndPreservesMappings(t *testing.T) {
	ctx := setupDB(t)
	id, err := hrm.UpsertEmployee(ctx, hrm.Employee{Name: "Local", UserID: 1, EmployeeNo: "EMP-1", Mobile: "13800000000", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	var depCalls atomic.Int32
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/department/list":
			respond(w, g.Map{"errcode": 0, "department": []Department{{ID: 2, Name: "A"}, {ID: 3, Name: "B"}}})
		case "/user/list":
			depCalls.Add(1)
			if r.URL.Query().Get("fetch_child") != "" || r.URL.Query().Get("department_id") == "1" {
				t.Error("must query visible departments explicitly")
			}
			respond(w, g.Map{"errcode": 0, "userlist": []User{{UserID: "alice", Name: "Alice", Mobile: "13800000000", Department: []int64{2, 3}, Status: 1}}})
		default:
			t.Error(r.URL.Path)
		}
	})
	for i := 0; i < 2; i++ {
		n, e := c.SyncContact(ctx)
		if e != nil || n != 1 {
			t.Fatal(n, e)
		}
	}
	if depCalls.Load() != 4 || countRows(t, ctx, "wecom_user") != 1 || countRows(t, ctx, "biz_hrm_employee") != 1 {
		t.Fatal("duplicates")
	}
	row, err := core.Model("biz_hrm_employee").Ctx(ctx).Where("id", id).One()
	if err != nil || row["user_id"].Int() != 1 || row["employee_no"].String() != "EMP-1" || row["wecom_userid"].String() != "alice" {
		t.Fatal(row, err)
	}
	raw, _ := core.Model("wecom_user").Ctx(ctx).One()
	if raw["employee_id"].String() != id {
		t.Fatal(raw)
	}
}
func TestContactMobileCollisionIsVisibleAndDoesNotCreateEmployee(t *testing.T) {
	ctx := setupDB(t)
	for _, uid := range []string{"one", "two"} {
		_, e := hrm.UpsertEmployee(ctx, hrm.Employee{Name: uid, Mobile: "same", Status: "active"})
		if e != nil {
			t.Fatal(e)
		}
	}
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/department/list" {
			respond(w, g.Map{"errcode": 0, "department": []Department{{ID: 1, Name: "root"}}})
		} else {
			respond(w, g.Map{"errcode": 0, "userlist": []User{{UserID: "ambiguous", Mobile: "same", Status: 1}}})
		}
	})
	n, err := c.SyncContact(ctx)
	var pe *PartialError
	if n != 0 || !errors.As(err, &pe) || countRows(t, ctx, "biz_hrm_employee") != 2 {
		t.Fatal(n, err)
	}
	row, _ := core.Model("wecom_user").Ctx(ctx).One()
	if row["processing_error"].String() == "" {
		t.Fatal("missing collision diagnostic")
	}
}
func TestCheckinSplitsEveryWindowAnd100Users(t *testing.T) {
	ctx := setupDB(t)
	for i := 0; i < 101; i++ {
		seedRawUser(t, ctx, fmt.Sprintf("u%03d", i))
	}
	var calls atomic.Int32
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Users []string `json:"useridlist"`
			Start int64    `json:"starttime"`
			End   int64    `json:"endtime"`
			Type  int      `json:"opencheckindatatype"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("decode")
		}
		if len(body.Users) > 100 || len(body.Users) == 0 || body.End-body.Start > 30*86400 || body.Type != 3 {
			t.Errorf("invalid body: %+v", body)
		}
		calls.Add(1)
		respond(w, g.Map{"errcode": 0, "checkindata": []any{}})
	})
	end := time.Now()
	if _, err := c.SyncCheckin(ctx, end.Add(-61*24*time.Hour), end); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 6 {
		t.Fatal(calls.Load())
	}
}
func TestCheckinNormalizationCorrectionAndIdempotency(t *testing.T) {
	ctx := setupDB(t)
	seedEmployee(t, ctx, "alice")
	seedRawUser(t, ctx, "alice")
	scheduled := time.Date(2026, 10, 27, 9, 0, 0, 0, core.Location).Unix()
	phase := 0
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		item := g.Map{"userid": "alice", "checkin_type": "上班打卡", "checkin_time": int64(0), "sch_checkin_time": scheduled, "exception_type": "未打卡", "lat": 30547645, "lng": 104063236}
		if phase > 0 {
			item["checkin_time"] = scheduled + 60
			item["exception_type"] = "时间异常"
		}
		respond(w, g.Map{"errcode": 0, "checkindata": []any{item}})
	})
	for i := 0; i < 3; i++ {
		phase = i
		n, err := c.SyncCheckin(ctx, time.Unix(scheduled-100, 0), time.Unix(scheduled+3600, 0))
		if n != 1 || err != nil {
			t.Fatal(n, err)
		}
	}
	if countRows(t, ctx, "wecom_checkin_raw") != 1 || countRows(t, ctx, "biz_hrm_attendance_clock") != 1 {
		t.Fatal("correction duplicated a fixed-shift punch")
	}
	row, err := core.Model("biz_hrm_attendance_clock").Ctx(ctx).One()
	if err != nil || row["clock_type"].String() != "in" || row["status"].String() != "late" || row["latitude"].String() != "30.547645" || row["longitude"].String() != "104.063236" {
		t.Fatal(row, err)
	}
}
func TestUnmappedCheckinNeverCreatesBlankEmployeeClock(t *testing.T) {
	ctx := setupDB(t)
	seedRawUser(t, ctx, "unknown")
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		respond(w, g.Map{"errcode": 0, "checkindata": []any{g.Map{"userid": "unknown", "checkin_type": "上班打卡", "checkin_time": 1700000000}}})
	})
	_, err := c.SyncCheckin(ctx, time.Unix(1699990000, 0), time.Unix(1700010000, 0))
	var pe *PartialError
	if !errors.As(err, &pe) || countRows(t, ctx, "biz_hrm_attendance_clock") != 0 {
		t.Fatal(err)
	}
	row, _ := core.Model("wecom_checkin_raw").Ctx(ctx).One()
	if row["processing_error"].String() == "" {
		t.Fatal("missing normalization exception")
	}
}
func approvalFixture(sp, template string, status int) g.Map {
	return g.Map{"sp_no": sp, "template_id": template, "sp_status": status, "apply_time": 1700000000, "applyer": g.Map{"userid": "alice"}, "sp_record": []any{g.Map{"approver": g.Map{"userid": "wrong-person"}}}, "apply_data": g.Map{"contents": []any{
		g.Map{"id": "vacation-control", "control": "Vacation", "value": g.Map{"vacation": g.Map{"selector": g.Map{"options": []any{g.Map{"key": "annual", "value": []any{g.Map{"text": "年假", "lang": "zh_CN"}}}}}, "attendance": g.Map{"type": 1, "date_range": g.Map{"new_begin": 1700000000, "new_end": 1700007200, "new_duration": 7200}}}}},
		g.Map{"id": "reason-control", "control": "Textarea", "value": g.Map{"text": "family"}},
		g.Map{"id": "start-guess", "control": "Text", "value": g.Map{"start_time": "BAD"}},
	}}}
}
func installTemplate(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := SaveTemplate(ctx, Template{TemplateID: "leave-tpl", ApprovalType: "leave", Enabled: 1, Mapping: Mapping{AttendanceControlID: "vacation-control", ReasonControlID: "reason-control"}}); err != nil {
		t.Fatal(err)
	}
}
func TestApprovalNewCursorPaginationMappingAndOldCancellation(t *testing.T) {
	ctx := setupDB(t)
	seedEmployee(t, ctx, "alice")
	installTemplate(t, ctx)
	phase := 0
	pages := 0
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch r.URL.Path {
		case "/oa/getapprovalinfo":
			pages++
			if _, ok := req["cursor"]; ok {
				t.Error("deprecated cursor sent")
			}
			if req["new_cursor"] == "" {
				respond(w, g.Map{"errcode": 0, "sp_no_list": []string{}, "new_next_cursor": "opaque-next"})
			} else {
				ids := []string{"SP-1"}
				if phase > 0 {
					ids = []string{}
				}
				respond(w, g.Map{"errcode": 0, "sp_no_list": ids})
			}
		case "/oa/getapprovaldetail":
			if req["sp_no"] != "SP-1" {
				t.Error(req)
			}
			detail := approvalFixture("SP-1", "leave-tpl", 2)
			if phase > 0 {
				detail = g.Map{"sp_no": "SP-1", "template_id": "leave-tpl", "sp_status": 7}
			}
			respond(w, g.Map{"errcode": 0, "info": detail})
		default:
			t.Error(r.URL.Path)
		}
	})
	start, end := time.Unix(1699990000, 0), time.Unix(1700010000, 0)
	if n, err := c.SyncApprovals(ctx, start, end); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	row, err := core.Model("biz_hrm_attendance_leave").Ctx(ctx).One()
	if err != nil || row["duration"].Float64() != 2 || row["approval_status"].String() != "approved" || row["reason"].String() != "family" {
		t.Fatal(row, err)
	}
	phase = 1
	if _, err = c.SyncApprovals(ctx, end.Add(100*24*time.Hour), end.Add(101*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	row, _ = core.Model("biz_hrm_attendance_leave").Ctx(ctx).One()
	if row["approval_status"].String() != "canceled" || countRows(t, ctx, "biz_hrm_attendance_leave") != 1 || pages != 4 {
		t.Fatal(row, pages)
	}
}
func TestUnknownApprovalTemplateRawRetentionAndCursorFailure(t *testing.T) {
	ctx := setupDB(t)
	seedEmployee(t, ctx, "alice")
	end := time.Now().Add(-time.Hour)
	old := end.Add(-time.Hour).Unix()
	if err := upsert(ctx, "wecom_sync_cursor", "sync_type", "approval", g.Map{"sync_type": "approval", "cursor_value": fmt.Sprint(old), "updated_at": now()}); err != nil {
		t.Fatal(err)
	}
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oa/getapprovalinfo" {
			respond(w, g.Map{"errcode": 0, "sp_no_list": []string{"SP-X"}})
		} else {
			respond(w, g.Map{"errcode": 0, "info": approvalFixture("SP-X", "unmapped", 2)})
		}
	})
	id, n, err := c.Run(ctx, "approval", end.Add(-24*time.Hour), end)
	var pe *PartialError
	if n != 0 || !errors.As(err, &pe) {
		t.Fatal(id, n, err)
	}
	if countRows(t, ctx, "biz_hrm_attendance_leave") != 0 || countRows(t, ctx, "wecom_approval_raw") != 1 {
		t.Fatal("invalid normalization")
	}
	row, _ := core.Model("wecom_sync_cursor").Ctx(ctx).Where("sync_type", "approval").One()
	if row["cursor_value"].Int64() != old {
		t.Fatal("failed job advanced cursor", row)
	}
	run, _ := core.Model("wecom_sync_run").Ctx(ctx).Where("id", id).One()
	if run["status"].String() != "partial" || run["failure_count"].Int() != 1 || run["finished_at"].String() == "" {
		t.Fatal(run)
	}
}
func TestApprovalRepeatedCursorFailsInsteadOfInfiniteLoop(t *testing.T) {
	ctx := setupDB(t)
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		respond(w, g.Map{"errcode": 0, "sp_no_list": []string{}, "new_next_cursor": "same"})
	})
	_, err := c.SyncApprovals(ctx, time.Now().Add(-time.Hour), time.Now())
	if err == nil || !strings.Contains(err.Error(), "repeated") {
		t.Fatal(err)
	}
}
func TestExplicitApprovalControlsRejectGuessing(t *testing.T) {
	var d approvalDetail
	_ = json.Unmarshal([]byte(mustJSON(approvalFixture("SP-1", "leave-tpl", 2))), &d)
	if _, err := decodeApproval(d, Mapping{AttendanceControlID: "start-guess"}, "leave"); err == nil {
		t.Fatal("guessed a field from unsupported control")
	}
	if _, err := decodeApproval(d, Mapping{AttendanceControlID: "missing"}, "leave"); err == nil {
		t.Fatal("guessed an absent control")
	}
	got, err := decodeApproval(d, Mapping{AttendanceControlID: "vacation-control", ReasonControlID: "reason-control"}, "leave")
	if err != nil || got.Hours != 2 || got.LeaveType != "年假" || got.Reason != "family" {
		t.Fatal(got, err)
	}
}
