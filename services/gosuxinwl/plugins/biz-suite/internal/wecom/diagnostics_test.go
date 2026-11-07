package wecom

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestCredentialChannelsAndLegacyEnvironment(t *testing.T) {
	for _, key := range []string{"WECOM_CORP_ID", "WECOM_AGENT_ID", "WECOM_CONTACT_SECRET", "WECOM_APP_SECRET", "WECOM_SECRET", "WECOM_CHECKIN_SECRET", "WECOM_APPROVAL_SECRET"} {
		t.Setenv(key, "")
	}
	t.Setenv("WECOM_CORP_ID", "corp")
	t.Setenv("WECOM_CONTACT_SECRET", "directory-only")
	t.Setenv("WECOM_APP_SECRET", "application-only")
	t.Setenv("WECOM_SECRET", "legacy-application")
	t.Setenv("WECOM_CHECKIN_SECRET", "obsolete-checkin")
	t.Setenv("WECOM_APPROVAL_SECRET", "obsolete-approval")
	c := NewFromEnv()
	if c.secret("contact") != "directory-only" {
		t.Fatal("contact must use its own credential")
	}
	for _, kind := range []string{"", "app", "checkin", "approval", "leave", "out", "trip", "unmapped"} {
		if c.secret(kind) != "application-only" {
			t.Fatalf("%s did not use application credential", kind)
		}
	}
	if c.secret("unknown") != "" || !c.Configured() {
		t.Fatal("unsupported scope must not resolve credentials; AgentID is not required for tokens")
	}
	if !reflect.DeepEqual(c.LegacyCredentialVariables, []string{"WECOM_CHECKIN_SECRET", "WECOM_APPROVAL_SECRET"}) {
		t.Fatal("obsolete variable names must be explicitly reported")
	}
	t.Setenv("WECOM_APP_SECRET", "")
	t.Setenv("WECOM_CONTACT_SECRET", "")
	c = NewFromEnv()
	if c.secret("app") != "legacy-application" || c.secret("contact") != "" || c.Configured() {
		t.Fatal("old secret is only an application alias, never a contact fallback")
	}
	t.Setenv("WECOM_SECRET", "")
	c = NewFromEnv()
	if c.secret("checkin") != "" || c.secret("approval") != "" {
		t.Fatal("obsolete checkin/approval secrets must not be silently adopted")
	}
}

func diagnosticClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gettoken" {
			secret := r.URL.Query().Get("corpsecret")
			if secret != "directory-private" && secret != "application-private" {
				t.Errorf("unexpected credential channel")
			}
			respond(w, map[string]any{"errcode": 0, "access_token": secret + "-token", "expires_in": 7200})
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	return &Client{HTTP: server.Client(), CorpID: "corp", AgentID: "1000002", ContactSecret: "directory-private", AppSecret: "application-private", BaseURL: server.URL, limiter: rate.NewLimiter(rate.Inf, 1)}
}

func assertDiagnosticSafe(t *testing.T, result DiagnosticResult) {
	t.Helper()
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"directory-private", "application-private", "hidden-upstream-body", "private-userid", "private-phone", "private-email", "private-sp-no"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("diagnostic disclosed forbidden data category %q", secret)
		}
	}
}

func TestContactDiagnosticReportsIDOnlyAccessWithoutSwitchingCredentials(t *testing.T) {
	ctx := setupDB(t)
	var paths []string
	c := diagnosticClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Query().Get("access_token") != "directory-private-token" {
			t.Error("contact probe must never fall back to application token")
		}
		switch r.URL.Path {
		case "/department/simplelist":
			respond(w, map[string]any{"errcode": 0, "department_id": []any{map[string]any{"id": 7, "parentid": 1}}})
		case "/department/list", "/user/list":
			if r.URL.Path == "/user/list" && r.URL.Query().Get("department_id") != "7" {
				t.Error("member probe must use an actual visible department ID")
			}
			respond(w, map[string]any{"errcode": 48009, "errmsg": "hidden-upstream-body directory-private application-private"})
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
		}
	})
	result, err := c.Diagnose(ctx, "contact")
	if err != nil || result.Status != "limited" || !result.CredentialConfigured || len(result.Checks) != 4 {
		t.Fatalf("unexpected result: status=%s checks=%d error=%v", result.Status, len(result.Checks), err)
	}
	if !reflect.DeepEqual(paths, []string{"/department/simplelist", "/department/list", "/user/list"}) {
		t.Fatal(paths)
	}
	for _, check := range result.Checks[2:] {
		if check.Code != 48009 {
			t.Fatal("permission failure must preserve the safe upstream code")
		}
		for _, guidance := range []string{"通讯录读取槽", "不会自动切换", "管理工具－通讯录同步", "企业自建应用 Secret", "可见范围", "可信 IP", "实际授权", "成员授权"} {
			if !strings.Contains(check.Message, guidance) {
				t.Fatalf("permission failure is missing credential guidance %q", guidance)
			}
		}
	}
	assertDiagnosticSafe(t, result)
	for _, table := range []string{"wecom_department", "wecom_user", "wecom_checkin_raw", "wecom_approval_raw", "wecom_sync_run", "wecom_sync_cursor", "biz_hrm_employee"} {
		if countRows(t, ctx, table) != 0 {
			t.Fatalf("diagnosis mutated %s", table)
		}
	}
	if !c.claim() {
		t.Fatal("diagnosis did not release operation lock")
	}
	c.release()
}

