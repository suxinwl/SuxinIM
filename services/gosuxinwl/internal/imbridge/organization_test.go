package imbridge

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func directoryItems(t *testing.T, value any) []DirectoryPerson {
	t.Helper()
	return value.(map[string]any)["items"].([]DirectoryPerson)
}

func TestLiveDirectorySearchNameWorkNumberMobileAndDepartmentTogether(t *testing.T) {
	ctx, s, r := fixture(t)
	provision(t, ctx, s, r)
	_, err := s.DB.ExecContext(ctx, `UPDATE biz_hrm_employee SET mobile='13800138000' WHERE id='c'`)
	must(t, err)
	for _, term := range []string{"新员工", "0003", "13800138000", "138 0013 8000"} {
		value, e := s.DirectoryForUID(ctx, 1, term, "2", 1, 50)
		must(t, e)
		items := directoryItems(t, value)
		if len(items) != 1 || items[0].UserID != UserID("c") || !items[0].CanDirectChat {
			t.Fatal("same-scope live employee was not searchable", term)
		}
		raw, _ := json.Marshal(value)
		if strings.Contains(string(raw), "13800138000") || strings.Contains(string(raw), `"mobile"`) {
			t.Fatal("search-only mobile value was disclosed")
		}
	}
	value, e := s.DirectoryForUID(ctx, 1, "0003", "1", 1, 50)
	must(t, e)
	if len(directoryItems(t, value)) != 0 {
		t.Fatal("keyword bypassed the department filter")
	}
	value, e = s.DirectoryForUID(ctx, 1, "", "", 1, 50)
	must(t, e)
	for _, p := range directoryItems(t, value) {
		if p.UserID == UserID("a") && p.CanDirectChat {
			t.Fatal("self chat was authorized")
		}
	}
}

func TestDirectoryAndPersonHideOtherBusinessAndImmediatelyDisabledPeers(t *testing.T) {
	ctx, s, r := fixture(t)
	provision(t, ctx, s, r)
	_, err := s.DB.ExecContext(ctx, `UPDATE gf_auth_dept SET business_id=9 WHERE id=2`)
	must(t, err)
	value, e := s.DirectoryForUID(ctx, 1, "", "", 1, 50)
	must(t, e)
	if len(directoryItems(t, value)) != 1 {
		t.Fatal("other business employees leaked")
	}
	for _, d := range value.(map[string]any)["departments"].([]DirectoryDepartment) {
		if d.ID == "2" {
			t.Fatal("other business department leaked")
		}
	}
	if _, e = s.DirectChatPerson(ctx, 1, UserID("c")); e == nil {
		t.Fatal("cross-business direct chat authorized")
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE gf_auth_dept SET status=1 WHERE id=2`)
	must(t, err)
	if _, e = s.DirectChatPerson(ctx, 1, UserID("c")); e == nil {
		t.Fatal("disabled foreign department fell back to business zero")
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE gf_auth_dept SET status=0 WHERE id=2`)
	must(t, err)
	_, err = s.DB.ExecContext(ctx, `UPDATE gf_auth_dept SET business_id=0 WHERE id=2`)
	must(t, err)
	var uid int64
	must(t, s.DB.QueryRowContext(ctx, `SELECT user_id FROM biz_hrm_employee WHERE id='c'`).Scan(&uid))
	var password string
	must(t, s.DB.QueryRowContext(ctx, `SELECT password FROM gf_admin WHERE id=?`, uid).Scan(&password))
	for _, change := range []string{`UPDATE gf_admin SET status=1 WHERE id=?`, `UPDATE gf_admin SET password='changed' WHERE id=?`, `DELETE FROM gf_auth_role_access WHERE uid=?`} {
		_, err = s.DB.ExecContext(ctx, change, uid)
		must(t, err)
		value, e = s.DirectoryForUID(ctx, 1, "0003", "", 1, 50)
		must(t, e)
		if len(directoryItems(t, value)) != 0 {
			t.Fatal("stale ready projection exposed an ineligible employee")
		}
		if _, e = s.DirectChatPerson(ctx, 1, UserID("c")); e == nil {
			t.Fatal("stale projection authorized person lookup")
		}
		if strings.Contains(change, "status=1") {
			_, err = s.DB.ExecContext(ctx, `UPDATE gf_admin SET status=0 WHERE id=?`, uid)
			must(t, err)
		}
		if strings.Contains(change, "password=") {
			_, err = s.DB.ExecContext(ctx, `UPDATE gf_admin SET password=? WHERE id=?`, password, uid)
			must(t, err)
		}
	}
}

func TestDepartmentDispatchRejectsStaleVersionAndOffBeforeReconcile(t *testing.T) {
	ctx, s, r := fixture(t)
	provision(t, ctx, s, r)
	_, err := s.DB.ExecContext(ctx, `UPDATE gf_auth_dept SET im_group_enabled=1,leader_uid=1 WHERE id=2`)
	must(t, err)
	must(t, s.Reconcile(ctx))
	var raw string
	must(t, s.DB.QueryRowContext(ctx, `SELECT projection_json FROM biz_openim_department WHERE department_id='2'`).Scan(&raw))
	var d Department
	must(t, json.Unmarshal([]byte(raw), &d))
	if !s.departmentDispatchAllowed(ctx, d) {
		t.Fatal("current enabled task was denied")
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE biz_openim_department SET desired_version=desired_version+1 WHERE department_id='2'`)
	must(t, err)
	must(t, s.Drain(ctx, r))
	if len(r.departments) != 0 {
		t.Fatal("stale enabled task reached the remote")
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE biz_openim_department SET desired_version=? WHERE department_id='2'`, d.Version)
	must(t, err)
	_, err = s.DB.ExecContext(ctx, `UPDATE biz_openim_outbox SET status='pending' WHERE kind='department'`)
	must(t, err)
	_, err = s.DB.ExecContext(ctx, `UPDATE gf_auth_dept SET im_group_enabled=0 WHERE id=2`)
	must(t, err)
	// No reconcile runs between turning the switch off and draining old work.
	must(t, s.Drain(ctx, r))
	if len(r.departments) != 0 {
		t.Fatal("previously selected enabled task escaped the current off switch")
	}
}

