package imbridge

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"

	mysql "github.com/go-sql-driver/mysql"
)

// EnsureDepartmentSchema upgrades existing host departments without opting any
// department into automatic group management.
func EnsureDepartmentSchema(ctx context.Context, db *sql.DB, prefix string) error {
	prefix, err := safePrefix(prefix)
	if err != nil {
		return err
	}
	columns := func() (map[string]bool, error) {
		rows, e := db.QueryContext(ctx, `SELECT * FROM `+prefix+`auth_dept LIMIT 0`)
		if e != nil {
			var m *mysql.MySQLError
			if errors.As(e, &m) && m.Number == 1146 || strings.Contains(e.Error(), "no such table") {
				return nil, nil
			}
			return nil, e
		}
		defer rows.Close()
		names, e := rows.Columns()
		if e != nil {
			return nil, e
		}
		out := map[string]bool{}
		for _, name := range names {
			out[name] = true
		}
		return out, nil
	}
	found, err := columns()
	if err != nil || found == nil {
		return err
	}
	for _, change := range []struct{ name, definition string }{{"im_group_enabled", "INTEGER NOT NULL DEFAULT 0"}, {"leader_uid", "BIGINT NOT NULL DEFAULT 0"}} {
		if found[change.name] {
			continue
		}
		if _, err = db.ExecContext(ctx, `ALTER TABLE `+prefix+`auth_dept ADD COLUMN `+change.name+` `+change.definition); err != nil {
			current, readErr := columns()
			if readErr != nil || !current[change.name] {
				return err
			}
		}
	}
	return nil
}

func (s *SQLStore) organizationScope(ctx context.Context, q sqlQuery, p Projection) (int64, error) {
	var scope int64
	// A disabled department retains its namespace; hiding it in the tree must
	// never move its employees into the instance-wide (business 0) namespace.
	err := q.QueryRowContext(ctx, `SELECT COALESCE(d.business_id,0) FROM biz_hrm_employee e LEFT JOIN `+s.Prefix+`auth_dept d ON d.id=e.dept_id WHERE e.id=? AND e.user_id=? AND e.status='active'`, p.EmployeeID, p.HostUID).Scan(&scope)
	return scope, err
}

func (s *SQLStore) departmentLeader(ctx context.Context, q sqlQuery, id, leaderUID int64) (Projection, error) {
	if leaderUID <= 0 {
		return Projection{}, errors.New("请先选择已开通通讯的部门负责人")
	}
	p, err := s.eligible(ctx, q, leaderUID)
	if err != nil {
		return p, errors.New("部门负责人必须是在职、账号启用且已开通通讯的员工")
	}
	{
		var business int64
		if id > 0 {
			err = q.QueryRowContext(ctx, `SELECT COALESCE(business_id,0) FROM `+s.Prefix+`auth_dept WHERE id=? AND status=0 AND deletetime IS NULL`, id).Scan(&business)
			if err != nil {
				return p, errors.New("停用或删除的部门不能开启部门群")
			}
		}
		scope, e := s.organizationScope(ctx, q, p)
		if e != nil || scope != business {
			return p, errors.New("部门负责人必须属于同一业务组织")
		}
	}
	return p, nil
}

func (s *SQLStore) ValidateDepartmentGroup(ctx context.Context, id, leaderUID int64) error {
	_, err := s.departmentLeader(ctx, s.DB, id, leaderUID)
	return err
}

func (s *SQLStore) DepartmentLeaderOptions(ctx context.Context, keyword string, departmentID int64, page, size int) (any, error) {
	return s.departmentLeaderOptionsWithLiveBatch(ctx, keyword, departmentID, page, size, s.directoryLiveBatch)
}

