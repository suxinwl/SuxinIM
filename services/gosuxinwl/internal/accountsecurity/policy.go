package accountsecurity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
)

var Zones = []string{"lan", "internet"}
var Clients = []string{"web_desktop", "web_mobile", "pc", "android", "ios"}

type Cell struct {
	Login string `json:"login"`
	Save  string `json:"save"`
}
type Matrix map[string]map[string]Cell
type Policy struct {
	Subject   string `json:"subject"`
	SubjectID int64  `json:"subjectID"`
	Matrix    Matrix `json:"matrix"`
	Revision  int64  `json:"revision"`
	UpdatedAt int64  `json:"updatedAt"`
	UpdatedBy int64  `json:"updatedBy"`
}
type PolicyView struct {
	Policy
	Effective map[string]map[string]EffectivePolicy `json:"effective"`
}

func (s *Store) PolicyView(ctx context.Context, uid int64) (PolicyView, error) {
	p, e := s.ReadPolicy(ctx, "person", uid)
	v := PolicyView{Policy: p, Effective: map[string]map[string]EffectivePolicy{}}
	if e != nil {
		return v, e
	}
	for _, z := range Zones {
		v.Effective[z] = map[string]EffectivePolicy{}
		for _, c := range Clients {
			cell, e := s.Resolve(ctx, uid, z, c)
			if e != nil {
				return v, e
			}
			v.Effective[z][c] = cell
		}
	}
	return v, nil
}

type Sources struct {
	Login string `json:"login"`
	Save  string `json:"save"`
}
type EffectivePolicy struct {
	Zone            string  `json:"zone"`
	LogicalClient   string  `json:"logicalClient"`
	Login           bool    `json:"login"`
	Preview         bool    `json:"preview"`
	Save            bool    `json:"save"`
	Sources         Sources `json:"sources"`
	PolicyRevision  int64   `json:"policyRevision"`
	NetworkRevision int64   `json:"networkRevision"`
	SecurityVersion int64   `json:"securityVersion"`
	CurrentIP       string  `json:"currentIP"`
}

