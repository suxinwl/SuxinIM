package imbridge

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

var directoryDriverID atomic.Int64

type directoryCountDriver struct {
	driver.Driver
	queries *atomic.Int64
}

func (d directoryCountDriver) Open(name string) (driver.Conn, error) {
	c, err := d.Driver.Open(name)
	if err != nil {
		return nil, err
	}
	return directoryCountConn{Conn: c, queries: d.queries}, nil
}

type directoryCountConn struct {
	driver.Conn
	queries *atomic.Int64
}

func (c directoryCountConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(query)), "SELECT") {
		c.queries.Add(1)
	}
	if q, ok := c.Conn.(driver.QueryerContext); ok {
		return q.QueryContext(ctx, query, args)
	}
	return nil, driver.ErrSkip
}

func countedDirectoryDB(t *testing.T, s *SQLStore) *atomic.Int64 {
	t.Helper()
	var number int
	var name, filename string
	must(t, s.DB.QueryRow(`PRAGMA database_list`).Scan(&number, &name, &filename))
	count := &atomic.Int64{}
	driverName := fmt.Sprintf("directory-count-%d", directoryDriverID.Add(1))
	sql.Register(driverName, directoryCountDriver{Driver: s.DB.Driver(), queries: count})
	db, err := sql.Open(driverName, filename)
	must(t, err)
	t.Cleanup(func() { _ = db.Close() })
	s.DB = db
	return count
}

func TestDirectoryUnfilteredLiveAuthorizationUsesBoundedSQL(t *testing.T) {
	ctx, s, remote := fixture(t)
	for i := 0; i < 60; i++ {
		_, err := s.DB.ExecContext(ctx, `INSERT INTO biz_hrm_employee(id,employee_no,dept_id,name,status,position) VALUES(?,?,2,?,'active','staff')`, fmt.Sprintf("batch-%03d", i), fmt.Sprintf("BATCH-%03d", i), fmt.Sprintf("Synthetic %03d", i))
		must(t, err)
	}
	provision(t, ctx, s, remote)
	// All five sources can change while a ready projection still exists. The
	// first page and its total must use the current account/HRM/RBAC/scope state.
	_, err := s.DB.ExecContext(ctx, `UPDATE gf_admin SET status=1 WHERE id=(SELECT user_id FROM biz_hrm_employee WHERE id='batch-000')`)
	must(t, err)
	_, err = s.DB.ExecContext(ctx, `UPDATE gf_admin SET password='changed' WHERE id=(SELECT user_id FROM biz_hrm_employee WHERE id='batch-001')`)
	must(t, err)
	_, err = s.DB.ExecContext(ctx, `DELETE FROM gf_auth_role_access WHERE uid=(SELECT user_id FROM biz_hrm_employee WHERE id='batch-002')`)
	must(t, err)
	_, err = s.DB.ExecContext(ctx, `UPDATE biz_hrm_employee SET status='inactive' WHERE id='batch-003'`)
	must(t, err)
	_, err = s.DB.ExecContext(ctx, `INSERT INTO gf_auth_dept(id,name,status,business_id) VALUES(9,'Other scope',0,9)`)
	must(t, err)
	_, err = s.DB.ExecContext(ctx, `UPDATE biz_hrm_employee SET dept_id=9 WHERE id='batch-004'`)
	must(t, err)
	count := countedDirectoryDB(t, s)
	value, err := s.DirectoryForUID(ctx, 1, "", "", 2, 7)
	must(t, err)
	if value.(map[string]any)["total"].(int) != 57 || len(directoryItems(t, value)) != 7 {
		t.Fatal("unfiltered total/page included an ineligible or other-scope employee")
	}
	if n := count.Load(); n > 24 {
		t.Fatalf("directory authorization still performs per-employee SQL: %d SELECTs for 62 ready employees", n)
	} else {
		t.Logf("62 ready employees, five current exclusions, total 57: %d SELECTs", n)
	}
}

