package imbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func leaderOptionItems(t *testing.T, value any) []map[string]any {
	t.Helper()
	return value.(map[string]any)["items"].([]map[string]any)
}

// Seed already-provisioned synthetic employees. No host, IM or HRM service is
// involved; each row carries the same live credentials and exact role grants
// that the real candidate authorization must check.
func seedLeaderOptions(t *testing.T, ctx context.Context, s *SQLStore, count int) {
	t.Helper()
	_, err := s.DB.ExecContext(ctx, `INSERT INTO gf_auth_role(id,name,rules,btns,status) VALUES(900,'synthetic leaders','','3,4,5',0)`)
	must(t, err)
	tx, err := s.DB.BeginTx(ctx, nil)
	must(t, err)
	defer tx.Rollback()
	for i := 0; i < count; i++ {
		uid := int64(1000 + i)
		id, account := fmt.Sprintf("leader-%04d", i), fmt.Sprintf("BATCH-%04d", i)
		p := Projection{UserID: UserID(id), Account: account, Nickname: fmt.Sprintf("Synthetic leader %04d", i), DepartmentID: "2", DepartmentName: "另一部门", EmployeeID: id, HostUID: uid, Enabled: true, Version: 1, AuthFingerprint: digest("synthetic-password:synthetic-salt")}
		raw, e := json.Marshal(p)
		must(t, e)
		_, err = tx.ExecContext(ctx, `INSERT INTO gf_admin(id,username,password,salt,status,dept_id) VALUES(?,?,'synthetic-password','synthetic-salt',0,2)`, uid, account)
		must(t, err)
		_, err = tx.ExecContext(ctx, `INSERT INTO biz_hrm_employee(id,employee_no,user_id,dept_id,name,status,position) VALUES(?,?,?,2,?,'active','staff')`, id, account, uid, p.Nickname)
		must(t, err)
		_, err = tx.ExecContext(ctx, `INSERT INTO biz_openim_employee(employee_id,openim_user_id,uid,desired_hash,desired_version,applied_version,enabled,provisioned,status,last_error,projection_json,updated_at) VALUES(?,?,?,'synthetic',1,1,1,1,'ready','',?,0)`, id, p.UserID, uid, string(raw))
		must(t, err)
		_, err = tx.ExecContext(ctx, `INSERT INTO gf_auth_role_access(uid,role_id) VALUES(?,900)`, uid)
		must(t, err)
	}
	must(t, tx.Commit())
}

func TestDepartmentLeaderOptionsUsesBoundedSQLAndPagesAfterAuthorization(t *testing.T) {
	for _, count := range []int{30, 420} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			ctx, s, remote := fixture(t)
			provision(t, ctx, s, remote)
			seedLeaderOptions(t, ctx, s, count)
			queries := countedDirectoryDB(t, s)
			s.DB.SetMaxOpenConns(1)
			value, err := s.DepartmentLeaderOptions(ctx, "", 1, 2, 7)
			must(t, err)
			page := value.(map[string]any)
			items := leaderOptionItems(t, value)
			if page["total"] != count+2 || page["page"] != 2 || page["pageSize"] != 7 || len(items) != 7 || items[0]["employee_no"] != "BATCH-0005" {
				t.Fatal("leader total, ordering or page contract changed")
			}
			limit := int64(9)
			if count > 400 {
				limit = 13
			}
			if n := queries.Load(); n > limit {
				t.Fatalf("%d employees required %d SELECTs; expected at most %d request-local batch queries", count+2, n, limit)
			} else {
				t.Logf("%d eligible employees: %d SELECTs, full total before page slicing", count+2, n)
			}
		})
	}
}

func TestDepartmentLeaderOptionsSearchAndPrivateFields(t *testing.T) {
	ctx, s, remote := fixture(t)
	provision(t, ctx, s, remote)
	_, err := s.DB.ExecContext(ctx, `UPDATE biz_hrm_employee SET mobile='13800138000' WHERE id='c'`)
	must(t, err)
	_, err = s.DB.ExecContext(ctx, `UPDATE gf_admin SET mobile='13900139000' WHERE id=(SELECT user_id FROM biz_hrm_employee WHERE id='c')`)
	must(t, err)
	for _, term := range []string{"新员工", "0003", "另一部门", "138 0013 8000", "139-0013-9000"} {
		value, e := s.DepartmentLeaderOptions(ctx, term, 1, 1, 20)
		must(t, e)
		items := leaderOptionItems(t, value)
		if len(items) != 1 || items[0]["id"] != singleDirectoryUID(t, s, "c") || items[0]["employee_no"] != "0003" {
			t.Fatalf("leader search did not match same-business employee for %q", term)
		}
		raw, e := json.Marshal(value)
		must(t, e)
		for _, private := range []string{"13800138000", "13900139000", "authFingerprint", "password", "salt", "hostUID"} {
			if strings.Contains(string(raw), private) {
				t.Fatal("candidate response disclosed a search-only or credential field")
			}
		}
	}
}

