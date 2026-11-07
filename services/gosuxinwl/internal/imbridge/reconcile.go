package imbridge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Reconcile takes a consistent current-state projection. It does not claim to
// run atomically with the separate HRM/WeCom write transactions.
func (s *SQLStore) Reconcile(ctx context.Context) (returnErr error) {
	defer func() {
		if returnErr != nil {
			var op *operationFault
			if !errors.As(returnErr, &op) {
				returnErr = fault("reconcile", returnErr)
			}
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var authorization string
	if e = tx.QueryRowContext(ctx, `SELECT v FROM biz_openim_state WHERE k='initial_provision'`).Scan(&authorization); e != nil {
		return e
	}
	employees, e := s.employees(ctx, tx)
	if e != nil {
		return e
	}
	conflicts, e := s.Conflicts(ctx, tx, employees)
	if e != nil {
		return e
	}
	var role int64
	if authorization == "1" {
		role, e = s.ensureRole(ctx, tx)
		if e != nil {
			return e
		}
	}
	seen := map[string]bool{}
	departments := map[string]Department{}
	for _, v := range employees {
		seen[v.ID] = true
		var oldRaw, oldHash string
		var version, applied int64
		var provisioned int
		e = tx.QueryRowContext(ctx, `SELECT projection_json,desired_hash,desired_version,applied_version,provisioned FROM biz_openim_employee WHERE employee_id=?`, v.ID).Scan(&oldRaw, &oldHash, &version, &applied, &provisioned)
		exists := e == nil
		if e != nil && e != sql.ErrNoRows {
			return e
		}
		var old Projection
		if oldRaw != "" {
			if e = json.Unmarshal([]byte(oldRaw), &old); e != nil {
				return e
			}
		}
		reason := conflicts[v.ID]
		if v.Status != "active" {
			reason = "员工非在职状态"
		}
		if v.Deleted {
			reason = "宿主账号已删除"
		}
		if authorization != "1" && reason == "" {
			reason = "等待首次开通确认"
		}
		if authorization == "1" && reason == "" && provisioned == 0 {
			if role == 0 {
				reason = "通讯专用角色被删除或停用，等待管理员处理"
			} else {
				if e = s.provision(ctx, tx, &v, role); e != nil {
					return fmt.Errorf("provision employee %s: %w", v.ID, e)
				}
				provisioned = 1
			}
		}
		enabled := reason == "" && v.UID > 0 && v.HostStatus == 0 && !v.Deleted
		if reason == "" && !enabled {
			reason = "宿主账号已停用"
		}
		if enabled {
			for _, perm := range []string{"runtime.plugin.biz-suite.admin", "biz-suite.im.read", "biz-suite.im.write"} {
				ok, e := s.hasPermission(ctx, tx, v.UID, perm)
				if e != nil {
					return e
				}
				if !ok {
					enabled = false
					reason = "通讯权限已收回"
					break
				}
			}
		}
		p := Projection{UserID: UserID(v.ID), Account: v.No, Nickname: v.Name, FaceURL: v.Avatar, DepartmentID: intString(v.Dept), DepartmentName: v.DeptName, EmployeeID: v.ID, HostUID: v.UID, Enabled: enabled, Reason: reason, AuthFingerprint: digest(v.Password + ":" + v.Salt), Profile: Profile{EmployeeID: v.ID, Account: v.No, Name: v.Name, FaceURL: v.Avatar, DepartmentID: intString(v.Dept), DepartmentName: v.DeptName, Position: v.Position}}
		p.FaceURL = faceURL(s.PublicURL, v.Avatar)
		p.Profile.FaceURL = p.FaceURL
		if !enabled && applied > 0 {
			if p.Account == "" {
				p.Account = old.Account
			}
			if p.Nickname == "" {
				p.Nickname = old.Nickname
			}
		}
		hash := fingerprint(p)
		if hash == oldHash && (enabled || applied > 0) {
			_, e = tx.ExecContext(ctx, `UPDATE biz_openim_outbox SET status='pending',next_attempt=0 WHERE id=? AND status='done'`, fmt.Sprintf("user:%s:%d", v.ID, version))
			if e != nil {
				return e
			}
		}
		if hash != oldHash {
			version++
			p.Version = version
			raw, _ := json.Marshal(p)
			state := "pending"
			if !enabled && applied == 0 {
				state = "waiting"
			}
			if exists {
				_, e = tx.ExecContext(ctx, `UPDATE biz_openim_employee SET uid=?,desired_hash=?,desired_version=?,enabled=?,provisioned=?,status=?,last_error='',projection_json=?,updated_at=? WHERE employee_id=?`, v.UID, hash, version, boolInt(enabled), provisioned, state, string(raw), time.Now().Unix(), v.ID)
			} else {
				_, e = tx.ExecContext(ctx, `INSERT INTO biz_openim_employee(employee_id,openim_user_id,uid,desired_hash,desired_version,applied_version,enabled,provisioned,status,last_error,projection_json,updated_at) VALUES(?,?,?,?,?,0,?,?,?,'',?,?)`, v.ID, p.UserID, v.UID, hash, version, boolInt(enabled), provisioned, state, string(raw), time.Now().Unix())
			}
			if e != nil {
				return e
			}
			if exists && v.UID > 0 && ((old.Enabled && !enabled) || old.AuthFingerprint != p.AuthFingerprint) {
				epochValue, e := epoch(ctx, tx, v.UID)
				if e != nil {
					return e
				}
				epochValue++
				if _, e = tx.ExecContext(ctx, `UPDATE biz_openim_epoch SET epoch=? WHERE uid=?`, epochValue, v.UID); e != nil {
					return e
				}
				if e = s.enqueue(ctx, tx, "revoke", v.ID, version, Revoke{UserID: p.UserID, All: true, Epoch: epochValue}); e != nil {
					return e
				}
			}
			// Pending employees with no previously-created IM identity stay local.
			if enabled || applied > 0 {
				if e = s.enqueue(ctx, tx, "user", v.ID, version, p); e != nil {
					return e
				}
			}
		}
		if enabled && v.Dept > 0 && v.DeptName != "" {
			key := intString(v.Dept)
			d := departments[key]
			d.DepartmentID = key
			d.Name = v.DeptName
			d.GroupID = GroupID(key)
			d.MemberUserIDs = append(d.MemberUserIDs, p.UserID)
			departments[key] = d
		}
	}
	// A truly deleted employee record still revokes its existing stable identity.
	rows, e := tx.QueryContext(ctx, `SELECT employee_id,projection_json,desired_version,applied_version FROM biz_openim_employee WHERE enabled=1`)
	if e != nil {
		return e
	}
	type missing struct {
		id, raw          string
		version, applied int64
	}
	missingRows := []missing{}
	for rows.Next() {
		var v missing
		if e = rows.Scan(&v.id, &v.raw, &v.version, &v.applied); e != nil {
			rows.Close()
			return e
		}
		if !seen[v.id] {
			missingRows = append(missingRows, v)
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range missingRows {
		var p Projection
		if e = json.Unmarshal([]byte(v.raw), &p); e != nil {
			return e
		}
		p.Enabled = false
		p.Reason = "员工记录已删除"
		p.Version = v.version + 1
		raw, _ := json.Marshal(p)
		if _, e = tx.ExecContext(ctx, `UPDATE biz_openim_employee SET enabled=0,desired_hash=?,desired_version=?,projection_json=?,status='pending',updated_at=? WHERE employee_id=?`, fingerprint(p), p.Version, string(raw), time.Now().Unix(), v.id); e != nil {
			return e
		}
		ep, e := epoch(ctx, tx, p.HostUID)
		if e != nil {
			return e
		}
		ep++
		if _, e = tx.ExecContext(ctx, `UPDATE biz_openim_epoch SET epoch=? WHERE uid=?`, ep, p.HostUID); e != nil {
			return e
		}
		if e = s.enqueue(ctx, tx, "revoke", v.id, p.Version, Revoke{UserID: p.UserID, All: true, Epoch: ep}); e != nil {
			return e
		}
		if e = s.enqueue(ctx, tx, "user", v.id, p.Version, p); e != nil {
			return e
		}
	}
	if e = s.reconcileDepartments(ctx, tx, departments); e != nil {
		return e
	}
	return tx.Commit()
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func (s *SQLStore) enqueue(ctx context.Context, tx *sql.Tx, kind, id string, version int64, payload any) error {
	b, e := json.Marshal(payload)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE biz_openim_outbox SET status='superseded',updated_at=? WHERE kind=? AND aggregate_id=? AND status IN ('pending','failed')`, time.Now().Unix(), kind, id)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO biz_openim_outbox(id,kind,aggregate_id,version,payload,status,attempts,next_attempt,last_error,created_at,updated_at) VALUES(?,?,?,?,?,'pending',0,0,'',?,?)`, fmt.Sprintf("%s:%s:%d", kind, id, version), kind, id, version, string(b), time.Now().Unix(), time.Now().Unix())
	return e
}
func (s *SQLStore) reconcileDepartments(ctx context.Context, tx *sql.Tx, current map[string]Department) error {
	// Each department is an explicit opt-in. The legacy global setting cannot
	// opt in other departments or suppress a department that was enabled here.
	rows, e := tx.QueryContext(ctx, `SELECT id,name,leader_uid FROM `+s.Prefix+`auth_dept WHERE im_group_enabled=1 AND status=0 AND deletetime IS NULL`)
	if e != nil {
		return e
	}
	type control struct {
		id     int64
		name   string
		leader int64
	}
	controls := []control{}
	for rows.Next() {
		var c control
		if e = rows.Scan(&c.id, &c.name, &c.leader); e != nil {
			rows.Close()
			return e
		}
		controls = append(controls, c)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	active := map[string]Department{}
	for _, c := range controls {
		id := intString(c.id)
		leader, err := s.departmentLeader(ctx, tx, c.id, c.leader)
		if err != nil {
			if _, e = tx.ExecContext(ctx, `UPDATE biz_openim_department SET status='blocked',last_error='部门负责人不可用，请重新选择已开通通讯员工' WHERE department_id=?`, id); e != nil {
				return e
			}
			continue
		}
		d := current[id]
		d.DepartmentID = id
		d.GroupID = GroupID(id)
		d.Name = c.name
		d.GroupEnabled = true
		d.LeaderUserID = leader.UserID
		d.Frozen = false
		found := false
		for _, member := range d.MemberUserIDs {
			if member == leader.UserID {
				found = true
			}
		}
		if !found {
			d.MemberUserIDs = append(d.MemberUserIDs, leader.UserID)
		}
		active[id] = d
	}
	// Pause old tasks only; existing groups, membership and history stay intact.
	rows, e = tx.QueryContext(ctx, `SELECT department_id FROM biz_openim_department`)
	if e != nil {
		return e
	}
	oldIDs := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		oldIDs = append(oldIDs, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range oldIDs {
		if _, ok := active[id]; !ok {
			if _, e = tx.ExecContext(ctx, `UPDATE biz_openim_outbox SET status='superseded',updated_at=? WHERE kind='department' AND aggregate_id=? AND status IN ('pending','failed')`, time.Now().Unix(), id); e != nil {
				return e
			}
			if _, e = tx.ExecContext(ctx, `UPDATE biz_openim_department SET status=CASE WHEN status='blocked' THEN 'blocked' ELSE 'paused' END WHERE department_id=?`, id); e != nil {
				return e
			}
		}
	}
	for id, d := range active {
		sort.Strings(d.MemberUserIDs)
		d.Version = 0
		b, _ := json.Marshal(d)
		hash := digest(string(b))
		var oldHash string
		var version int64
		e = tx.QueryRowContext(ctx, `SELECT desired_hash,desired_version FROM biz_openim_department WHERE department_id=?`, id).Scan(&oldHash, &version)
		exists := e == nil
		if e != nil && e != sql.ErrNoRows {
			return e
		}
		if oldHash == hash {
			if _, e = tx.ExecContext(ctx, `UPDATE biz_openim_outbox SET status='pending',next_attempt=0 WHERE id=? AND status IN ('done','superseded')`, fmt.Sprintf("department:%s:%d", id, version)); e != nil {
				return e
			}
			continue
		}
		version++
		d.Version = version
		b, _ = json.Marshal(d)
		if exists {
			_, e = tx.ExecContext(ctx, `UPDATE biz_openim_department SET desired_hash=?,desired_version=?,projection_json=?,status='pending',last_error='',updated_at=? WHERE department_id=?`, hash, version, string(b), time.Now().Unix(), id)
		} else {
			_, e = tx.ExecContext(ctx, `INSERT INTO biz_openim_department(department_id,group_id,desired_hash,desired_version,applied_version,status,last_error,projection_json,updated_at) VALUES(?,?,?,?,0,'pending','',?,?)`, id, d.GroupID, hash, version, string(b), time.Now().Unix())
		}
		if e != nil {
			return e
		}
		if e = s.enqueue(ctx, tx, "department", id, version, d); e != nil {
			return e
		}
	}
	return nil
}
func (s *SQLStore) Drain(ctx context.Context, remote Remote) (returnErr error) {
	return s.drain(ctx, remote, 15*time.Second)
}

func (s *SQLStore) drain(ctx context.Context, remote Remote, budget time.Duration) (returnErr error) {
	defer func() {
		if returnErr != nil {
			var op *operationFault
			if !errors.As(returnErr, &op) {
				returnErr = fault("outbox", returnErr)
			}
		}
	}()
	// The host owns one scanner/worker. A mutex also serializes admin retries.
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, e := s.DB.QueryContext(ctx, `SELECT id,kind,aggregate_id,version,payload,attempts FROM biz_openim_outbox WHERE status IN ('pending','failed') AND next_attempt<=? ORDER BY CASE kind WHEN 'revoke' THEN 0 WHEN 'user' THEN 1 ELSE 2 END,created_at LIMIT 100`, time.Now().Unix())
	if e != nil {
		return e
	}
	type task struct {
		id, kind, aggregate, raw string
		version                  int64
		attempts                 int
	}
	tasks := []task{}
	for rows.Next() {
		var t task
		if e = rows.Scan(&t.id, &t.kind, &t.aggregate, &t.version, &t.raw, &t.attempts); e != nil {
			rows.Close()
			return e
		}
		tasks = append(tasks, t)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	started := time.Now()
	for _, t := range tasks {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Keep each HTTP request's existing timeout and allow its result to be
		// persisted. Stop between tasks rather than canceling a successful
		// downstream write before its acknowledgement can be saved. Remaining
		// durable tasks are picked up on the next tick.
		if time.Since(started) >= budget {
			break
		}
		switch t.kind {
		case "revoke":
			var r Revoke
			e = json.Unmarshal([]byte(t.raw), &r)
			if e == nil {
				e = remote.Revoke(ctx, r)
			}
		case "user":
			var p Projection
			e = json.Unmarshal([]byte(t.raw), &p)
			if e == nil {
				e = remote.User(ctx, p)
			}
		case "department":
			var d Department
			departmentMutation.RLock()
			allowed := json.Unmarshal([]byte(t.raw), &d) == nil && s.departmentDispatchAllowed(ctx, d)
			if allowed {
				e = remote.Department(ctx, d)
			}
			departmentMutation.RUnlock()
			if !allowed {
				if _, err := s.DB.ExecContext(ctx, `UPDATE biz_openim_outbox SET status='superseded',updated_at=? WHERE id=?`, time.Now().Unix(), t.id); err != nil {
					return err
				}
				continue
			}
		}
		if e != nil {
			attempts := t.attempts + 1
			delay := time.Duration(1<<min(attempts, 6)) * time.Second
			safeError := "OpenIM同步失败，请检查服务状态后重试"
			if _, err := s.DB.ExecContext(ctx, `UPDATE biz_openim_outbox SET status='failed',attempts=?,next_attempt=?,last_error=?,updated_at=? WHERE id=?`, attempts, time.Now().Add(delay).Unix(), safeError, time.Now().Unix(), t.id); err != nil {
				return err
			}
			table, key := "biz_openim_employee", "employee_id"
			if t.kind == "department" {
				table, key = "biz_openim_department", "department_id"
			}
			if t.kind != "revoke" {
				if _, err := s.DB.ExecContext(ctx, `UPDATE `+table+` SET status='failed',last_error=? WHERE `+key+`=? AND desired_version=?`, safeError, t.aggregate, t.version); err != nil {
					return err
				}
			}
			continue
		}
		if _, e = s.DB.ExecContext(ctx, `UPDATE biz_openim_outbox SET status='done',last_error='',updated_at=? WHERE id=?`, time.Now().Unix(), t.id); e != nil {
			return e
		}
		if t.kind != "revoke" {
			table, key := "biz_openim_employee", "employee_id"
			if t.kind == "department" {
				table, key = "biz_openim_department", "department_id"
			}
			if _, e = s.DB.ExecContext(ctx, `UPDATE `+table+` SET applied_version=?,status='ready',last_error='' WHERE `+key+`=? AND desired_version=?`, t.version, t.aggregate, t.version); e != nil {
				return e
			}
		}
	}
	return nil
}
func (s *SQLStore) Retry(ctx context.Context) error {
	_, e := s.DB.ExecContext(ctx, `UPDATE biz_openim_outbox SET status='pending',next_attempt=0 WHERE status='failed'`)
	return e
}
func (s *SQLStore) QueueRevoke(ctx context.Context, r Revoke) error {
	value, e := token()
	if e != nil {
		return e
	}
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	_, e = s.DB.ExecContext(ctx, `INSERT INTO biz_openim_outbox(id,kind,aggregate_id,version,payload,status,attempts,next_attempt,last_error,created_at,updated_at) VALUES(?,'revoke',?,0,?,'pending',0,0,'',?,?)`, "logout:"+value[:32], r.UserID, string(b), time.Now().Unix(), time.Now().Unix())
	return e
}
func (s *SQLStore) List(ctx context.Context, department bool) (any, error) {
	table, key := "biz_openim_employee", "employee_id"
	if department {
		table, key = "biz_openim_department", "department_id"
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT `+key+`,projection_json,status,last_error,desired_version,applied_version FROM `+table+` ORDER BY `+key)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, raw, status, last string
		var desired, applied int64
		if e = rows.Scan(&id, &raw, &status, &last, &desired, &applied); e != nil {
			return nil, e
		}
		var p any
		if e = json.Unmarshal([]byte(raw), &p); e != nil {
			return nil, e
		}
		if object, ok := p.(map[string]any); ok {
			delete(object, "authFingerprint")
		}
		out = append(out, map[string]any{"id": id, "projection": p, "status": status, "error": last, "desiredVersion": desired, "appliedVersion": applied})
	}
	return out, rows.Err()
}
func (s *SQLStore) Directory(ctx context.Context, keyword string, page, size int) (any, error) {
	return s.DirectoryInDepartment(ctx, keyword, "", page, size)
}

type DirectoryDepartment struct {
	ID           string `json:"id"`
	ParentID     string `json:"parentID"`
	Name         string `json:"name"`
	GroupEnabled bool   `json:"groupEnabled"`
	LeaderUserID string `json:"leaderUserID"`
	LeaderName   string `json:"leaderName"`
}

func (s *SQLStore) directoryDepartments(ctx context.Context) ([]DirectoryDepartment, error) {
	return s.directoryDepartmentsForScope(ctx, nil)
}

func (s *SQLStore) DirectoryInDepartment(ctx context.Context, keyword, departmentID string, page, size int) (any, error) {
	return s.directory(ctx, nil, keyword, departmentID, page, size)
}