func TestDirectoryBatchRechecksLiveChangesBeforeTotalAndPage(t *testing.T) {
	for _, change := range []string{"disabled", "password", "role", "employee", "scope", "phone", "projection", "viewer"} {
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
			viewer, err := s.Eligible(ctx, 1)
			must(t, err)
			passes := 0
			check := func(ctx context.Context, uids []int64) (map[int64]directoryLiveProfile, error) {
				live, err := s.directoryLiveBatch(ctx, uids)
				if err != nil {
					return nil, err
				}
				passes++
				if passes == 1 {
					query := map[string]string{
						"disabled":   `UPDATE gf_admin SET status=1 WHERE id=?`,
						"password":   `UPDATE gf_admin SET password='changed' WHERE id=?`,
						"role":       `UPDATE gf_auth_role SET status=1 WHERE id=900 AND ? > 0`,
						"employee":   `UPDATE biz_hrm_employee SET status='inactive' WHERE user_id=?`,
						"scope":      `UPDATE gf_auth_dept SET business_id=9 WHERE id=2 AND ? > 0`,
						"phone":      `UPDATE biz_hrm_employee SET mobile='13900139000' WHERE user_id=?`,
						"projection": `UPDATE biz_openim_employee SET desired_version=desired_version+1 WHERE uid=?`,
						"viewer":     `UPDATE gf_admin SET status=1 WHERE id=1 AND ? > 0`,
					}[change]
					_, err = s.DB.ExecContext(ctx, query, uid)
					must(t, err)
				}
				return live, nil
			}
			keyword, page, size := "", 2, 1
			if change == "phone" {
				keyword, page = "13800138000", 1
			}
			value, err := s.directoryWithLiveBatch(ctx, &viewer, keyword, "", page, size, check)
			if change == "viewer" {
				if err == nil {
					t.Fatal("viewer disabled during the request retained directory access")
				}
				return
			}
			must(t, err)
			want := 1
			if change == "phone" {
				want = 0
			}
			if passes != 2 || value.(map[string]any)["total"].(int) != want || len(directoryItems(t, value)) != 0 {
				t.Fatal("a change between live passes escaped final total/page filtering", passes)
			}
		})
	}
}

func TestDirectoryBatchMatchesEligibleRoleAndRuleSemantics(t *testing.T) {
	ctx, s, remote := fixture(t)
	provision(t, ctx, s, remote)
	uid := singleDirectoryUID(t, s, "c")
	_, err := s.DB.ExecContext(ctx, `INSERT INTO gf_auth_role(id,name,rules,btns,status) VALUES(900,'isolated role','','3,4,5',0)`)
	must(t, err)
	_, err = s.DB.ExecContext(ctx, `DELETE FROM gf_auth_role_access WHERE uid=?`, uid)
	must(t, err)
	_, err = s.DB.ExecContext(ctx, `INSERT INTO gf_auth_role_access(uid,role_id) VALUES(?,900)`, uid)
	must(t, err)
	for _, state := range []struct {
		rules, buttons         string
		roleStatus, ruleStatus int
	}{
		{"", "3,4,5", 0, 0}, {"", "[3,4,5]", 0, 0}, {"", "3,4", 0, 0},
		{"", "3,4,5", 1, 0}, {"", "3,4,5", 0, 1}, {"*", "", 0, 1}, {" * ", "", 0, 0},
	} {
		_, err = s.DB.ExecContext(ctx, `UPDATE gf_auth_role SET rules=?,btns=?,status=? WHERE id=900`, state.rules, state.buttons, state.roleStatus)
		must(t, err)
		_, err = s.DB.ExecContext(ctx, `UPDATE gf_auth_rule SET status=? WHERE id=5`, state.ruleStatus)
		must(t, err)
		_, eligibleErr := s.Eligible(ctx, uid)
		live, err := s.directoryLiveBatch(ctx, []int64{uid})
		must(t, err)
		_, batchAllowed := live[uid]
		if batchAllowed != (eligibleErr == nil) {
			t.Fatal("batch changed existing role/button/rule authorization semantics")
		}
	}
}