func TestDepartmentLeaderOptionsRejectsStaleAndIneligibleCandidates(t *testing.T) {
	changes := map[string]string{
		"disabled":      `UPDATE gf_admin SET status=1 WHERE id=?`,
		"deleted":       `UPDATE gf_admin SET deletetime='2026-11-05' WHERE id=?`,
		"password":      `UPDATE gf_admin SET password='changed' WHERE id=?`,
		"employee":      `UPDATE biz_hrm_employee SET status='inactive' WHERE user_id=?`,
		"disabled IM":   `UPDATE biz_openim_employee SET enabled=0 WHERE uid=?`,
		"not ready":     `UPDATE biz_openim_employee SET status='pending' WHERE uid=?`,
		"version":       `UPDATE biz_openim_employee SET desired_version=desired_version+1 WHERE uid=?`,
		"role":          `DELETE FROM gf_auth_role_access WHERE uid=?`,
		"disabled role": `UPDATE gf_auth_role SET status=1 WHERE id=900 AND ?>0`,
		"disabled rule": `UPDATE gf_auth_rule SET status=1 WHERE id=5 AND ?>0`,
		"other scope":   `UPDATE gf_auth_dept SET business_id=9 WHERE id=2 AND ?>0`,
		"locked":        `INSERT INTO gf_account_security(uid,must_change,locked_until) VALUES(?,0,4102444800)`,
		"must change":   `INSERT INTO gf_account_security(uid,must_change,locked_until) VALUES(?,1,0)`,
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			ctx, s, remote := fixture(t)
			provision(t, ctx, s, remote)
			uid := singleDirectoryUID(t, s, "c")
			_, err := s.DB.ExecContext(ctx, `INSERT INTO gf_auth_role(id,name,rules,btns,status) VALUES(900,'isolated role','','3,4,5',0)`)
			must(t, err)
			_, err = s.DB.ExecContext(ctx, `DELETE FROM gf_auth_role_access WHERE uid=?`, uid)
			must(t, err)
			_, err = s.DB.ExecContext(ctx, `INSERT INTO gf_auth_role_access(uid,role_id) VALUES(?,900)`, uid)
			must(t, err)
			_, err = s.DB.ExecContext(ctx, change, uid)
			must(t, err)
			value, err := s.DepartmentLeaderOptions(ctx, "0003", 1, 1, 20)
			must(t, err)
			if len(leaderOptionItems(t, value)) != 0 || value.(map[string]any)["total"] != 0 {
				t.Fatal("stale ready projection exposed an ineligible leader")
			}
		})
	}
}

