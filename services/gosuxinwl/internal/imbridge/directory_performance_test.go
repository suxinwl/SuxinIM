package imbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestDirectoryPrefiltersUnrelatedEmployeesAndCountsLiveMatches(t *testing.T) {
	ctx, s, r := fixture(t)
	_, err := s.DB.ExecContext(ctx, `UPDATE biz_hrm_employee SET name='匹配员工甲' WHERE id='c'`)
	must(t, err)
	for i := 0; i < 42; i++ {
		id, name := fmt.Sprintf("search-%02d", i), "不相关人员"
		if i < 2 {
			name = "匹配员工"
		}
		_, err = s.DB.ExecContext(ctx, `INSERT INTO biz_hrm_employee(id,employee_no,dept_id,name,status,position) VALUES(?,?,2,?,'active','职员')`, id, fmt.Sprintf("SEARCH-%02d", i), name)
		must(t, err)
	}
	provision(t, ctx, s, r)
	var ready int
	must(t, s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM biz_openim_employee WHERE enabled=1 AND status='ready' AND applied_version=desired_version`).Scan(&ready))
	if ready != 44 {
		t.Fatal("expected 44 ready candidates before search", ready)
	}
	var disabledUID int64
	must(t, s.DB.QueryRowContext(ctx, `SELECT user_id FROM biz_hrm_employee WHERE id='search-01'`).Scan(&disabledUID))
	_, err = s.DB.ExecContext(ctx, `UPDATE gf_admin SET status=1 WHERE id=?`, disabledUID)
	must(t, err)
	viewer, err := s.Eligible(ctx, 1)
	must(t, err)
	calls := map[int64]int{}
	check := func(ctx context.Context, uid int64) (Projection, error) {
		calls[uid]++
		return s.Eligible(ctx, uid)
	}
	value, err := s.directoryWithEligibility(ctx, &viewer, "匹配员工", "2", 2, 1, check)
	must(t, err)
	if value.(map[string]any)["total"].(int) != 2 || len(directoryItems(t, value)) != 1 {
		t.Fatal("pagination total counted the stale ready but disabled candidate")
	}
	if len(calls) != 3 || calls[disabledUID] != 1 {
		t.Fatal("unrelated employees were repeatedly authorized", len(calls))
	}
	for _, count := range calls {
		if count > 2 {
			t.Fatal("candidate eligibility exceeded the two request-local live passes")
		}
	}
	calls = map[int64]int{}
	value, err = s.directoryWithEligibility(ctx, &viewer, "0003", "2", 1, 100, check)
	must(t, err)
	if len(calls) != 1 || calls[singleDirectoryUID(t, s, "c")] != 2 || value.(map[string]any)["total"].(int) != 1 {
		t.Fatal("exact search did not authorize only the one matching candidate")
	}
	calls = map[int64]int{}
	value, err = s.directoryWithEligibility(ctx, &viewer, "0003", "1", 1, 100, check)
	must(t, err)
	if len(calls) != 0 || value.(map[string]any)["total"].(int) != 0 {
		t.Fatal("department AND keyword prefilter was bypassed")
	}
}

func singleDirectoryUID(t *testing.T, s *SQLStore, employeeID string) int64 {
	t.Helper()
	var uid int64
	must(t, s.DB.QueryRow(`SELECT user_id FROM biz_hrm_employee WHERE id=?`, employeeID).Scan(&uid))
	return uid
}

func TestDirectoryRechecksPhoneDepartmentAndScopeAfterPrefilter(t *testing.T) {
	for _, change := range []string{"phone", "department", "scope"} {
		t.Run(change, func(t *testing.T) {
			ctx, s, r := fixture(t)
			_, err := s.DB.ExecContext(ctx, `UPDATE biz_hrm_employee SET mobile='13800138000' WHERE id='c'`)
			must(t, err)
			provision(t, ctx, s, r)
			viewer, err := s.Eligible(ctx, 1)
			must(t, err)
			calls := 0
			check := func(ctx context.Context, uid int64) (Projection, error) {
				calls++
				switch change {
				case "phone":
					_, err = s.DB.ExecContext(ctx, `UPDATE biz_hrm_employee SET mobile='13900139000' WHERE id='c'`)
				case "department":
					p, e := s.ProfileForUID(ctx, uid)
					must(t, e)
					p.DepartmentID = "1"
					raw, e := json.Marshal(p)
					must(t, e)
					_, err = s.DB.ExecContext(ctx, `UPDATE biz_openim_employee SET projection_json=? WHERE uid=?`, string(raw), uid)
				case "scope":
					_, err = s.DB.ExecContext(ctx, `UPDATE gf_auth_dept SET business_id=9 WHERE id=2`)
				}
				must(t, err)
				return s.Eligible(ctx, uid)
			}
			value, err := s.directoryWithEligibility(ctx, &viewer, "13800138000", "2", 1, 100, check)
			must(t, err)
			if calls != 1 || value.(map[string]any)["total"].(int) != 0 {
				t.Fatal("changed candidate escaped final live conditions")
			}
		})
	}
}