func ValidClient(v string) bool {
	for _, x := range Clients {
		if x == v {
			return true
		}
	}
	return false
}
func DefaultMatrix(value string) Matrix {
	m := Matrix{}
	for _, z := range Zones {
		m[z] = map[string]Cell{}
		for _, c := range Clients {
			m[z][c] = Cell{value, value}
		}
	}
	return m
}
func ValidateMatrix(m Matrix, personal bool) error {
	if len(m) != len(Zones) {
		return ErrInvalid
	}
	for _, z := range Zones {
		if len(m[z]) != len(Clients) {
			return ErrInvalid
		}
		for _, c := range Clients {
			v, ok := m[z][c]
			if !ok {
				return ErrInvalid
			}
			for _, a := range []string{v.Login, v.Save} {
				if a != "allow" && a != "deny" && !(personal && a == "inherit") {
					return ErrInvalid
				}
			}
		}
	}
	return nil
}
func (s *Store) EnsurePolicies(ctx context.Context) error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS ` + s.Prefix + `account_access_policy(subject VARCHAR(16) NOT NULL,subject_id BIGINT NOT NULL,matrix_json TEXT NOT NULL,revision BIGINT NOT NULL,updated_at BIGINT NOT NULL,updated_by BIGINT NOT NULL,PRIMARY KEY(subject,subject_id))`,
		`CREATE TABLE IF NOT EXISTS ` + s.Prefix + `account_policy_version(singleton INTEGER PRIMARY KEY,revision BIGINT NOT NULL)`,
	} {
		if _, e := s.DB.ExecContext(ctx, q); e != nil {
			return e
		}
	}
	_, e := s.DB.ExecContext(ctx, `INSERT INTO `+s.Prefix+`account_policy_version(singleton,revision) SELECT 1,1 WHERE NOT EXISTS(SELECT 1 FROM `+s.Prefix+`account_policy_version WHERE singleton=1)`)
	return e
}
func (s *Store) ReadPolicy(ctx context.Context, subject string, id int64) (Policy, error) {
	return s.readPolicy(ctx, s.DB, subject, id)
}
func (s *Store) readPolicy(ctx context.Context, q query, subject string, id int64) (p Policy, e error) {
	p = Policy{Subject: subject, SubjectID: id, Matrix: DefaultMatrix("allow")}
	if subject == "person" {
		p.Matrix = DefaultMatrix("inherit")
	}
	if (subject != "person" && subject != "role") || id <= 0 {
		return p, ErrInvalid
	}
	var raw string
	e = q.QueryRowContext(ctx, `SELECT matrix_json,revision,updated_at,updated_by FROM `+s.Prefix+`account_access_policy WHERE subject=? AND subject_id=?`, subject, id).Scan(&raw, &p.Revision, &p.UpdatedAt, &p.UpdatedBy)
	if errors.Is(e, sql.ErrNoRows) {
		return p, nil
	}
	if e != nil {
		return
	}
	if json.Unmarshal([]byte(raw), &p.Matrix) != nil || ValidateMatrix(p.Matrix, subject == "person") != nil {
		return p, ErrDenied
	}
	return
}

// Resolve only unions configured, active roles. With no configured role it
// preserves the previous all-allow transport default. Personal cells are final.
func (s *Store) Resolve(ctx context.Context, uid int64, zone, client string) (EffectivePolicy, error) {
	v := EffectivePolicy{Zone: zone, LogicalClient: client, Sources: Sources{"default", "default"}}
	if (zone != "lan" && zone != "internet") || !ValidClient(client) {
		return v, ErrDenied
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT r.id FROM `+s.Prefix+`auth_role r JOIN `+s.Prefix+`auth_role_access a ON a.role_id=r.id WHERE a.uid=? AND r.status=0 ORDER BY r.id`, uid)
	if e != nil {
		return v, e
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return v, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return v, e
	}
	count := 0
	login, save := false, false
	for _, id := range ids {
		p, e := s.ReadPolicy(ctx, "role", id)
		if e != nil {
			return v, e
		}
		if p.Revision > 0 {
			count++
			c := p.Matrix[zone][client]
			login = login || c.Login == "allow"
			save = save || c.Save == "allow"
		}
	}
	if count == 0 {
		login, save = true, true
	} else {
		v.Sources = Sources{"roles", "roles"}
	}
	person, e := s.ReadPolicy(ctx, "person", uid)
	if e != nil {
		return v, e
	}
	if person.Revision == 0 && s.IMEnabled {
		var lan, pc, mobile, internet, download int
		e = s.DB.QueryRowContext(ctx, `SELECT allow_lan,allow_pc,allow_mobile,allow_internet,allow_internet_download FROM biz_openim_employee_access WHERE employee_id=(SELECT employee_id FROM biz_openim_employee WHERE uid=?)`, uid).Scan(&lan, &pc, &mobile, &internet, &download)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return v, e
		}
		if e == nil {
			person.Matrix = LegacyMatrix(lan == 1, pc == 1, mobile == 1, internet == 1, download == 1)
		}
	}
	c := person.Matrix[zone][client]
	if c.Login != "inherit" {
		login = c.Login == "allow"
		v.Sources.Login = "person"
	}
	if c.Save != "inherit" {
		save = c.Save == "allow"
		v.Sources.Save = "person"
	}
	v.Login = login
	v.Preview = login
	v.Save = login && save
	e = s.DB.QueryRowContext(ctx, `SELECT revision FROM `+s.Prefix+`account_policy_version WHERE singleton=1`).Scan(&v.PolicyRevision)
	if e != nil {
		return v, e
	}
	state, e := s.state(ctx, s.DB, uid)
	v.SecurityVersion = state.SecurityVersion
	return v, e
}