func TestDepartmentLeaderOptionsScopeAndEmptyPageContract(t *testing.T) {
	ctx, s, remote := fixture(t)
	provision(t, ctx, s, remote)
	_, err := s.DB.ExecContext(ctx, `UPDATE gf_auth_dept SET business_id=9 WHERE id=2`)
	must(t, err)
	for _, department := range []int64{0, 1, 2} {
		value, e := s.DepartmentLeaderOptions(ctx, "", department, 1, 20)
		must(t, e)
		items := leaderOptionItems(t, value)
		want := int64(1)
		if department == 2 {
			want = singleDirectoryUID(t, s, "c")
		}
		if len(items) != 1 || items[0]["id"] != want {
			t.Fatal("department business scope changed")
		}
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE gf_auth_dept SET status=1 WHERE id=2`)
	must(t, err)
	for _, department := range []int64{2, 999} {
		value, e := s.DepartmentLeaderOptions(ctx, "", department, 0, 500)
		must(t, e)
		if value.(map[string]any)["page"] != 1 || value.(map[string]any)["pageSize"] != 100 || value.(map[string]any)["total"] != 0 || leaderOptionItems(t, value) == nil {
			t.Fatal("unavailable department must have an empty array and normalized page metadata")
		}
	}
	value, err := s.DepartmentLeaderOptions(ctx, "", 1, 1000001, 0)
	must(t, err)
	if value.(map[string]any)["page"] != 1000000 || value.(map[string]any)["pageSize"] != 20 || value.(map[string]any)["total"] != 1 || len(leaderOptionItems(t, value)) != 0 {
		t.Fatal("out-of-range page changed the live total or defaults")
	}
}

func TestDepartmentLeaderOptionsCanceledContextFailsWithoutSuccess(t *testing.T) {
	ctx, s, remote := fixture(t)
	provision(t, ctx, s, remote)
	ctx, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancel()
	if _, err := s.DepartmentLeaderOptions(ctx, "", 1, 1, 20); err == nil {
		t.Fatal("canceled query returned a successful candidate page")
	}
}

func TestDepartmentLeaderOptionsRechecksChangesBeforeCountingAndPaging(t *testing.T) {
	for _, change := range []string{"disabled", "password", "role", "employee", "scope", "phone", "projection", "target disabled", "target scope"} {
		t.Run(change, func(t *testing.T) {
			ctx, s, remote := fixture(t)
			provision(t, ctx, s, remote)
			uid := singleDirectoryUID(t, s, "c")
			_, err := s.DB.ExecContext(ctx, `INSERT INTO gf_auth_role(id,name,rules,btns,status) VALUES(900,'isolated role','','3,4,5',0)`)
			must(t, err)
			_, err = s.DB.ExecContext(ctx, `DELETE FROM gf_auth_role_access WHERE uid=?`, uid)
			must(t, err)
			_, err = s.DB.ExecContext(ctx, `INSERT INTO gf_auth_role_access(uid,role_id) VALUES(?,900)`, uid)
			must(t, err)
			_, err = s.DB.ExecContext(ctx, `UPDATE biz_hrm_employee SET mobile='13800138000' WHERE id='c'`)
			must(t, err)
			passes := 0
			check := func(ctx context.Context, uids []int64) (map[int64]directoryLiveProfile, error) {
				live, e := s.directoryLiveBatch(ctx, uids)
				if e != nil {
					return nil, e
				}
				passes++
				if passes == 1 {
					query := map[string]string{
						"disabled":        `UPDATE gf_admin SET status=1 WHERE id=?`,
						"password":        `UPDATE gf_admin SET password='changed' WHERE id=?`,
						"role":            `UPDATE gf_auth_role SET status=1 WHERE id=900 AND ?>0`,
						"employee":        `UPDATE biz_hrm_employee SET status='inactive' WHERE user_id=?`,
						"scope":           `UPDATE gf_auth_dept SET business_id=9 WHERE id=2 AND ?>0`,
						"phone":           `UPDATE biz_hrm_employee SET mobile='13900139000' WHERE user_id=?`,
						"projection":      `UPDATE biz_openim_employee SET desired_version=desired_version+1 WHERE uid=?`,
						"target disabled": `UPDATE gf_auth_dept SET status=1 WHERE id=1 AND ?>0`,
						"target scope":    `UPDATE gf_auth_dept SET business_id=9 WHERE id=1 AND ?>0`,
					}[change]
					_, e = s.DB.ExecContext(ctx, query, uid)
					must(t, e)
				}
				return live, nil
			}
			keyword, page := "", 2
			if change == "phone" {
				keyword, page = "13800138000", 1
			}
			value, err := s.departmentLeaderOptionsWithLiveBatch(ctx, keyword, 1, page, 1, check)
			must(t, err)
			want := 1
			if change == "phone" || strings.HasPrefix(change, "target ") {
				want = 0
			}
			if passes != 2 || value.(map[string]any)["total"] != want || len(leaderOptionItems(t, value)) != 0 {
				t.Fatal("a concurrent change escaped final live filtering and total/page calculation", passes)
			}
		})
	}
}

func TestDepartmentLeaderOptionsRejectsProjectionIdentityMismatch(t *testing.T) {
	for _, field := range []string{"hostUID", "employeeID"} {
		t.Run(field, func(t *testing.T) {
			ctx, s, remote := fixture(t)
			provision(t, ctx, s, remote)
			uid := singleDirectoryUID(t, s, "c")
			p, err := s.Eligible(ctx, uid)
			must(t, err)
			if field == "hostUID" {
				p.HostUID = 999999
			} else {
				p.EmployeeID = "a"
			}
			raw, err := json.Marshal(p)
			must(t, err)
			_, err = s.DB.ExecContext(ctx, `UPDATE biz_openim_employee SET projection_json=? WHERE uid=?`, string(raw), uid)
			must(t, err)
			value, err := s.DepartmentLeaderOptions(ctx, "0003", 1, 1, 20)
			must(t, err)
			if len(leaderOptionItems(t, value)) != 0 || value.(map[string]any)["total"] != 0 {
				t.Fatal("mismatched projection was accepted as a department leader")
			}
		})
	}
}

func TestDepartmentLeaderOptionsQueryFailureIsNotSuccessfulEmptyList(t *testing.T) {
	ctx, s, remote := fixture(t)
	provision(t, ctx, s, remote)
	_, err := s.DB.ExecContext(ctx, `DROP TABLE gf_auth_rule`)
	must(t, err)
	if _, err = s.DepartmentLeaderOptions(ctx, "", 1, 1, 20); err == nil {
		t.Fatal("authorization database failure was swallowed as a successful candidate list")
	}
}