func TestExplicitDepartmentOptInOverridesGlobalFalseAndOffPreservesGroup(t *testing.T) {
	ctx, s, r := fixture(t)
	provision(t, ctx, s, r)
	if len(r.departments) != 0 {
		t.Fatal("legacy global true enabled departments without per-department opt-in")
	}
	t.Setenv("GOSUXIN_IM_AUTO_DEPARTMENT_GROUPS", "false")
	_, err := s.DB.ExecContext(ctx, `UPDATE gf_auth_dept SET im_group_enabled=1,leader_uid=1 WHERE id=2`)
	must(t, err)
	must(t, s.Reconcile(ctx))
	must(t, s.Drain(ctx, r))
	if len(r.departments) != 1 {
		t.Fatal("explicit switch did not select exactly one department")
	}
	d := r.departments["2"]
	if !d.GroupEnabled || d.LeaderUserID != UserID("a") || len(d.MemberUserIDs) != 2 || d.Frozen {
		t.Fatal("owner was not included in the managed department group")
	}
	before, _ := json.Marshal(d)
	_, err = s.DB.ExecContext(ctx, `UPDATE gf_auth_dept SET im_group_enabled=0 WHERE id=2`)
	must(t, err)
	must(t, s.Reconcile(ctx))
	must(t, s.Drain(ctx, r))
	after, _ := json.Marshal(r.departments["2"])
	if string(before) != string(after) {
		t.Fatal("switching off changed group membership, frozen state or history")
	}
	var pending int
	must(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM biz_openim_outbox WHERE kind='department' AND status IN ('pending','failed')`).Scan(&pending))
	if pending != 0 {
		t.Fatal("disabled department retained automatic work")
	}
}

func TestDepartmentOwnerMustBeEligibleAndSameBusiness(t *testing.T) {
	ctx, s, r := fixture(t)
	provision(t, ctx, s, r)
	if s.ValidateDepartmentGroup(ctx, 2, 0) == nil {
		t.Fatal("missing owner accepted")
	}
	if s.ValidateDepartmentGroup(ctx, 2, 2) == nil {
		t.Fatal("non-employee owner accepted")
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE gf_auth_dept SET business_id=9 WHERE id=2`)
	must(t, err)
	if s.ValidateDepartmentGroup(ctx, 2, 1) == nil {
		t.Fatal("foreign business owner accepted")
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE gf_auth_dept SET business_id=0,im_group_enabled=1,leader_uid=1 WHERE id=2`)
	must(t, err)
	must(t, s.Reconcile(ctx))
	must(t, s.Drain(ctx, r))
	_, err = s.DB.ExecContext(ctx, `UPDATE gf_admin SET status=1 WHERE id=1`)
	must(t, err)
	must(t, s.Reconcile(ctx))
	must(t, s.Drain(ctx, r))
	var state string
	must(t, s.DB.QueryRowContext(ctx, `SELECT status FROM biz_openim_department WHERE department_id='2'`).Scan(&state))
	if state != "blocked" {
		t.Fatal("lost owner did not stop automatic group synchronization")
	}
}

func TestDirectChatAccessHMACUsesLivePeerAndRejectsSelfAndOtherBusiness(t *testing.T) {
	ctx, s, r := fixture(t)
	provision(t, ctx, s, r)
	now := time.Now()
	key := strings.Repeat("e", 48)
	svc := &Service{Store: s, Remote: r, Cache: newFakeCache(), Now: func() time.Time { return now }}
	handler := svc.internalAccessHandler(key, &accessHMACVerifier{})
	n := 0
	check := func(peer string, want int) {
		t.Helper()
		n++
		raw, _ := json.Marshal(AccessCheckRequest{UserID: UserID("a"), Platform: 3, ClientIP: "192.168.10.50", Action: "direct_chat", ResourcePath: peer})
		w := httptest.NewRecorder()
		handler(w, signedAccessRequest(t, key, raw, fmt.Sprintf("%064x", n), now))
		if w.Code != want {
			t.Fatalf("direct chat access got %d want %d", w.Code, want)
		}
	}
	check(UserID("c"), 200)
	_, err := s.DB.ExecContext(ctx, `INSERT INTO biz_openim_employee_access(employee_id,allow_lan,allow_pc,allow_mobile,allow_internet,allow_internet_download,revision,updated_at,updated_by) VALUES('c',0,0,0,0,0,1,0,1)`)
	must(t, err)
	check(UserID("c"), 200) // A recipient need not be online on the sender's device/network.
	check(UserID("a"), 403)
	_, err = s.DB.ExecContext(ctx, `UPDATE gf_auth_dept SET business_id=9 WHERE id=2`)
	must(t, err)
	check(UserID("c"), 403)
	_, err = s.DB.ExecContext(ctx, `UPDATE gf_auth_dept SET business_id=0 WHERE id=2`)
	must(t, err)
	_, err = s.DB.ExecContext(ctx, `UPDATE biz_hrm_employee SET status='inactive' WHERE id='c'`)
	must(t, err)
	check(UserID("c"), 403)
}
