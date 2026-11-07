package wecom

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/migration"
)

func TestSettingsPersistenceGuardsAndAdditiveMigration(t *testing.T) {
	ctx := setupDB(t)
	v1, _ := core.Model("biz_schema_version").Where("version", 1).Value("checksum")
	c := &Client{CorpID: "env-corp", AgentID: "100", ContactSecret: "contact-private-do-not-persist", AppSecret: "private-do-not-persist"}
	if err := c.LoadSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if !c.Settings().IncludeSubdepartments || c.CorpID != "env-corp" {
		t.Fatal("environment/defaults lost")
	}
	in := Settings{CorpID: " new-corp ", AgentID: "200", IncludeDepartments: []int64{2, 2, 1}, ExcludeUsers: []string{" bob ", "bob"}, IncludeSubdepartments: true}
	c.tokens = map[string]cachedToken{"old": {Value: "old-token", Until: time.Now().Add(time.Hour)}}
	if err := c.SaveSettings(ctx, in); err != nil {
		t.Fatal(err)
	}
	if c.CorpID != "new-corp" || len(c.tokens) != 0 {
		t.Fatal("company/token cache not updated")
	}
	row, _ := core.Model("wecom_settings").Where("id", 1).One()
	if strings.Contains(row["settings_json"].String()+mustJSON(c.Status()), "private-do-not-persist") {
		t.Fatal("secret leaked")
	}
	d := &Client{CorpID: "env-corp", AgentID: "100"}
	if err := d.LoadSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Settings().IncludeDepartments, []int64{1, 2}) || d.CorpID != "new-corp" || d.AgentID != "200" {
		t.Fatal("saved settings not reloaded")
	}
	if !c.claim() {
		t.Fatal("lock")
	}
	if err := c.SaveSettings(ctx, Settings{}); err == nil {
		t.Fatal("settings changed during a job")
	}
	c.release()
	if err := c.SaveSettings(ctx, Settings{AgentID: "invalid"}); err == nil {
		t.Fatal("invalid agent accepted")
	}
	if err := c.SaveSettings(ctx, Settings{}); err != nil || c.CorpID != "env-corp" {
		t.Fatal("environment fallback", err)
	}
	seedRawUser(t, ctx, "alice")
	if err := c.SaveSettings(ctx, Settings{CorpID: "other-corp"}); err == nil {
		t.Fatal("tenant mixing allowed")
	}
	if err := migration.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}
	again, _ := core.Model("biz_schema_version").Where("version", 1).Value("checksum")
	if v1.String() != again.String() || countRows(t, ctx, "biz_schema_version") != 7 || countRows(t, ctx, "wecom_settings") != 1 {
		t.Fatal("migration not additive/idempotent")
	}
}