func (s *Store) SavePolicy(ctx context.Context, p Policy, actor int64) (Policy, error) {
	if actor <= 0 || p.SubjectID <= 0 || (p.Subject != "person" && p.Subject != "role") || ValidateMatrix(p.Matrix, p.Subject == "person") != nil {
		return p, ErrInvalid
	}
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return p, e
	}
	defer tx.Rollback()
	// A single version row serializes writes, expected revisions and the users
	// affected by a role matrix. It is not a cross-request policy cache.
	if _, e = tx.ExecContext(ctx, `UPDATE `+s.Prefix+`account_policy_version SET revision=revision+1 WHERE singleton=1`); e != nil {
		return p, e
	}
	old, e := s.readPolicy(ctx, tx, p.Subject, p.SubjectID)
	if e != nil {
		return p, e
	}
	if old.Revision != p.Revision {
		return p, ErrConflict
	}
	raw, e := json.Marshal(p.Matrix)
	if e != nil {
		return p, e
	}
	p.Revision++
	p.UpdatedAt = s.now().Unix()
	p.UpdatedBy = actor
	if p.Revision == 1 {
		_, e = tx.ExecContext(ctx, `INSERT INTO `+s.Prefix+`account_access_policy(subject,subject_id,matrix_json,revision,updated_at,updated_by) VALUES(?,?,?,?,?,?)`, p.Subject, p.SubjectID, string(raw), p.Revision, p.UpdatedAt, actor)
	} else {
		_, e = tx.ExecContext(ctx, `UPDATE `+s.Prefix+`account_access_policy SET matrix_json=?,revision=?,updated_at=?,updated_by=? WHERE subject=? AND subject_id=?`, string(raw), p.Revision, p.UpdatedAt, actor, p.Subject, p.SubjectID)
	}
	if e != nil {
		return p, e
	}
	uids := []int64{p.SubjectID}
	if p.Subject == "role" {
		uids = nil
		rows, e := tx.QueryContext(ctx, `SELECT DISTINCT uid FROM `+s.Prefix+`auth_role_access WHERE role_id=?`, p.SubjectID)
		if e != nil {
			return p, e
		}
		for rows.Next() {
			var uid int64
			if e = rows.Scan(&uid); e != nil {
				rows.Close()
				return p, e
			}
			uids = append(uids, uid)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return p, e
		}
	}
	for _, uid := range uids {
		if e = s.RevokeTx(ctx, tx, uid); e != nil {
			return p, e
		}
	}
	if e = s.audit(ctx, tx, p.SubjectID, actor, "policy_"+p.Subject, map[string]any{"before": old, "after": p}); e != nil {
		return p, e
	}
	return p, tx.Commit()
}