func TestContactDiagnosticReturnsOnlySampleVisibility(t *testing.T) {
	c := diagnosticClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/department/simplelist":
			respond(w, map[string]any{"errcode": 0, "department_id": []any{map[string]any{"id": 1}}})
		case "/department/list":
			respond(w, map[string]any{"errcode": 0, "department": []any{map[string]any{"id": 1, "name": "Office"}}})
		case "/user/list":
			respond(w, map[string]any{"errcode": 0, "userlist": []any{map[string]any{"userid": "private-userid", "name": "Private Name", "mobile": "private-phone", "email": "private-email"}}})
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
		}
	})
	result, err := c.Diagnose(context.Background(), "contact")
	if err != nil || result.Status != "success" {
		t.Fatalf("unexpected diagnostic state: %s %v", result.Status, err)
	}
	details := result.Checks[3].Details
	if details["sample_user_count"] != 1 || details["name_available"] != true || details["mobile_available"] != true || details["email_available"] != true {
		t.Fatal("only safe counts and field-presence flags must be returned")
	}
	assertDiagnosticSafe(t, result)
}

func TestApplicationDiagnosticUsesApplicationTokenAndSafeVisibleScope(t *testing.T) {
	c := diagnosticClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agent/get" || r.URL.Query().Get("agentid") != "1000002" || r.URL.Query().Get("access_token") != "application-private-token" {
			t.Errorf("incorrect application probe endpoint or credential")
		}
		respond(w, map[string]any{"errcode": 0, "agentid": 1000002, "name": "Attendance", "allow_userinfos": map[string]any{"user": []any{map[string]any{"userid": "private-userid"}}}, "allow_partys": map[string]any{"partyid": []int64{2, 7}}, "allow_tags": map[string]any{"tagid": []int64{9}}})
	})
	result, err := c.Diagnose(context.Background(), "app")
	if err != nil || result.Status != "success" || len(result.Checks) != 2 {
		t.Fatalf("application diagnosis failed: %s %v", result.Status, err)
	}
	d := result.Checks[1].Details
	if d["app_name"] != "Attendance" || d["visible_user_count"] != 1 || d["visible_tag_count"] != 1 || !reflect.DeepEqual(d["visible_department_ids"], []int64{2, 7}) {
		t.Fatal("safe scope summary is incomplete")
	}
	assertDiagnosticSafe(t, result)
}

func TestApplicationDiagnosticDoesNotMarkDisabledAppUsable(t *testing.T) {
	c := diagnosticClient(t, func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]any{"errcode": 0, "agentid": 1000002, "name": "Attendance", "close": 1})
	})
	result, err := c.Diagnose(context.Background(), "app")
	if err != nil || result.Status != "failed" || result.Checks[1].Status != "failed" || result.Checks[1].Details["app_enabled"] != false || !strings.Contains(result.Checks[1].Message, "停用") {
		t.Fatalf("disabled app must not be declared usable: state=%s error=%v", result.Status, err)
	}
}

func TestContactDiagnosticDoesNotTreatIdentifiersAsProfileNames(t *testing.T) {
	c := diagnosticClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/department/simplelist":
			respond(w, map[string]any{"errcode": 0, "department_id": []any{map[string]any{"id": 1}}})
		case "/department/list":
			respond(w, map[string]any{"errcode": 0, "department": []any{map[string]any{"id": 1, "name": ""}}})
		case "/user/list":
			respond(w, map[string]any{"errcode": 0, "userlist": []any{map[string]any{"userid": "private-userid", "name": "private-userid"}}})
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
		}
	})
	result, err := c.Diagnose(context.Background(), "contact")
	if err != nil || result.Status != "limited" || result.Checks[2].Details["department_name_available"] != false || result.Checks[3].Details["name_available"] != false {
		t.Fatalf("identifier-only results must not imply profile access: state=%s error=%v", result.Status, err)
	}
	assertDiagnosticSafe(t, result)
}

