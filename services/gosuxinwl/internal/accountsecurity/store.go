// Package accountsecurity owns the shared HRM/IM account security state.
// The existing IM epoch is the single security version for both products.
package accountsecurity

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"
)

var (
	ErrDenied                 = errors.New("账号、密码或验证码无效")
	ErrConflict               = errors.New("配置已更新，请刷新后重试")
	ErrInvalid                = errors.New("参数无效")
	ErrPasswordChangeRequired = errors.New("请先修改密码")
)

type Store struct {
	DB        *sql.DB
	Prefix    string
	IMEnabled bool
	Now       func() time.Time
	mu        sync.Mutex
}
type query interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func New(db *sql.DB, prefix string) (*Store, error) {
	if db == nil || !regexp.MustCompile(`^[a-zA-Z0-9_]*$`).MatchString(prefix) {
		return nil, ErrInvalid
	}
	return &Store{DB: db, Prefix: prefix}, nil
}
func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func Hash(v string) string { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }
func passwordHash(password, salt string) string {
	h := md5.Sum([]byte(password + salt))
	return hex.EncodeToString(h[:])
}
func validPassword(v string) bool { return regexp.MustCompile(`^[a-fA-F0-9]{32}$`).MatchString(v) }

func (s *Store) Ensure(ctx context.Context) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS ` + s.Prefix + `account_security(uid BIGINT PRIMARY KEY,must_change INTEGER NOT NULL DEFAULT 0,locked_until BIGINT NOT NULL DEFAULT 0,updated_at BIGINT NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS ` + s.Prefix + `account_lock_policy(singleton INTEGER PRIMARY KEY,window_seconds BIGINT NOT NULL,failures INTEGER NOT NULL,lock_seconds BIGINT NOT NULL,revision BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS ` + s.Prefix + `account_failure(id VARCHAR(64) PRIMARY KEY,uid BIGINT NOT NULL,failed_at BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS ` + s.Prefix + `account_change_ticket(token_hash VARCHAR(64) PRIMARY KEY,uid BIGINT NOT NULL,expires_at BIGINT NOT NULL,epoch BIGINT NOT NULL,credential VARCHAR(64) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS ` + s.Prefix + `account_security_audit(id VARCHAR(64) PRIMARY KEY,uid BIGINT NOT NULL,actor_uid BIGINT NOT NULL,action VARCHAR(64) NOT NULL,created_at BIGINT NOT NULL,details_json TEXT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_openim_epoch(uid BIGINT PRIMARY KEY,epoch BIGINT NOT NULL DEFAULT 1)`,
		`CREATE TABLE IF NOT EXISTS biz_openim_outbox(id VARCHAR(100) PRIMARY KEY,kind VARCHAR(20) NOT NULL,aggregate_id VARCHAR(64) NOT NULL,version BIGINT NOT NULL,payload TEXT NOT NULL,status VARCHAR(20) NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,next_attempt BIGINT NOT NULL DEFAULT 0,last_error VARCHAR(500) NOT NULL,created_at BIGINT NOT NULL,updated_at BIGINT NOT NULL)`,
	} {
		if _, e := s.DB.ExecContext(ctx, q); e != nil {
			return e
		}
	}
	_, e := s.DB.ExecContext(ctx, `INSERT INTO `+s.Prefix+`account_lock_policy(singleton,window_seconds,failures,lock_seconds,revision) SELECT 1,600,5,1800,1 WHERE NOT EXISTS(SELECT 1 FROM `+s.Prefix+`account_lock_policy WHERE singleton=1)`)
	if e != nil {
		return e
	}
	return s.EnsurePolicies(ctx)
}

type LockPolicy struct {
	WindowSeconds int64 `json:"windowSeconds"`
	Failures      int   `json:"failures"`
	LockSeconds   int64 `json:"lockSeconds"`
	Revision      int64 `json:"revision"`
}

func (s *Store) LockPolicy(ctx context.Context) (LockPolicy, error) { return s.lockPolicy(ctx, s.DB) }
func (s *Store) lockPolicy(ctx context.Context, q query) (v LockPolicy, e error) {
	e = q.QueryRowContext(ctx, `SELECT window_seconds,failures,lock_seconds,revision FROM `+s.Prefix+`account_lock_policy WHERE singleton=1`).Scan(&v.WindowSeconds, &v.Failures, &v.LockSeconds, &v.Revision)
	return
}
func (s *Store) SaveLockPolicy(ctx context.Context, v LockPolicy, actor int64) (LockPolicy, error) {
	if actor <= 0 || v.Failures < 1 || v.Failures > 100 || v.WindowSeconds < 60 || v.WindowSeconds > 86400 || v.LockSeconds < 60 || v.LockSeconds > 604800 {
		return v, ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return v, e
	}
	defer tx.Rollback()
	r, e := tx.ExecContext(ctx, `UPDATE `+s.Prefix+`account_lock_policy SET window_seconds=?,failures=?,lock_seconds=?,revision=revision+1 WHERE singleton=1 AND revision=?`, v.WindowSeconds, v.Failures, v.LockSeconds, v.Revision)
	if e != nil {
		return v, e
	}
	n, e := r.RowsAffected()
	if e != nil || n != 1 {
		return v, ErrConflict
	}
	if e = s.audit(ctx, tx, 0, actor, "lock_policy"); e != nil {
		return v, e
	}
	v.Revision++
	return v, tx.Commit()
}

type State struct {
	UID             int64 `json:"uid"`
	MustChange      bool  `json:"mustChangePassword"`
	LockedUntil     int64 `json:"lockedUntil"`
	Locked          bool  `json:"locked"`
	SecurityVersion int64 `json:"securityVersion"`
	RecentFailures  int   `json:"recentFailures"`
}

func (s *Store) state(ctx context.Context, q query, uid int64) (v State, e error) {
	v.UID = uid
	var change int
	e = q.QueryRowContext(ctx, `SELECT must_change,locked_until FROM `+s.Prefix+`account_security WHERE uid=?`, uid).Scan(&change, &v.LockedUntil)
	if errors.Is(e, sql.ErrNoRows) {
		e = nil
	}
	if e != nil {
		return
	}
	v.MustChange = change == 1
	v.Locked = v.LockedUntil > s.now().Unix()
	e = q.QueryRowContext(ctx, `SELECT epoch FROM biz_openim_epoch WHERE uid=?`, uid).Scan(&v.SecurityVersion)
	if errors.Is(e, sql.ErrNoRows) {
		v.SecurityVersion = 1
		e = nil
	}
	return
}
func (s *Store) State(ctx context.Context, uid int64) (State, error) {
	v, e := s.state(ctx, s.DB, uid)
	if e != nil {
		return v, e
	}
	p, e := s.LockPolicy(ctx)
	if e != nil {
		return v, e
	}
	e = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.Prefix+`account_failure WHERE uid=? AND failed_at>?`, uid, s.now().Unix()-p.WindowSeconds).Scan(&v.RecentFailures)
	return v, e
}

// Account-row locking serializes password checks, failures, resets and unlocks
// across processes. SQLite tests use the same write-first transaction ordering.
func (s *Store) lock(ctx context.Context, tx *sql.Tx, uid int64) error {
	r, e := tx.ExecContext(ctx, `UPDATE `+s.Prefix+`admin SET login_attempts=login_attempts WHERE id=? AND status=0 AND deletetime IS NULL`, uid)
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		// MySQL may report changed rows rather than matched rows for a no-op UPDATE.
		var active int
		e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.Prefix+`admin WHERE id=? AND status=0 AND deletetime IS NULL`, uid).Scan(&active)
		if e != nil {
			return e
		}
		if active != 1 {
			return ErrDenied
		}
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO `+s.Prefix+`account_security(uid,must_change,locked_until,updated_at) SELECT ?,0,0,? WHERE NOT EXISTS(SELECT 1 FROM `+s.Prefix+`account_security WHERE uid=?)`, uid, s.now().Unix(), uid)
	return e
}
func (s *Store) audit(ctx context.Context, q query, uid, actor int64, action string, details ...any) error {
	id, e := randomToken()
	if e != nil {
		return e
	}
	var detail any
	if len(details) > 0 {
		detail = details[0]
	}
	raw, _ := json.Marshal(detail)
	_, e = q.ExecContext(ctx, `INSERT INTO `+s.Prefix+`account_security_audit(id,uid,actor_uid,action,created_at,details_json) VALUES(?,?,?,?,?,?)`, id, uid, actor, action, s.now().Unix(), string(raw))
	return e
}

// RevokeTx must be called inside the SAME transaction as the security change.
// No remote RPC is required to commit: fresh access checks reject immediately.
func (s *Store) RevokeTx(ctx context.Context, tx *sql.Tx, uid int64) error {
	_, e := tx.ExecContext(ctx, `INSERT INTO biz_openim_epoch(uid,epoch) SELECT ?,1 WHERE NOT EXISTS(SELECT 1 FROM biz_openim_epoch WHERE uid=?)`, uid, uid)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE biz_openim_epoch SET epoch=epoch+1 WHERE uid=?`, uid); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM `+s.Prefix+`account_change_ticket WHERE uid=?`, uid); e != nil {
		return e
	}
	if !s.IMEnabled {
		return nil
	}
	var userID string
	e = tx.QueryRowContext(ctx, `SELECT openim_user_id FROM biz_openim_employee WHERE uid=?`, uid).Scan(&userID)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	var epoch int64
	if e = tx.QueryRowContext(ctx, `SELECT epoch FROM biz_openim_epoch WHERE uid=?`, uid).Scan(&epoch); e != nil {
		return e
	}
	id, e := randomToken()
	if e != nil {
		return e
	}
	body, _ := json.Marshal(map[string]any{"userID": userID, "all": true, "epoch": epoch})
	_, e = tx.ExecContext(ctx, `INSERT INTO biz_openim_outbox(id,kind,aggregate_id,version,payload,status,attempts,next_attempt,last_error,created_at,updated_at) VALUES(?,'revoke',?,0,?,'pending',0,0,'',?,?)`, "security:"+id, userID, string(body), s.now().Unix(), s.now().Unix())
	return e
}
func (s *Store) Revoke(ctx context.Context, uid int64) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = s.RevokeTx(ctx, tx, uid); e != nil {
		return e
	}
	return tx.Commit()
}

// Authenticate is only called AFTER a valid captcha (when captcha is enabled).
// All failures use a rolling time window; captcha failures never enter it.
func (s *Store) Authenticate(ctx context.Context, uid int64, password string) error {
	_, e := s.AuthenticateVersion(ctx, uid, password)
	return e
}
func (s *Store) AuthenticateVersion(ctx context.Context, uid int64, password string) (version int64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if uid <= 0 || !validPassword(password) {
		return 0, ErrDenied
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	if e = s.lock(ctx, tx, uid); e != nil {
		return 0, e
	}
	state, e := s.state(ctx, tx, uid)
	if e != nil {
		return 0, e
	}
	if state.Locked {
		return 0, ErrDenied
	}
	var stored, salt string
	var legacyLock legacyTime
	if e = tx.QueryRowContext(ctx, `SELECT COALESCE(password,''),COALESCE(salt,''),lock_time FROM `+s.Prefix+`admin WHERE id=?`, uid).Scan(&stored, &salt, &legacyLock); e != nil {
		return 0, e
	}
	if legacyLock.Valid && legacyLock.Time.After(s.now()) {
		return 0, ErrDenied
	}
	if subtle.ConstantTimeCompare([]byte(passwordHash(password, salt)), []byte(stored)) != 1 {
		p, e := s.lockPolicy(ctx, tx)
		if e != nil {
			return 0, e
		}
		id, e := randomToken()
		if e != nil {
			return 0, e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO `+s.Prefix+`account_failure(id,uid,failed_at) VALUES(?,?,?)`, id, uid, s.now().Unix()); e != nil {
			return 0, e
		}
		var count int
		if e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.Prefix+`account_failure WHERE uid=? AND failed_at>?`, uid, s.now().Unix()-p.WindowSeconds).Scan(&count); e != nil {
			return 0, e
		}
		if count >= p.Failures {
			if _, e = tx.ExecContext(ctx, `UPDATE `+s.Prefix+`account_security SET locked_until=?,updated_at=? WHERE uid=?`, s.now().Unix()+p.LockSeconds, s.now().Unix(), uid); e != nil {
				return 0, e
			}
			if e = s.RevokeTx(ctx, tx, uid); e != nil {
				return 0, e
			}
			if e = s.audit(ctx, tx, uid, 0, "account_locked"); e != nil {
				return 0, e
			}
			if _, e = tx.ExecContext(ctx, `DELETE FROM `+s.Prefix+`account_failure WHERE uid=?`, uid); e != nil {
				return 0, e
			}
		}
		if e = tx.Commit(); e != nil {
			return 0, e
		}
		return 0, ErrDenied
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM `+s.Prefix+`account_failure WHERE uid=?`, uid); e != nil {
		return 0, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE `+s.Prefix+`admin SET login_attempts=0,lock_time=NULL WHERE id=?`, uid); e != nil {
		return 0, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE `+s.Prefix+`account_security SET locked_until=0 WHERE uid=?`, uid); e != nil {
		return 0, e
	}
	if e = tx.Commit(); e != nil {
		return 0, e
	}
	return state.SecurityVersion, nil
}

func (s *Store) Unlock(ctx context.Context, uid, actor int64) error {
	if uid <= 0 || actor <= 0 {
		return ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	// Disabled accounts can be unlocked without re-enabling them.
	if _, e = tx.ExecContext(ctx, `UPDATE `+s.Prefix+`admin SET login_attempts=0,lock_time=NULL WHERE id=? AND deletetime IS NULL`, uid); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE `+s.Prefix+`account_security SET locked_until=0,updated_at=? WHERE uid=?`, s.now().Unix(), uid); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM `+s.Prefix+`account_failure WHERE uid=?`, uid); e != nil {
		return e
	}
	if e = s.audit(ctx, tx, uid, actor, "account_unlocked"); e != nil {
		return e
	}
	return tx.Commit()
}

type ChangeChallenge struct {
	ReasonCode             string `json:"reasonCode"`
	RequiresPasswordChange bool   `json:"requiresPasswordChange"`
	ChangeTicket           string `json:"changeTicket"`
	ExpiresAt              int64  `json:"expiresAt"`
}

func (s *Store) Challenge(ctx context.Context, uid int64, expected ...int64) (ChangeChallenge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return ChangeChallenge{}, e
	}
	defer tx.Rollback()
	if e = s.lock(ctx, tx, uid); e != nil {
		return ChangeChallenge{}, e
	}
	state, e := s.state(ctx, tx, uid)
	if e != nil {
		return ChangeChallenge{}, e
	}
	if !state.MustChange || state.Locked {
		return ChangeChallenge{}, ErrDenied
	}
	if len(expected) > 0 && expected[0] > 0 && expected[0] != state.SecurityVersion {
		return ChangeChallenge{}, ErrDenied
	}
	var password, salt string
	if e = tx.QueryRowContext(ctx, `SELECT COALESCE(password,''),COALESCE(salt,'') FROM `+s.Prefix+`admin WHERE id=?`, uid).Scan(&password, &salt); e != nil {
		return ChangeChallenge{}, e
	}
	raw, e := randomToken()
	if e != nil {
		return ChangeChallenge{}, e
	}
	expires := s.now().Add(5 * time.Minute).Unix()
	if _, e = tx.ExecContext(ctx, `DELETE FROM `+s.Prefix+`account_change_ticket WHERE uid=?`, uid); e != nil {
		return ChangeChallenge{}, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO `+s.Prefix+`account_change_ticket(token_hash,uid,expires_at,epoch,credential) VALUES(?,?,?,?,?)`, Hash(raw), uid, expires, state.SecurityVersion, Hash(password+":"+salt)); e != nil {
		return ChangeChallenge{}, e
	}
	if e = tx.Commit(); e != nil {
		return ChangeChallenge{}, e
	}
	return ChangeChallenge{"PASSWORD_CHANGE_REQUIRED", true, raw, expires}, nil
}
func (s *Store) TicketStatus(ctx context.Context, ticket string) (ChangeChallenge, error) {
	if len(ticket) != 64 {
		return ChangeChallenge{}, ErrDenied
	}
	var uid, expires, epoch int64
	var credential string
	e := s.DB.QueryRowContext(ctx, `SELECT uid,expires_at,epoch,credential FROM `+s.Prefix+`account_change_ticket WHERE token_hash=?`, Hash(ticket)).Scan(&uid, &expires, &epoch, &credential)
	if e != nil || expires <= s.now().Unix() {
		return ChangeChallenge{}, ErrDenied
	}
	v, e := s.state(ctx, s.DB, uid)
	if e != nil {
		return ChangeChallenge{}, e
	}
	if !v.MustChange || v.Locked || v.SecurityVersion != epoch {
		return ChangeChallenge{}, ErrDenied
	}
	var password, salt string
	var count int
	if e = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.Prefix+`admin WHERE id=? AND status=0 AND deletetime IS NULL`, uid).Scan(&count); e != nil || count != 1 {
		return ChangeChallenge{}, ErrDenied
	}
	if e = s.DB.QueryRowContext(ctx, `SELECT COALESCE(password,''),COALESCE(salt,'') FROM `+s.Prefix+`admin WHERE id=?`, uid).Scan(&password, &salt); e != nil || Hash(password+":"+salt) != credential {
		return ChangeChallenge{}, ErrDenied
	}
	return ChangeChallenge{ReasonCode: "PASSWORD_CHANGE_REQUIRED", RequiresPasswordChange: true, ExpiresAt: expires}, nil
}
func (s *Store) CancelTicket(ctx context.Context, ticket string) error {
	if len(ticket) != 64 {
		return ErrDenied
	}
	_, e := s.DB.ExecContext(ctx, `DELETE FROM `+s.Prefix+`account_change_ticket WHERE token_hash=?`, Hash(ticket))
	return e
}

// ChangePassword is also used by admin reset; only reset sets must_change.
// Existing MD5 transport remains compatible. Neither hashes nor salts leave it.
func (s *Store) ChangePassword(ctx context.Context, uid, actor int64, old, newPassword, ticket string, reset bool) error {
	if !validPassword(newPassword) || uid < 0 {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Resolve the account outside the transaction, then lock that account BEFORE
	// the transactional ticket read. This avoids an older MySQL RR snapshot
	// authorizing a ticket that a concurrent reset has already superseded.
	if ticket != "" {
		if len(ticket) != 64 {
			return ErrDenied
		}
		if e := s.DB.QueryRowContext(ctx, `SELECT uid FROM `+s.Prefix+`account_change_ticket WHERE token_hash=?`, Hash(ticket)).Scan(&uid); e != nil {
			return ErrDenied
		}
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var expires, ticketEpoch int64
	var credential string
	if ticket != "" {
		if e = s.lock(ctx, tx, uid); e != nil {
			return e
		}
	}
	if ticket != "" {
		if len(ticket) != 64 {
			return ErrDenied
		}
		e = tx.QueryRowContext(ctx, `SELECT uid,expires_at,epoch,credential FROM `+s.Prefix+`account_change_ticket WHERE token_hash=?`, Hash(ticket)).Scan(&uid, &expires, &ticketEpoch, &credential)
		if e != nil || expires <= s.now().Unix() {
			return ErrDenied
		}
	}
	if uid <= 0 {
		return ErrInvalid
	}
	// A reset of an inactive account is allowed but cannot enable it. Self-change
	// requires the same active account used for login.
	if !reset && ticket == "" {
		if e = s.lock(ctx, tx, uid); e != nil {
			return e
		}
	} else if reset {
		if actor <= 0 {
			return ErrDenied
		}
		if _, e = tx.ExecContext(ctx, `UPDATE `+s.Prefix+`admin SET login_attempts=login_attempts WHERE id=? AND deletetime IS NULL`, uid); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO `+s.Prefix+`account_security(uid,must_change,locked_until,updated_at) SELECT ?,0,0,? WHERE NOT EXISTS(SELECT 1 FROM `+s.Prefix+`account_security WHERE uid=?)`, uid, s.now().Unix(), uid); e != nil {
			return e
		}
	}
	if actor == 0 && !reset {
		actor = uid
	}
	state, e := s.state(ctx, tx, uid)
	if e != nil {
		return e
	}
	if !reset && state.Locked {
		return ErrDenied
	}
	var stored, salt string
	if e = tx.QueryRowContext(ctx, `SELECT COALESCE(password,''),COALESCE(salt,'') FROM `+s.Prefix+`admin WHERE id=? AND deletetime IS NULL`, uid).Scan(&stored, &salt); e != nil {
		return ErrDenied
	}
	if ticket != "" {
		if !state.MustChange || state.SecurityVersion != ticketEpoch || credential != Hash(stored+":"+salt) {
			return ErrDenied
		}
	} else if !reset {
		if !validPassword(old) || subtle.ConstantTimeCompare([]byte(passwordHash(old, salt)), []byte(stored)) != 1 {
			return ErrDenied
		}
	}
	if passwordHash(newPassword, salt) == stored {
		return errors.New("新密码不能与当前密码相同")
	}
	fresh, e := randomToken()
	if e != nil {
		return e
	}
	salt = fresh[:24]
	if _, e = tx.ExecContext(ctx, `UPDATE `+s.Prefix+`admin SET password=?,salt=?,pwd_reset_time=? WHERE id=? AND deletetime IS NULL`, passwordHash(newPassword, salt), salt, s.now(), uid); e != nil {
		return e
	}
	change := 0
	action := "password_changed"
	if reset {
		change = 1
		action = "password_reset"
	}
	if _, e = tx.ExecContext(ctx, `UPDATE `+s.Prefix+`account_security SET must_change=?,updated_at=? WHERE uid=?`, change, s.now().Unix(), uid); e != nil {
		return e
	}
	if e = s.RevokeTx(ctx, tx, uid); e != nil {
		return e
	}
	if e = s.audit(ctx, tx, uid, actor, action); e != nil {
		return e
	}
	return tx.Commit()
}

func (s *Store) CheckSession(ctx context.Context, uid, version int64, fingerprint string) error {
	var password, salt string
	var status, deleted int
	if e := s.DB.QueryRowContext(ctx, `SELECT COALESCE(password,''),COALESCE(salt,''),status,CASE WHEN deletetime IS NULL THEN 0 ELSE 1 END FROM `+s.Prefix+`admin WHERE id=?`, uid).Scan(&password, &salt, &status, &deleted); e != nil {
		return ErrDenied
	}
	if status != 0 || deleted != 0 || version <= 0 || fingerprint != Hash(password+":"+salt) {
		return ErrDenied
	}
	v, e := s.state(ctx, s.DB, uid)
	if e != nil {
		return e
	}
	if v.Locked || v.MustChange || v.SecurityVersion != version {
		return ErrDenied
	}
	return nil
}
func (s *Store) CheckActive(ctx context.Context, uid int64) error {
	v, e := s.state(ctx, s.DB, uid)
	if e != nil {
		return e
	}
	if v.Locked {
		return ErrDenied
	}
	if v.MustChange {
		return ErrPasswordChangeRequired
	}
	return nil
}

func (s *Store) String() string { return fmt.Sprintf("accountsecurity.Store(prefix=%q)", s.Prefix) }