// MigrateLegacyPolicies expands only explicitly stored five-flag policies.
// New accounts remain inherited; reruns never overwrite an administrator edit.
func (s *Store) MigrateLegacyPolicies(ctx context.Context) error {
	rows, e := s.DB.QueryContext(ctx, `SELECT e.user_id,p.allow_lan,p.allow_pc,p.allow_mobile,p.allow_internet,p.allow_internet_download FROM biz_openim_employee_access p JOIN biz_hrm_employee e ON e.id=p.employee_id WHERE e.user_id>0`)
	if e != nil {
		return e
	}
	type legacy struct {
		uid                                 int64
		lan, pc, mobile, internet, download int
	}
	items := []legacy{}
	for rows.Next() {
		var v legacy
		if e = rows.Scan(&v.uid, &v.lan, &v.pc, &v.mobile, &v.internet, &v.download); e != nil {
			rows.Close()
			return e
		}
		items = append(items, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, v := range items {
		m := DefaultMatrix("deny")
		for _, z := range Zones {
			for _, client := range Clients {
				allowed := (client == "pc" || client == "web_desktop") && v.pc == 1 || (client == "android" || client == "ios" || client == "web_mobile") && v.mobile == 1
				allowed = allowed && ((z == "lan" && v.lan == 1) || (z == "internet" && v.internet == 1))
				login, save := "deny", "deny"
				if allowed {
					login = "allow"
					if z == "lan" || v.download == 1 {
						save = "allow"
					}
				}
				m[z][client] = Cell{login, save}
			}
		}
		raw, _ := json.Marshal(m)
		if _, e = s.DB.ExecContext(ctx, `INSERT INTO `+s.Prefix+`account_access_policy(subject,subject_id,matrix_json,revision,updated_at,updated_by) SELECT 'person',?,?,1,?,0 WHERE NOT EXISTS(SELECT 1 FROM `+s.Prefix+`account_access_policy WHERE subject='person' AND subject_id=?)`, v.uid, string(raw), s.now().Unix(), v.uid); e != nil {
			return e
		}
	}
	return nil
}

func LegacyMatrix(lan, pc, mobile, internet, download bool) Matrix {
	m := DefaultMatrix("deny")
	for _, z := range Zones {
		for _, c := range Clients {
			allowed := ((c == "pc" || c == "web_desktop") && pc || (c == "web_mobile" || c == "android" || c == "ios") && mobile) && ((z == "lan" && lan) || (z == "internet" && internet))
			v := Cell{"deny", "deny"}
			if allowed {
				v.Login = "allow"
				if z == "lan" || download {
					v.Save = "allow"
				}
			}
			m[z][c] = v
		}
	}
	return m
}
func (s *Store) SetPersonalTx(ctx context.Context, tx *sql.Tx, uid, actor int64, m Matrix) error {
	if uid <= 0 || ValidateMatrix(m, true) != nil {
		return ErrInvalid
	}
	raw, _ := json.Marshal(m)
	old, e := s.readPolicy(ctx, tx, "person", uid)
	if e != nil {
		return e
	}
	if old.Revision == 0 {
		_, e = tx.ExecContext(ctx, `INSERT INTO `+s.Prefix+`account_access_policy(subject,subject_id,matrix_json,revision,updated_at,updated_by) VALUES('person',?,?,1,?,?)`, uid, string(raw), s.now().Unix(), actor)
	} else {
		_, e = tx.ExecContext(ctx, `UPDATE `+s.Prefix+`account_access_policy SET matrix_json=?,revision=revision+1,updated_at=?,updated_by=? WHERE subject='person' AND subject_id=?`, string(raw), s.now().Unix(), actor, uid)
	}
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `UPDATE `+s.Prefix+`account_policy_version SET revision=revision+1 WHERE singleton=1`)
	return e
}

// AssertManagedTarget protects password and policy operations independently
// from menu visibility. Super users manage all; delegated actors are restricted
// to descendants of their assigned role tree, with matching business scope.
func (s *Store) AssertManagedTarget(ctx context.Context, actor, target int64, role bool) error {
	if actor <= 0 || target <= 0 {
		return ErrDenied
	}
	var exists int
	table := "admin"
	suffix := " AND deletetime IS NULL"
	if role {
		table = "auth_role"
		suffix = ""
	}
	if e := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.Prefix+table+` WHERE id=?`+suffix, target).Scan(&exists); e != nil {
		return e
	}
	if exists != 1 {
		return ErrDenied
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT r.id,r.rules,COALESCE(r.business_id,0) FROM `+s.Prefix+`auth_role r JOIN `+s.Prefix+`auth_role_access a ON a.role_id=r.id WHERE a.uid=? AND r.status=0`, actor)
	if e != nil {
		return e
	}
	assigned := map[int64]bool{}
	business := map[int64]bool{}
	super := false
	for rows.Next() {
		var id, b int64
		var rules string
		if e = rows.Scan(&id, &rules, &b); e != nil {
			rows.Close()
			return e
		}
		assigned[id] = true
		business[b] = true
		super = super || rules == "*"
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if super {
		return nil
	}
	if !role && actor == target {
		return ErrDenied
	} // Reset is an administrator action; use self-change.
	rows, e = s.DB.QueryContext(ctx, `SELECT id,pid,COALESCE(business_id,0),rules FROM `+s.Prefix+`auth_role`)
	if e != nil {
		return e
	}
	parents := map[int64]int64{}
	scope := map[int64]int64{}
	protected := map[int64]bool{}
	for rows.Next() {
		var id, pid, b int64
		var rules string
		if e = rows.Scan(&id, &pid, &b, &rules); e != nil {
			rows.Close()
			return e
		}
		parents[id] = pid
		scope[id] = b
		protected[id] = rules == "*"
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	allowed := func(id int64) bool {
		if assigned[id] || protected[id] || !business[scope[id]] {
			return false
		}
		seen := map[int64]bool{}
		for id > 0 && !seen[id] {
			seen[id] = true
			id = parents[id]
			if assigned[id] {
				return true
			}
		}
		return false
	}
	if role {
		if allowed(target) {
			return nil
		}
		return ErrDenied
	}
	rows, e = s.DB.QueryContext(ctx, `SELECT role_id FROM `+s.Prefix+`auth_role_access WHERE uid=?`, target)
	if e != nil {
		return e
	}
	defer rows.Close()
	has := false
	for rows.Next() {
		var id int64
		if e = rows.Scan(&id); e != nil {
			return e
		}
		if !allowed(id) {
			return ErrDenied
		}
		has = true
	}
	if e = rows.Err(); e != nil {
		return e
	}
	if !has {
		return ErrDenied
	}
	return nil
}

func ParseID(v string) (int64, error) {
	n, e := strconv.ParseInt(v, 10, 64)
	if e != nil || n <= 0 {
		return 0, ErrInvalid
	}
	return n, nil
}