func (s *SQLStore) departmentLeaderOptionsWithLiveBatch(ctx context.Context, keyword string, departmentID int64, page, size int, check directoryLiveCheck) (any, error) {
	if page < 1 {
		page = 1
	}
	if page > 1000000 {
		page = 1000000
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	items := []map[string]any{}
	result := func() map[string]any {
		total := len(items)
		start := min((page-1)*size, total)
		end := min(start+size, total)
		return map[string]any{"items": items[start:end], "total": total, "page": page, "pageSize": size}
	}
	readScope := func() (int64, bool, error) {
		var business int64
		if departmentID <= 0 {
			return 0, true, nil
		}
		err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(business_id,0) FROM `+s.Prefix+`auth_dept WHERE id=? AND status=0 AND deletetime IS NULL`, departmentID).Scan(&business)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return business, err == nil, err
	}
	business, active, err := readScope()
	if err != nil {
		return nil, err
	}
	if !active {
		return result(), nil
	}
	// Preselect the business namespace once. A ready projection alone never
	// authorizes a candidate: both live passes below verify the current account,
	// credentials, HRM status, security state, role grants and business scope.
	rows, err := s.DB.QueryContext(ctx, `SELECT b.uid FROM biz_openim_employee b JOIN biz_hrm_employee e ON e.id=b.employee_id AND e.user_id=b.uid LEFT JOIN `+s.Prefix+`auth_dept d ON d.id=e.dept_id WHERE b.enabled=1 AND b.status='ready' AND b.applied_version=b.desired_version AND e.status='active' AND COALESCE(d.business_id,0)=? ORDER BY b.employee_id`, business)
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	seen := map[int64]bool{}
	for rows.Next() {
		var uid int64
		if err = rows.Scan(&uid); err != nil {
			rows.Close()
			return nil, err
		}
		if uid > 0 && !seen[uid] {
			ids = append(ids, uid)
			seen[uid] = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	first, err := check(ctx, ids)
	if err != nil {
		return nil, err
	}
	matches := func(v directoryLiveProfile) bool {
		return v.business == business && matchesDirectory(v.p, keyword, v.mobile, v.hostMobile)
	}
	matched := []int64{}
	for _, uid := range ids {
		if v, ok := first[uid]; ok && matches(v) {
			matched = append(matched, uid)
		}
	}
	// Filter again before counting and slicing, rather than running one
	// eligibility query chain per employee or applying SQL LIMIT before RBAC.
	last, err := check(ctx, matched)
	if err != nil {
		return nil, err
	}
	currentBusiness, active, err := readScope()
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if !active || currentBusiness != business {
		return result(), nil
	}
	for _, uid := range matched {
		if v, ok := last[uid]; ok && matches(v) {
			p := v.p
			items = append(items, map[string]any{"id": uid, "name": p.Nickname, "employee_no": p.Account, "dept_id": p.DepartmentID, "department": p.DepartmentName})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		left, right := items[i]["employee_no"].(string), items[j]["employee_no"].(string)
		if left == right {
			return items[i]["id"].(int64) < items[j]["id"].(int64)
		}
		return left < right
	})
	return result(), nil
}

var departmentChanges = make(chan struct{}, 1)
var departmentMutation sync.RWMutex

func DepartmentMutationLock() func() { departmentMutation.Lock(); return departmentMutation.Unlock }

func NotifyDepartmentsChanged() {
	select {
	case departmentChanges <- struct{}{}:
	default:
	}
}

func (s *SQLStore) departmentDispatchAllowed(ctx context.Context, d Department) bool {
	if !d.GroupEnabled || d.LeaderUserID == "" {
		return false
	}
	id, err := strconv.ParseInt(d.DepartmentID, 10, 64)
	if err != nil || id <= 0 {
		return false
	}
	var enabled, status int
	var leader int64
	var deleted int
	err = s.DB.QueryRowContext(ctx, `SELECT im_group_enabled,leader_uid,status,CASE WHEN deletetime IS NULL THEN 0 ELSE 1 END FROM `+s.Prefix+`auth_dept WHERE id=?`, id).Scan(&enabled, &leader, &status, &deleted)
	if err != nil || enabled != 1 || status != 0 || deleted != 0 {
		return false
	}
	var currentVersion int64
	if err = s.DB.QueryRowContext(ctx, `SELECT desired_version FROM biz_openim_department WHERE department_id=?`, d.DepartmentID).Scan(&currentVersion); err != nil || d.Version <= 0 || currentVersion != d.Version {
		return false
	}
	p, err := s.departmentLeader(ctx, s.DB, id, leader)
	return err == nil && p.UserID == d.LeaderUserID
}
