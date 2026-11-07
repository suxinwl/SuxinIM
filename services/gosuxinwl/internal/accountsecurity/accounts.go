package accountsecurity

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// SaveAccount uses a field allowlist and the same SQL transaction for account,
// role membership, security version and revocation delivery. Resetting an
// existing password is deliberately a separate authorized operation.
func (s *Store) SaveAccount(ctx context.Context, uid, actor int64, fields map[string]any, roles []int64, password string) (int64, error) {
	if actor <= 0 || uid < 0 {
		return 0, ErrDenied
	}
	if uid > 0 {
		if password != "" {
			return 0, fmt.Errorf("请使用独立的重置密码操作")
		}
		if e := s.AssertManagedTarget(ctx, actor, uid, false); e != nil {
			return 0, e
		}
	} else if !validPassword(password) {
		return 0, fmt.Errorf("新账号必须设置初始密码")
	}
	for _, role := range roles {
		if e := s.AssertManagedTarget(ctx, actor, role, true); e != nil {
			return 0, e
		}
	}
	allowed := map[string]bool{"name": true, "username": true, "mobile": true, "tel": true, "email": true, "avatar": true, "remark": true, "dept_id": true}
	keys := []string{}
	for key := range fields {
		if !allowed[key] {
			return 0, ErrInvalid
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	args := []any{}
	for _, key := range keys {
		args = append(args, fields[key])
	}
	create := uid == 0
	if create {
		token, e := randomToken()
		if e != nil {
			return 0, e
		}
		salt := token[:24]
		keys = append(keys, "password", "salt", "account_id", "createtime")
		args = append(args, passwordHash(password, salt), salt, actor, s.now())
		marks := make([]string, len(keys))
		for i := range marks {
			marks[i] = "?"
		}
		r, e := tx.ExecContext(ctx, `INSERT INTO `+s.Prefix+`admin(`+strings.Join(keys, ",")+`) VALUES(`+strings.Join(marks, ",")+`)`, args...)
		if e != nil {
			return 0, e
		}
		uid, e = r.LastInsertId()
		if e != nil || uid <= 0 {
			return 0, ErrDenied
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO `+s.Prefix+`account_security(uid,must_change,locked_until,updated_at) VALUES(?,1,0,?)`, uid, s.now().Unix()); e != nil {
			return 0, e
		}
	} else {
		sets := make([]string, len(keys))
		for i, key := range keys {
			sets[i] = key + "=?"
		}
		args = append(args, uid)
		if _, e = tx.ExecContext(ctx, `UPDATE `+s.Prefix+`admin SET `+strings.Join(sets, ",")+` WHERE id=? AND deletetime IS NULL`, args...); e != nil {
			return 0, e
		}
	}
	if roles != nil {
		if _, e = tx.ExecContext(ctx, `DELETE FROM `+s.Prefix+`auth_role_access WHERE uid=?`, uid); e != nil {
			return 0, e
		}
		seen := map[int64]bool{}
		for _, role := range roles {
			if seen[role] {
				continue
			}
			seen[role] = true
			if _, e = tx.ExecContext(ctx, `INSERT INTO `+s.Prefix+`auth_role_access(uid,role_id) VALUES(?,?)`, uid, role); e != nil {
				return 0, e
			}
		}
	}
	if e = s.RevokeTx(ctx, tx, uid); e != nil {
		return 0, e
	}
	if e = s.audit(ctx, tx, uid, actor, "account_saved"); e != nil {
		return 0, e
	}
	return uid, tx.Commit()
}
func (s *Store) SetAccountStatus(ctx context.Context, uid, actor int64, status uint, remove bool) error {
	if status > 1 {
		return ErrInvalid
	}
	if e := s.AssertManagedTarget(ctx, actor, uid, false); e != nil {
		return e
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if remove {
		_, e = tx.ExecContext(ctx, `UPDATE `+s.Prefix+`admin SET deletetime=?,status=1 WHERE id=? AND deletetime IS NULL`, s.now(), uid)
	} else {
		_, e = tx.ExecContext(ctx, `UPDATE `+s.Prefix+`admin SET status=? WHERE id=? AND deletetime IS NULL`, status, uid)
	}
	if e != nil {
		return e
	}
	if e = s.RevokeTx(ctx, tx, uid); e != nil {
		return e
	}
	if e = s.audit(ctx, tx, uid, actor, "account_status"); e != nil {
		return e
	}
	return tx.Commit()
}

// RolesChangedTx is shared by role writes. Implementations using framework
// transactions can pass their underlying sql.Tx; no writer may bump an epoch
// without its durable outbox entry.
func (s *Store) RolesChangedTx(ctx context.Context, tx *sql.Tx, roles []int64) error {
	seen := map[int64]bool{}
	for _, role := range roles {
		rows, e := tx.QueryContext(ctx, `SELECT uid FROM `+s.Prefix+`auth_role_access WHERE role_id=?`, role)
		if e != nil {
			return e
		}
		for rows.Next() {
			var uid int64
			if e = rows.Scan(&uid); e != nil {
				rows.Close()
				return e
			}
			seen[uid] = true
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
	}
	for uid := range seen {
		if e := s.RevokeTx(ctx, tx, uid); e != nil {
			return e
		}
	}
	return nil
}