func TestDepartmentAndMemberScopeActuallyFiltersContactAndCheckin(t *testing.T) {
	ctx := setupDB(t)
	var memberDepartments, requestedUsers []string
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/department/list":
			respond(w, g.Map{"errcode": 0, "department": []Department{{ID: 1, Name: "root"}, {ID: 2, ParentID: 1, Name: "allowed"}, {ID: 3, ParentID: 2, Name: "excluded"}, {ID: 4, ParentID: 2, Name: "child"}}})
		case "/user/list":
			memberDepartments = append(memberDepartments, r.URL.Query().Get("department_id"))
			users := []User{{UserID: "alice", Name: "Alice", Department: []int64{2}, Status: 1}, {UserID: "bob", Name: "Bob", Department: []int64{2}, Status: 1}, {UserID: "cross", Name: "Cross", Department: []int64{2, 3}, Status: 1}, {UserID: "dave", Name: "Dave", Department: []int64{4}, Status: 1}}
			respond(w, g.Map{"errcode": 0, "userlist": users})
		case "/checkin/getcheckindata":
			var in struct {
				Users []string `json:"useridlist"`
			}
			_ = json.NewDecoder(r.Body).Decode(&in)
			requestedUsers = append(requestedUsers, in.Users...)
			var clocks []Checkin
			for _, uid := range in.Users {
				clocks = append(clocks, Checkin{UserID: uid, Type: "上班打卡", Time: 1700000000})
			}
			respond(w, g.Map{"errcode": 0, "checkindata": clocks})
		default:
			t.Error(r.URL.Path)
		}
	})
	if err := c.LoadSettings(ctx); err != nil {
		t.Fatal(err)
	}
	s := Settings{IncludeDepartments: []int64{2}, ExcludeDepartments: []int64{3}, ExcludeUsers: []string{"bob"}, IncludeSubdepartments: true}
	if err := c.SaveSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if n, e := c.SyncContact(ctx); e != nil || n != 2 {
			t.Fatal(n, e)
		}
	}
	if strings.Join(memberDepartments, ",") != "2,4,2,4" || countRows(t, ctx, "biz_hrm_employee") != 2 || countRows(t, ctx, "wecom_user") != 2 || countRows(t, ctx, "wecom_department") != 4 {
		t.Fatal("contact filtering/idempotence failed", memberDepartments)
	}
	seedRawUser(t, ctx, "bob")
	if _, e := core.Model("wecom_user").Where("wecom_userid", "bob").Update(g.Map{"department_ids": "[2]"}); e != nil {
		t.Fatal(e)
	}
	start, end := time.Unix(1699990000, 0), time.Unix(1700010000, 0)
	for i := 0; i < 2; i++ {
		if n, e := c.SyncCheckin(ctx, start, end); e != nil || n != 2 {
			t.Fatal(n, e)
		}
	}
	if strings.Join(requestedUsers, ",") != "alice,dave,alice,dave" || countRows(t, ctx, "biz_hrm_attendance_clock") != 2 {
		t.Fatal("checkin scope/idempotence", requestedUsers)
	}
	s.IncludeUsers = []string{"alice", "bob"}
	s.ExcludeUsers = []string{"bob"}
	if err := c.SaveSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	f, e := c.filter(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if !f.allows("alice", []int64{4}) || f.allows("alice", []int64{4, 3}) || f.allows("bob", []int64{2}) || f.allows("dave", []int64{2}) || f.allows("alice", nil) {
		t.Fatal("scope precedence/intersection failed")
	}
	s.IncludeSubdepartments = false
	if err := c.SaveSettings(ctx, s); err != nil {
		t.Fatal(err)
	}
	f, _ = c.filter(ctx)
	if f.allows("alice", []int64{4}) {
		t.Fatal("child flag ignored")
	}
	if countRows(t, ctx, "biz_hrm_employee") != 2 {
		t.Fatal("filter removed existing history")
	}
}

