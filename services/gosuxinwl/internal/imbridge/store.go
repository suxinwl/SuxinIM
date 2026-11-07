package imbridge

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/suxinwl/GoSuxin/internal/accountsecurity"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SQLStore struct {
	DB        *sql.DB
	Prefix    string
	PublicURL string
	mu        sync.Mutex
	Security  *accountsecurity.Store
}

func NewSQLStore(db *sql.DB, prefix string) (*SQLStore, error) {
	p, e := safePrefix(prefix)
	if e != nil {
		return nil, e
	}
	security, e := accountsecurity.New(db, p)
	if e != nil {
		return nil, e
	}
	security.IMEnabled = true
	return &SQLStore{DB: db, Prefix: p, PublicURL: PublicEndpoints().HRM, Security: security}, nil
}
func (s *SQLStore) Ensure(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS biz_openim_state (k VARCHAR(64) PRIMARY KEY, v TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_openim_epoch (uid BIGINT PRIMARY KEY, epoch BIGINT NOT NULL DEFAULT 1)`,
		`CREATE TABLE IF NOT EXISTS biz_openim_employee (employee_id VARCHAR(64) PRIMARY KEY, openim_user_id VARCHAR(64) NOT NULL UNIQUE, uid BIGINT NOT NULL DEFAULT 0, desired_hash VARCHAR(64) NOT NULL, desired_version BIGINT NOT NULL DEFAULT 0, applied_version BIGINT NOT NULL DEFAULT 0, enabled INTEGER NOT NULL DEFAULT 0, provisioned INTEGER NOT NULL DEFAULT 0, status VARCHAR(32) NOT NULL, last_error VARCHAR(500) NOT NULL, projection_json TEXT NOT NULL, updated_at BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_openim_department (department_id VARCHAR(64) PRIMARY KEY, group_id VARCHAR(64) NOT NULL UNIQUE, desired_hash VARCHAR(64) NOT NULL, desired_version BIGINT NOT NULL DEFAULT 0, applied_version BIGINT NOT NULL DEFAULT 0, status VARCHAR(32) NOT NULL, last_error VARCHAR(500) NOT NULL, projection_json TEXT NOT NULL, updated_at BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_openim_outbox (id VARCHAR(100) PRIMARY KEY, kind VARCHAR(20) NOT NULL, aggregate_id VARCHAR(64) NOT NULL, version BIGINT NOT NULL, payload TEXT NOT NULL, status VARCHAR(20) NOT NULL, attempts INTEGER NOT NULL DEFAULT 0, next_attempt BIGINT NOT NULL DEFAULT 0, last_error VARCHAR(500) NOT NULL, created_at BIGINT NOT NULL, updated_at BIGINT NOT NULL)`,
	}
	for _, q := range statements {
		if _, e := s.DB.ExecContext(ctx, q); e != nil {
			return fmt.Errorf("initialize IM schema: %w", e)
		}
	}
	_, e := s.DB.ExecContext(ctx, `INSERT INTO biz_openim_state(k,v) SELECT 'initial_provision','0' WHERE NOT EXISTS (SELECT 1 FROM biz_openim_state WHERE k='initial_provision')`)
	if e != nil {
		return e
	}
	if e = s.ensureAccessSchema(ctx); e != nil {
		return e
	}
	if e = EnsureDepartmentSchema(ctx, s.DB, s.Prefix); e != nil {
		return e
	}
	security, e := accountsecurity.New(s.DB, s.Prefix)
	if e != nil {
		return e
	}
	return security.Ensure(ctx)
}

type sqlQuery interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *SQLStore) Epoch(ctx context.Context, uid int64) (int64, error) { return epoch(ctx, s.DB, uid) }
func epoch(ctx context.Context, q sqlQuery, uid int64) (int64, error) {
	var n int64
	e := q.QueryRowContext(ctx, `SELECT epoch FROM biz_openim_epoch WHERE uid=?`, uid).Scan(&n)
	if e == sql.ErrNoRows {
		_, e = q.ExecContext(ctx, `INSERT INTO biz_openim_epoch(uid,epoch) SELECT ?,1 WHERE NOT EXISTS (SELECT 1 FROM biz_openim_epoch WHERE uid=?)`, uid, uid)
		if e != nil {
			if readErr := q.QueryRowContext(ctx, `SELECT epoch FROM biz_openim_epoch WHERE uid=?`, uid).Scan(&n); readErr == nil {
				return n, nil
			}
			return 0, e
		}
		e = q.QueryRowContext(ctx, `SELECT epoch FROM biz_openim_epoch WHERE uid=?`, uid).Scan(&n)
	}
	return n, e
}
func (s *SQLStore) BumpEpoch(ctx context.Context, uid int64) (int64, error) {
	if _, e := s.Epoch(ctx, uid); e != nil {
		return 0, e
	}
	if _, e := s.DB.ExecContext(ctx, `UPDATE biz_openim_epoch SET epoch=epoch+1 WHERE uid=?`, uid); e != nil {
		return 0, e
	}
	return s.Epoch(ctx, uid)
}

type employee struct {
	ID, No, Name, Status, Position   string
	UID, Dept                        int64
	Username, Avatar, Password, Salt string
	HostStatus                       int
	Deleted                          bool
	DeptName                         string
}

func (s *SQLStore) employees(ctx context.Context, q sqlQuery) ([]employee, error) {
	rows, e := q.QueryContext(ctx, `SELECT e.id,COALESCE(e.employee_no,''),COALESCE(e.name,''),COALESCE(e.status,''),COALESCE(e.position,''),COALESCE(e.user_id,0),COALESCE(e.dept_id,0),COALESCE(a.username,''),COALESCE(a.avatar,''),COALESCE(a.password,''),COALESCE(a.salt,''),COALESCE(a.status,1),CASE WHEN a.deletetime IS NULL THEN 0 ELSE 1 END,COALESCE(d.name,''),CASE WHEN a.id IS NULL THEN 0 ELSE 1 END FROM biz_hrm_employee e LEFT JOIN `+s.Prefix+`admin a ON a.id=e.user_id LEFT JOIN `+s.Prefix+`auth_dept d ON d.id=e.dept_id AND d.status=0 AND d.deletetime IS NULL ORDER BY e.id`)
	if e != nil {
		return nil, fault("source.employees.query", e)
	}
	defer rows.Close()
	out := []employee{}
	for rows.Next() {
		var v employee
		var d, hostExists int
		if e = rows.Scan(&v.ID, &v.No, &v.Name, &v.Status, &v.Position, &v.UID, &v.Dept, &v.Username, &v.Avatar, &v.Password, &v.Salt, &v.HostStatus, &d, &v.DeptName, &hostExists); e != nil {
			return nil, fault("source.employees.scan", e)
		}
		v.Deleted = d != 0 || v.UID > 0 && hostExists == 0
		out = append(out, v)
	}
	if e = rows.Err(); e != nil {
		return nil, fault("source.employees.rows", e)
	}
	return out, nil
}
func parseIDs(raw string) []int64 {
	raw = strings.TrimSpace(raw)
	var v []any
	if json.Unmarshal([]byte(raw), &v) == nil {
		out := []int64{}
		for _, n := range v {
			if id, e := strconv.ParseInt(fmt.Sprint(n), 10, 64); e == nil && id > 0 {
				out = append(out, id)
			}
		}
		return out
	}
	out := []int64{}
	for _, s := range strings.Split(raw, ",") {
		if n, e := strconv.ParseInt(strings.TrimSpace(s), 10, 64); e == nil && n > 0 {
			out = append(out, n)
		}
	}
	return out
}
func (s *SQLStore) hasPermission(ctx context.Context, q sqlQuery, uid int64, permission string) (bool, error) {
	rows, e := q.QueryContext(ctx, `SELECT COALESCE(r.rules,''),COALESCE(r.btns,'') FROM `+s.Prefix+`auth_role r JOIN `+s.Prefix+`auth_role_access x ON x.role_id=r.id WHERE x.uid=? AND r.status=0`, uid)
	if e != nil {
		return false, e
	}
	ids := []int64{}
	super := false
	for rows.Next() {
		var rules, btns string
		if e = rows.Scan(&rules, &btns); e != nil {
			rows.Close()
			return false, e
		}
		if rules == "*" {
			super = true
		}
		ids = append(ids, parseIDs(btns)...)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return false, e
	}
	if super {
		return true, nil
	}
	if len(ids) == 0 {
		return false, nil
	}
	args := []any{permission}
	marks := []string{}
	for _, id := range ids {
		marks = append(marks, "?")
		args = append(args, id)
	}
	var n int
	e = q.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.Prefix+`auth_rule WHERE permission=? AND status=0 AND type=2 AND id IN (`+strings.Join(marks, ",")+`)`, args...).Scan(&n)
	return n > 0, e
}
func (s *SQLStore) Eligible(ctx context.Context, uid int64) (Projection, error) {
	return s.eligible(ctx, s.DB, uid)
}
func (s *SQLStore) eligible(ctx context.Context, q sqlQuery, uid int64) (Projection, error) {
	security := s.Security
	if err := security.CheckActive(ctx, uid); err != nil {
		if errors.Is(err, accountsecurity.ErrDenied) || errors.Is(err, accountsecurity.ErrPasswordChangeRequired) {
			return Projection{}, ErrDenied
		}
		return Projection{}, fault("security.state", err)
	}
	var raw string
	var applied, version int64
	var enabled int
	e := q.QueryRowContext(ctx, `SELECT projection_json,applied_version,desired_version,enabled FROM biz_openim_employee WHERE uid=?`, uid).Scan(&raw, &applied, &version, &enabled)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return Projection{}, fault("eligibility.projection", e)
	}
	if e != nil || enabled != 1 || applied != version {
		return Projection{}, ErrDenied
	}
	var p Projection
	if json.Unmarshal([]byte(raw), &p) != nil {
		return p, ErrDenied
	}
	var status, deleted int
	var pass, salt string
	e = q.QueryRowContext(ctx, `SELECT status,CASE WHEN deletetime IS NULL THEN 0 ELSE 1 END,COALESCE(password,''),COALESCE(salt,'') FROM `+s.Prefix+`admin WHERE id=?`, uid).Scan(&status, &deleted, &pass, &salt)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return p, fault("eligibility.account", e)
	}
	if e != nil || status != 0 || deleted != 0 {
		return p, ErrDenied
	}
	if digest(pass+":"+salt) != p.AuthFingerprint {
		return p, ErrDenied
	}
	var es string
	e = q.QueryRowContext(ctx, `SELECT status FROM biz_hrm_employee WHERE id=? AND user_id=?`, p.EmployeeID, uid).Scan(&es)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return p, fault("eligibility.employee", e)
	}
	if e != nil || es != "active" {
		return p, ErrDenied
	}
	for _, perm := range []string{"runtime.plugin.biz-suite.admin", "biz-suite.im.read", "biz-suite.im.write"} {
		ok, e := s.hasPermission(ctx, q, uid, perm)
		if e != nil {
			return p, fault("eligibility.permission", e)
		}
		if !ok {
			return p, ErrDenied
		}
	}
	return p, nil
}
func (s *SQLStore) ResolveAccount(ctx context.Context, account string) (int64, error) {
	// Search both namespaces and reject cross-namespace collisions.
	rows, e := s.DB.QueryContext(ctx, `SELECT DISTINCT a.id FROM `+s.Prefix+`admin a LEFT JOIN biz_hrm_employee e ON e.user_id=a.id WHERE a.deletetime IS NULL AND (LOWER(a.username)=? OR LOWER(e.employee_no)=?)`, normalize(account), normalize(account))
	if e != nil {
		return 0, e
	}
	defer rows.Close()
	var uid int64
	count := 0
	for rows.Next() {
		if e = rows.Scan(&uid); e != nil {
			return 0, e
		}
		count++
	}
	if count != 1 {
		return 0, ErrDenied
	}
	return uid, rows.Err()
}
func (s *SQLStore) ProfileForUID(ctx context.Context, uid int64) (Projection, error) {
	var raw string
	e := s.DB.QueryRowContext(ctx, `SELECT projection_json FROM biz_openim_employee WHERE uid=?`, uid).Scan(&raw)
	var p Projection
	if e != nil {
		return p, e
	}
	e = json.Unmarshal([]byte(raw), &p)
	return p, e
}
func (s *SQLStore) Conflicts(ctx context.Context, q sqlQuery, employees []employee) (map[string]string, error) {
	usernames := map[string][]int64{}
	rows, e := q.QueryContext(ctx, `SELECT id,COALESCE(username,'') FROM `+s.Prefix+`admin WHERE deletetime IS NULL`)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		var id int64
		var name string
		if e = rows.Scan(&id, &name); e != nil {
			rows.Close()
			return nil, e
		}
		usernames[normalize(name)] = append(usernames[normalize(name)], id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	out := map[string]string{}
	nos := map[string][]string{}
	for _, v := range employees {
		if strings.TrimSpace(v.Name) == "" {
			out[v.ID] = "缺少员工姓名"
		}
		if len([]byte(v.No)) > 64 {
			out[v.ID] = "工号超过通讯账号64字节限制"
		}
		if v.UID == 0 && len([]rune(v.No)) > 20 {
			out[v.ID] = "工号超过宿主账号20字符限制"
			continue
		}
		n := normalize(v.No)
		if n != "" {
			nos[n] = append(nos[n], v.ID)
		}
	}
	for _, v := range employees {
		if strings.TrimSpace(v.No) == "" {
			out[v.ID] = "缺少工号"
			continue
		}
		if len(nos[normalize(v.No)]) > 1 {
			out[v.ID] = "工号冲突"
		}
		for _, id := range usernames[normalize(v.No)] {
			if id != v.UID {
				out[v.ID] = "工号与其他账号冲突"
			}
		}
		if v.Username != "" {
			for _, id := range usernames[normalize(v.Username)] {
				if id != v.UID {
					out[v.ID] = "登录账号重复"
				}
			}
			for _, other := range nos[normalize(v.Username)] {
				if other != v.ID {
					out[v.ID] = "账号与其他员工工号冲突"
				}
			}
		}
	}
	return out, nil
}
func (s *SQLStore) Preview(ctx context.Context) (any, error) {
	v, e := s.employees(ctx, s.DB)
	if e != nil {
		return nil, e
	}
	conf, e := s.Conflicts(ctx, s.DB, v)
	if e != nil {
		return nil, e
	}
	out := []map[string]any{}
	for _, p := range v {
		if p.Status == "active" {
			reason := conf[p.ID]
			if p.Deleted {
				reason = "关联账号已删除"
			}
			out = append(out, map[string]any{"employeeID": p.ID, "employeeNo": p.No, "name": p.Name, "uid": p.UID, "reason": reason, "willEnable": reason == ""})
		}
	}
	var flag string
	e = s.DB.QueryRowContext(ctx, `SELECT v FROM biz_openim_state WHERE k='initial_provision'`).Scan(&flag)
	return map[string]any{"initialProvisioned": flag == "1", "items": out}, e
}

// ConfirmProvision records explicit initial authorization. The regular scanner
// provisions each employee only once, so subsequent manual disables win.
func (s *SQLStore) ConfirmProvision(ctx context.Context) error {
	_, e := s.DB.ExecContext(ctx, `UPDATE biz_openim_state SET v='1' WHERE k='initial_provision'`)
	return e
}
func (s *SQLStore) ensureRole(ctx context.Context, tx *sql.Tx) (int64, error) {
	var stored string
	e := tx.QueryRowContext(ctx, `SELECT v FROM biz_openim_state WHERE k='im_role_id'`).Scan(&stored)
	if e == nil {
		id, e := strconv.ParseInt(stored, 10, 64)
		if e != nil {
			return 0, e
		}
		var status int
		e = tx.QueryRowContext(ctx, `SELECT status FROM `+s.Prefix+`auth_role WHERE id=?`, id).Scan(&status)
		if e != nil || status != 0 {
			if e == sql.ErrNoRows || e == nil {
				return 0, nil
			}
			return 0, e
		}
		return id, nil
	}
	if e != sql.ErrNoRows {
		return 0, e
	}
	ids := []int64{}
	for _, permission := range []string{"runtime.plugin.biz-suite.admin", "biz-suite.im.read", "biz-suite.im.write"} {
		var id int64
		e = tx.QueryRowContext(ctx, `SELECT id FROM `+s.Prefix+`auth_rule WHERE permission=? AND status=0 AND type=2 ORDER BY id LIMIT 1`, permission).Scan(&id)
		if e != nil {
			return 0, fmt.Errorf("missing IM permission: %s", permission)
		}
		ids = append(ids, id)
	}
	btns, _ := json.Marshal(ids)
	menus := []int64{}
	for _, name := range []string{"runtime_plugin_biz-suite", "biz_suite_im"} {
		var id int64
		if e = tx.QueryRowContext(ctx, `SELECT id FROM `+s.Prefix+`auth_rule WHERE routename=? AND status=0 AND type=1 ORDER BY id LIMIT 1`, name).Scan(&id); e != nil {
			return 0, e
		}
		menus = append(menus, id)
	}
	rules := append(append([]int64{}, menus...), ids...)
	seen := map[int64]bool{}
	for i := 0; i < len(rules); i++ {
		id := rules[i]
		if seen[id] {
			continue
		}
		seen[id] = true
		var pid int64
		if e = tx.QueryRowContext(ctx, `SELECT pid FROM `+s.Prefix+`auth_rule WHERE id=?`, id).Scan(&pid); e != nil {
			return 0, e
		}
		if pid > 0 && !seen[pid] {
			rules = append(rules, pid)
		}
	}
	parts := []string{}
	for id := range seen {
		parts = append(parts, intString(id))
	}
	sort.Strings(parts)
	menu, _ := json.Marshal(menus)
	result, e := tx.ExecContext(ctx, `INSERT INTO `+s.Prefix+`auth_role(business_id,account_id,pid,name,rules,menu,btns,status,data_access,remark,weigh,createtime) VALUES(0,0,0,?,?,?,?,0,0,?,0,?)`, "内网通讯专用", strings.Join(parts, ","), string(menu), string(btns), "仅包含通讯访问权限，不授予HRM管理权限", time.Now())
	if e != nil {
		return 0, e
	}
	id, e := result.LastInsertId()
	if e != nil {
		return 0, e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO biz_openim_state(k,v) VALUES('im_role_id',?)`, intString(id))
	return id, e
}
func (s *SQLStore) provision(ctx context.Context, tx *sql.Tx, v *employee, role int64) error {
	if v.UID == 0 {
		if len([]rune(v.No)) > 20 {
			return errors.New("工号超过宿主账号20字符限制")
		}
		password, e := token()
		if e != nil {
			return e
		}
		salt, e := token()
		if e != nil {
			return e
		}
		sum := md5.Sum([]byte(password + salt[:16]))
		r, e := tx.ExecContext(ctx, `INSERT INTO `+s.Prefix+`admin(account_id,dept_id,username,password,salt,name,nickname,avatar,email,mobile,tel,status,remark,createtime,updatetime) VALUES(0,?,?,?,?,?,?,?,?,?,?,0,?,?,?)`, v.Dept, v.No, hex.EncodeToString(sum[:]), salt[:16], v.Name, v.Name, "/resource/static/brand/default-avatar.png?v=puty2", "", "", "", "HRM通讯开通：请管理员设置初始密码", time.Now(), time.Now())
		if e != nil {
			return e
		}
		v.UID, e = r.LastInsertId()
		if e != nil {
			return e
		}
		v.Username = v.No
		v.Password = hex.EncodeToString(sum[:])
		v.Salt = salt[:16]
		if _, e = tx.ExecContext(ctx, `UPDATE biz_hrm_employee SET user_id=? WHERE id=? AND (user_id IS NULL OR user_id=0)`, v.UID, v.ID); e != nil {
			return e
		}
	}
	if _, e := tx.ExecContext(ctx, `UPDATE `+s.Prefix+`admin SET status=0 WHERE id=? AND deletetime IS NULL`, v.UID); e != nil {
		return e
	}
	v.HostStatus = 0
	var n int
	if e := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.Prefix+`auth_role_access WHERE uid=? AND role_id=?`, v.UID, role).Scan(&n); e != nil {
		return e
	}
	if n == 0 {
		_, e := tx.ExecContext(ctx, `INSERT INTO `+s.Prefix+`auth_role_access(uid,role_id) VALUES(?,?)`, v.UID, role)
		return e
	}
	return nil
}