func TestCheckinAndApprovalDiagnosticsDoNotRequireDirectoryOrAgentID(t *testing.T) {
	for _, kind := range []string{"checkin", "approval"} {
		t.Run(kind, func(t *testing.T) {
			c := diagnosticClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("access_token") != "application-private-token" || r.Method != http.MethodPost {
					t.Error("business probes must use application token and POST")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if kind == "checkin" {
					if r.URL.Path != "/checkin/getcorpcheckinoption" || len(body) != 0 {
						t.Error("checkin diagnostic must use empty rules query")
					}
					respond(w, map[string]any{"errcode": 0, "group": []any{map[string]any{"groupname": "private-userid", "groupid": 4}}})
				} else {
					if r.URL.Path != "/oa/getapprovalinfo" || body["size"] != float64(1) || body["new_cursor"] != "" {
						t.Error("approval diagnostic must fetch at most one approval number")
					}
					respond(w, map[string]any{"errcode": 0, "sp_no_list": []string{"private-sp-no"}})
				}
			})
			c.ContactSecret, c.AgentID = "", ""
			result, err := c.Diagnose(context.Background(), kind)
			if err != nil || result.Status != "success" || !result.CredentialConfigured {
				t.Fatalf("business diagnosis incorrectly depended on directory/AgentID: %s %v", result.Status, err)
			}
			assertDiagnosticSafe(t, result)
		})
	}
}

func TestDiagnosticMissingInvalidAndBusyDoNotCallUpstream(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		respond(w, map[string]any{"errcode": 0})
	}))
	defer server.Close()
	c := &Client{CorpID: "corp", AppSecret: "app", BaseURL: server.URL}
	if result, err := c.Diagnose(context.Background(), "contact"); err != nil || result.Status != "missing" || result.CredentialConfigured {
		t.Fatal("missing contact credential must not fall back to app")
	}
	if result, err := c.Diagnose(context.Background(), "app"); err != nil || result.Status != "missing" || !result.CredentialConfigured {
		t.Fatal("AgentID is required only for application identity probe")
	}
	if !c.claim() {
		t.Fatal("could not acquire operation lock")
	}
	if _, err := c.Diagnose(context.Background(), "checkin"); !errors.Is(err, ErrSyncRunning) {
		t.Fatal("active operation must prevent another probe")
	}
	if _, err := c.Diagnose(context.Background(), "https://example.invalid"); err == nil || errors.Is(err, ErrSyncRunning) {
		t.Fatal("invalid kind must be rejected before acquiring operation lock")
	}
	c.release()
	if calls.Load() != 0 {
		t.Fatal("invalid/missing/busy diagnostics must not contact upstream")
	}
}

func TestDiagnosticTokenFailureIsSanitizedAndUnlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gettoken" {
			t.Error("business API must not be called after token failure")
		}
		respond(w, map[string]any{"errcode": 40001, "errmsg": "hidden-upstream-body directory-private application-private"})
	}))
	defer server.Close()
	c := &Client{HTTP: server.Client(), CorpID: "corp", AppSecret: "application-private", BaseURL: server.URL}
	result, err := c.Diagnose(context.Background(), "approval")
	if err != nil || result.Status != "failed" || result.Checks[0].Code != 40001 {
		t.Fatalf("token failure must be a safe diagnostic result: %s %v", result.Status, err)
	}
	assertDiagnosticSafe(t, result)
	if !c.claim() {
		t.Fatal("failed diagnostic retained operation lock")
	}
	c.release()
}

func TestDiagnosticOperationBlocksSyncAndSaveWithoutPersisting(t *testing.T) {
	ctx := setupDB(t)
	entered, unblock, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	c := diagnosticClient(t, func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-unblock
		respond(w, map[string]any{"errcode": 0, "group": []any{}})
	})
	go func() { _, err := c.Diagnose(ctx, "checkin"); done <- err }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("diagnostic request did not start")
	}
	if _, _, err := c.Run(ctx, "contact", time.Time{}, time.Time{}); !errors.Is(err, ErrSyncRunning) {
		t.Error("sync must be excluded while diagnostic is active")
	}
	if err := c.SaveSettings(ctx, Settings{CorpID: "other-corp", AgentID: "1000002"}); err == nil {
		t.Error("settings save must be excluded while diagnostic is active")
	}
	close(unblock)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if countRows(t, ctx, "wecom_sync_run") != 0 || countRows(t, ctx, "wecom_settings") != 0 {
		t.Fatal("diagnostics must not create business synchronization or settings records")
	}
}