func TestSeparateApprovalTypesCursorsFilteringAndTripCancellation(t *testing.T) {
	ctx := setupDB(t)
	seedEmployee(t, ctx, "alice")
	seedRawUser(t, ctx, "alice")
	for _, kind := range []string{"leave", "out", "trip"} {
		if err := SaveTemplate(ctx, Template{TemplateID: kind + "-tpl", ApprovalType: kind, Enabled: 1, Mapping: Mapping{AttendanceControlID: "period"}}); err != nil {
			t.Fatal(err)
		}
	}
	phase := 0
	queried := []string{}
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch r.URL.Path {
		case "/oa/getapprovalinfo":
			filters, ok := req["filters"].([]any)
			if !ok || len(filters) != 1 {
				t.Error("typed job omitted template filter")
				respond(w, g.Map{"errcode": 0})
				return
			}
			f := filters[0].(map[string]any)
			if f["key"] != "template_id" {
				t.Error(f)
			}
			tpl := f["value"].(string)
			queried = append(queried, tpl)
			ids := []string{strings.TrimSuffix(tpl, "-tpl") + "-sp"}
			if phase > 0 {
				ids = nil
			}
			respond(w, g.Map{"errcode": 0, "sp_no_list": ids})
		case "/oa/getapprovaldetail":
			sp := req["sp_no"].(string)
			kind := strings.TrimSuffix(sp, "-sp")
			state := 2
			if phase > 0 {
				state = 7
			}
			respond(w, g.Map{"errcode": 0, "info": g.Map{"sp_no": sp, "template_id": kind + "-tpl", "sp_status": state, "applyer": g.Map{"userid": "alice"}, "apply_data": g.Map{"contents": []any{g.Map{"id": "period", "control": "Attendance", "value": g.Map{"attendance": g.Map{"type": map[string]int{"leave": 1, "out": 4, "trip": 3}[kind], "date_range": g.Map{"new_begin": 1700000000, "new_end": 1700007200, "new_duration": 7200}}}}}}}})
		default:
			t.Error(r.URL.Path)
		}
	})
	if err := c.LoadSettings(ctx); err != nil {
		t.Fatal(err)
	}
	start, end := time.Unix(1699990000, 0), time.Unix(1700010000, 0)
	for i, kind := range []string{"leave", "out", "trip"} {
		if _, n, e := c.Run(ctx, kind, start, end.Add(time.Duration(i)*time.Hour)); e != nil || n != 1 {
			t.Fatal(kind, n, e)
		}
		if countRows(t, ctx, "wecom_approval_raw") != i+1 {
			t.Fatal("sync imported another type")
		}
		cursor, _ := core.Model("wecom_sync_cursor").Where("sync_type", kind).Value("cursor_value")
		if cursor.String() != fmt.Sprint(end.Add(time.Duration(i)*time.Hour).Unix()) {
			t.Fatal("independent cursor missing")
		}
	}
	if strings.Join(queried, ",") != "leave-tpl,out-tpl,trip-tpl" || countRows(t, ctx, "biz_hrm_attendance_trip") != 1 || countRows(t, ctx, "biz_hrm_attendance_out") != 1 {
		t.Fatal("approval types mixed", queried)
	}
	if err := c.SaveSettings(ctx, Settings{ExcludeUsers: []string{"alice"}}); err != nil {
		t.Fatal(err)
	}
	if _, n, e := c.Run(ctx, "trip", start, end); e != nil || n != 0 {
		t.Fatal("excluded approved applicant imported", n, e)
	}
	phase = 1
	if _, n, e := c.Run(ctx, "trip", start, end); e != nil || n != 1 {
		t.Fatal("excluded historical cancellation lost", n, e)
	}
	trip, _ := core.Model("biz_hrm_attendance_trip").One()
	leave, _ := core.Model("biz_hrm_attendance_leave").One()
	if trip["approval_status"].String() != "canceled" || leave["approval_status"].String() != "approved" {
		t.Fatal("trip cancellation affected another type")
	}
	if countRows(t, ctx, "biz_hrm_attendance_trip") != 1 {
		t.Fatal("duplicate trip")
	}
}

func TestUnmappedDiscoveryRetainsRawWithoutImportingMappedApprovals(t *testing.T) {
	ctx := setupDB(t)
	seedEmployee(t, ctx, "alice")
	installTemplate(t, ctx)
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oa/getapprovalinfo" {
			respond(w, g.Map{"errcode": 0, "sp_no_list": []string{"mapped", "unknown"}})
			return
		}
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		template := "leave-tpl"
		if in["sp_no"] == "unknown" {
			template = "not-configured"
		}
		respond(w, g.Map{"errcode": 0, "info": approvalFixture(in["sp_no"], template, 2)})
	})
	_, _, err := c.Run(ctx, "unmapped", time.Unix(1699990000, 0), time.Unix(1700010000, 0))
	if err == nil || countRows(t, ctx, "wecom_approval_raw") != 1 || countRows(t, ctx, "biz_hrm_attendance_leave") != 0 {
		t.Fatal("unmapped discovery bypassed type isolation", err)
	}
	row, _ := core.Model("wecom_approval_raw").One()
	if row["sp_no"].String() != "unknown" || row["processing_error"].String() == "" {
		t.Fatal("unmapped approval not retained for review")
	}
}
