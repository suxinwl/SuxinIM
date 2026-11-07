package accountsecurity

import (
	"context"
	"time"
)

// MigrateLegacyLocks preserves active pre-upgrade locks. Historical attempt
// counts have no timestamps and cannot be invented into a new rolling window.
func (s *Store) MigrateLegacyLocks(ctx context.Context) error {
	rows, e := s.DB.QueryContext(ctx, `SELECT id,lock_time FROM `+s.Prefix+`admin WHERE lock_time IS NOT NULL AND deletetime IS NULL`)
	if e != nil {
		return e
	}
	type item struct {
		uid int64
		at  time.Time
	}
	items := []item{}
	for rows.Next() {
		var uid int64
		var at legacyTime
		if e = rows.Scan(&uid, &at); e != nil {
			rows.Close()
			return e
		}
		if at.Valid && at.Time.After(s.now()) {
			items = append(items, item{uid, at.Time})
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, v := range items {
		if _, e = tx.ExecContext(ctx, `INSERT INTO `+s.Prefix+`account_security(uid,must_change,locked_until,updated_at) SELECT ?,0,?,? WHERE NOT EXISTS(SELECT 1 FROM `+s.Prefix+`account_security WHERE uid=?)`, v.uid, v.at.Unix(), s.now().Unix(), v.uid); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE `+s.Prefix+`account_security SET locked_until=CASE WHEN locked_until<? THEN ? ELSE locked_until END WHERE uid=?`, v.at.Unix(), v.at.Unix(), v.uid); e != nil {
			return e
		}
	}
	_, e = tx.ExecContext(ctx, `UPDATE `+s.Prefix+`admin SET login_attempts=0 WHERE login_attempts<>0`)
	if e != nil {
		return e
	}
	return tx.Commit()
}
