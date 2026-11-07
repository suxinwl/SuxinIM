package imbridge

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"time"
)

var ErrConflict = errors.New("配置已被其他管理员修改，请刷新后重试")
var ErrInvalid = errors.New("配置参数无效")

type accessContextKey struct{}

// WithClientIP carries an already verified transport address. Headers supplied
// by an Internet client are never authoritative for this value.
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, accessContextKey{}, strings.TrimSpace(ip))
}

func requestClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return ""
	}
	peer = peer.Unmap()
	// Nginx connects to HRM over this exact local address and overwrites this
	// single-valued header. X-Forwarded-For and X-Real-IP are deliberately ignored.
	if peer == netip.MustParseAddr("127.0.0.1") {
		values := r.Header.Values("X-GoSuxin-Client-IP")
		if len(values) == 1 {
			ip, e := netip.ParseAddr(strings.TrimSpace(values[0]))
			if e == nil && ip.Zone() == "" {
				return ip.Unmap().String()
			}
		}
	}
	return peer.String()
}

func requestAccessContext(ctx context.Context, r *http.Request) context.Context {
	return WithClientIP(ctx, requestClientIP(r))
}

func defaultLANCIDRs() []string {
	return []string{"192.168.6.0/24", "192.168.10.0/24", "192.168.14.0/24", "192.168.17.0/24", "192.168.18.0/24", "192.168.19.0/24", "192.168.26.0/24", "192.168.27.0/24", "192.168.28.0/24", "192.168.29.0/24", "192.168.30.0/24", "192.168.31.0/24", "192.168.32.0/24"}
}

type NetworkPolicy struct {
	Revision    int64    `json:"revision"`
	CIDRs       []string `json:"cidrs"`
	CurrentIP   string   `json:"currentIP,omitempty"`
	CurrentZone string   `json:"currentZone,omitempty"`
	UpdatedAt   int64    `json:"updatedAt"`
	UpdatedBy   int64    `json:"updatedBy"`
}

type EmployeeAccess struct {
	EmployeeID            string `json:"employeeID"`
	AllowLAN              bool   `json:"allowLAN"`
	AllowPC               bool   `json:"allowPC"`
	AllowMobile           bool   `json:"allowMobile"`
	AllowInternet         bool   `json:"allowInternet"`
	AllowInternetDownload bool   `json:"allowInternetDownload"`
	Revision              int64  `json:"revision"`
	UpdatedAt             int64  `json:"updatedAt"`
	UpdatedBy             int64  `json:"updatedBy"`
}

type EmployeeAccessRow struct {
	EmployeeAccess
	UID            int64  `json:"uid"`
	UserID         string `json:"userID"`
	Account        string `json:"account"`
	Name           string `json:"name"`
	DepartmentName string `json:"departmentName"`
	Eligible       bool   `json:"eligible"`
	Reason         string `json:"reason"`
}

type AccessDecision struct {
	Allowed         bool   `json:"allowed"`
	Zone            string `json:"zone"`
	PolicyRevision  int64  `json:"policyRevision"`
	NetworkRevision int64  `json:"networkRevision"`
}

func (s *SQLStore) ensureAccessSchema(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS biz_openim_access_network (singleton INTEGER PRIMARY KEY, revision BIGINT NOT NULL, cidrs_json TEXT NOT NULL, updated_at BIGINT NOT NULL, updated_by BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_openim_employee_access (employee_id VARCHAR(64) PRIMARY KEY, allow_lan INTEGER NOT NULL, allow_pc INTEGER NOT NULL, allow_mobile INTEGER NOT NULL, allow_internet INTEGER NOT NULL, allow_internet_download INTEGER NOT NULL, revision BIGINT NOT NULL, updated_at BIGINT NOT NULL, updated_by BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_openim_access_audit (id VARCHAR(64) PRIMARY KEY, kind VARCHAR(32) NOT NULL, employee_id VARCHAR(64) NOT NULL, actor_uid BIGINT NOT NULL, client_ip VARCHAR(64) NOT NULL, before_json TEXT NOT NULL, after_json TEXT NOT NULL, created_at BIGINT NOT NULL)`,
	} {
		if _, err := s.DB.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	raw, _ := json.Marshal(defaultLANCIDRs())
	_, err := s.DB.ExecContext(ctx, `INSERT INTO biz_openim_access_network(singleton,revision,cidrs_json,updated_at,updated_by) SELECT 1,1,?,0,0 WHERE NOT EXISTS(SELECT 1 FROM biz_openim_access_network WHERE singleton=1)`, string(raw))
	return err
}

func readNetworkPolicy(ctx context.Context, q sqlQuery) (NetworkPolicy, error) {
	var v NetworkPolicy
	var raw string
	err := q.QueryRowContext(ctx, `SELECT revision,cidrs_json,updated_at,updated_by FROM biz_openim_access_network WHERE singleton=1`).Scan(&v.Revision, &raw, &v.UpdatedAt, &v.UpdatedBy)
	if err != nil {
		return v, err
	}
	if json.Unmarshal([]byte(raw), &v.CIDRs) != nil || len(v.CIDRs) == 0 {
		return v, ErrDenied
	}
	return v, nil
}

func (s *SQLStore) NetworkPolicy(ctx context.Context) (NetworkPolicy, error) {
	v, err := readNetworkPolicy(ctx, s.DB)
	if err == nil {
		v.CurrentIP, _ = ctx.Value(accessContextKey{}).(string)
		v.CurrentZone = networkZone(v.CurrentIP, v.CIDRs)
	}
	return v, err
}

func readEmployeeAccess(ctx context.Context, q sqlQuery, employeeID string) (EmployeeAccess, error) {
	v := EmployeeAccess{EmployeeID: employeeID, AllowLAN: true, AllowPC: true}
	var lan, pc, mobile, internet, download int
	err := q.QueryRowContext(ctx, `SELECT allow_lan,allow_pc,allow_mobile,allow_internet,allow_internet_download,revision,updated_at,updated_by FROM biz_openim_employee_access WHERE employee_id=?`, employeeID).Scan(&lan, &pc, &mobile, &internet, &download, &v.Revision, &v.UpdatedAt, &v.UpdatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return v, nil
	}
	if err != nil {
		return v, err
	}
	v.AllowLAN, v.AllowPC, v.AllowMobile, v.AllowInternet, v.AllowInternetDownload = lan == 1, pc == 1, mobile == 1, internet == 1, download == 1
	return v, nil
}

func networkZone(ip string, cidrs []string) string {
	a, err := netip.ParseAddr(ip)
	if err != nil || a.Zone() != "" {
		return "invalid"
	}
	a = a.Unmap()
	for _, raw := range cidrs {
		if p, e := netip.ParsePrefix(raw); e == nil && p.Contains(a) {
			return "lan"
		}
	}
	return "internet"
}

func canonicalLANCIDRs(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > 64 {
		return nil, ErrInvalid
	}
	private := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("fc00::/7")}
	seen := map[string]bool{}
	out := []string{}
	for _, raw := range values {
		p, err := netip.ParsePrefix(strings.TrimSpace(raw))
		if err != nil || p.Addr().Is4In6() || p.Addr().Zone() != "" {
			return nil, ErrInvalid
		}
		p = p.Masked()
		valid := false
		for _, r := range private {
			if r.Addr().BitLen() == p.Addr().BitLen() && p.Bits() >= r.Bits() && r.Contains(p.Addr()) {
				valid = true
			}
		}
		if !valid {
			return nil, ErrInvalid
		}
		if !seen[p.String()] {
			out = append(out, p.String())
			seen[p.String()] = true
		}
	}
	sort.Strings(out)
	return out, nil
}

func writeAccessAudit(ctx context.Context, q sqlQuery, kind, employeeID string, actor int64, before, after any) error {
	id, err := token()
	if err != nil {
		return err
	}
	b, err := json.Marshal(before)
	if err != nil {
		return err
	}
	a, err := json.Marshal(after)
	if err != nil {
		return err
	}
	ip, _ := ctx.Value(accessContextKey{}).(string)
	_, err = q.ExecContext(ctx, `INSERT INTO biz_openim_access_audit(id,kind,employee_id,actor_uid,client_ip,before_json,after_json,created_at) VALUES(?,?,?,?,?,?,?,?)`, id, kind, employeeID, actor, ip, string(b), string(a), time.Now().Unix())
	return err
}

func (s *SQLStore) SaveNetworkPolicy(ctx context.Context, cidrs []string, expected, actor int64) (NetworkPolicy, error) {
	if actor <= 0 {
		return NetworkPolicy{}, ErrDenied
	}
	values, err := canonicalLANCIDRs(cidrs)
	if err != nil {
		return NetworkPolicy{}, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return NetworkPolicy{}, err
	}
	defer tx.Rollback()
	old, err := readNetworkPolicy(ctx, tx)
	if err != nil {
		return old, err
	}
	if old.Revision != expected {
		return old, ErrConflict
	}
	next := NetworkPolicy{Revision: expected + 1, CIDRs: values, UpdatedAt: time.Now().Unix(), UpdatedBy: actor}
	raw, _ := json.Marshal(values)
	result, err := tx.ExecContext(ctx, `UPDATE biz_openim_access_network SET revision=?,cidrs_json=?,updated_at=?,updated_by=? WHERE singleton=1 AND revision=?`, next.Revision, string(raw), next.UpdatedAt, actor, expected)
	if err != nil {
		return next, err
	}
	if n, e := result.RowsAffected(); e != nil || n != 1 {
		return next, ErrConflict
	}
	if err = writeAccessAudit(ctx, tx, "networks", "", actor, old, next); err != nil {
		return next, err
	}
	if err = tx.Commit(); err != nil {
		return next, err
	}
	return s.NetworkPolicy(ctx)
}

func (s *SQLStore) EmployeeAccessList(ctx context.Context) ([]EmployeeAccessRow, error) {
	employees, err := s.employees(ctx, s.DB)
	if err != nil {
		return nil, err
	}
	out := make([]EmployeeAccessRow, 0, len(employees))
	for _, e := range employees {
		p, err := readEmployeeAccess(ctx, s.DB, e.ID)
		if err != nil {
			return nil, err
		}
		row := EmployeeAccessRow{EmployeeAccess: p, UID: e.UID, UserID: UserID(e.ID), Account: e.No, Name: e.Name, DepartmentName: e.DeptName}
		if e.UID > 0 {
			_, err = s.Eligible(ctx, e.UID)
			row.Eligible = err == nil
		}
		if !row.Eligible {
			row.Reason = "员工未就绪、停用或缺少通讯权限"
		}
		out = append(out, row)
	}
	return out, nil
}

// SaveEmployeeAccess commits policy, audit and epoch/outbox revocation in one
// transaction. Thus a restart cannot leave a tightened grant with valid tokens.
func (s *Service) SaveEmployeeAccess(ctx context.Context, next EmployeeAccess, expected, actor int64) (EmployeeAccess, error) {
	if actor <= 0 || next.EmployeeID == "" || len(next.EmployeeID) > 64 || expected < 0 || next.AllowInternetDownload && !next.AllowInternet {
		return next, ErrInvalid
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return next, err
	}
	defer tx.Rollback()
	var uid int64
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(user_id,0) FROM biz_hrm_employee WHERE id=?`, next.EmployeeID).Scan(&uid)
	if errors.Is(err, sql.ErrNoRows) {
		return next, ErrInvalid
	}
	if err != nil {
		return next, err
	}
	old, err := readEmployeeAccess(ctx, tx, next.EmployeeID)
	if err != nil {
		return next, err
	}
	if old.Revision != expected {
		return next, ErrConflict
	}
	next.Revision, next.UpdatedAt, next.UpdatedBy = expected+1, time.Now().Unix(), actor
	args := []any{boolInt(next.AllowLAN), boolInt(next.AllowPC), boolInt(next.AllowMobile), boolInt(next.AllowInternet), boolInt(next.AllowInternetDownload), next.Revision, next.UpdatedAt, actor, next.EmployeeID}
	if expected == 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO biz_openim_employee_access(allow_lan,allow_pc,allow_mobile,allow_internet,allow_internet_download,revision,updated_at,updated_by,employee_id) VALUES(?,?,?,?,?,?,?,?,?)`, args...)
		if err != nil {
			var count int
			if e := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM biz_openim_employee_access WHERE employee_id=?`, next.EmployeeID).Scan(&count); e == nil && count > 0 {
				return next, ErrConflict
			}
			return next, err
		}
	} else {
		args = append(args, expected)
		var result sql.Result
		result, err = tx.ExecContext(ctx, `UPDATE biz_openim_employee_access SET allow_lan=?,allow_pc=?,allow_mobile=?,allow_internet=?,allow_internet_download=?,revision=?,updated_at=?,updated_by=? WHERE employee_id=? AND revision=?`, args...)
		if err != nil {
			return next, err
		}
		if n, e := result.RowsAffected(); e != nil || n != 1 {
			return next, ErrConflict
		}
	}
	if err = writeAccessAudit(ctx, tx, "employee", next.EmployeeID, actor, old, next); err != nil {
		return next, err
	}
	tightened := old.AllowLAN && !next.AllowLAN || old.AllowPC && !next.AllowPC || old.AllowMobile && !next.AllowMobile || old.AllowInternet && !next.AllowInternet || old.AllowInternetDownload && !next.AllowInternetDownload
	var revoke *Revoke
	if tightened && uid > 0 {
		ep, e := epoch(ctx, tx, uid)
		if e != nil {
			return next, e
		}
		ep++
		if _, err = tx.ExecContext(ctx, `UPDATE biz_openim_epoch SET epoch=? WHERE uid=?`, ep, uid); err != nil {
			return next, err
		}
		r := Revoke{UserID: UserID(next.EmployeeID), All: true, Epoch: ep}
		revoke = &r
		id, e := token()
		if e != nil {
			return next, e
		}
		raw, _ := json.Marshal(r)
		if _, err = tx.ExecContext(ctx, `INSERT INTO biz_openim_outbox(id,kind,aggregate_id,version,payload,status,attempts,next_attempt,last_error,created_at,updated_at) VALUES(?,'revoke',?,0,?,'pending',0,0,'',?,?)`, "access:"+id[:32], r.UserID, string(raw), next.UpdatedAt, next.UpdatedAt); err != nil {
			return next, err
		}
	}
	if err = tx.Commit(); err != nil {
		return next, err
	}
	// Delivery is also durably queued; failed direct notification is retried by
	// the existing one-second worker, while fresh policy checks already deny.
	if revoke != nil {
		_ = s.Remote.Revoke(ctx, *revoke)
	}
	return next, nil
}

func (s *SQLStore) CheckAccess(ctx context.Context, uid int64, platform int, clientIP, action string) (AccessDecision, error) {
	var result AccessDecision
	if platform != 2 && platform != 3 && platform != 5 {
		return result, ErrDenied
	}
	if action != "connect" && action != "download" && action != "upload" {
		return result, ErrDenied
	}
	profile, err := s.Eligible(ctx, uid)
	if err != nil {
		return result, ErrDenied
	}
	policy, err := readEmployeeAccess(ctx, s.DB, profile.EmployeeID)
	if err != nil {
		return result, err
	}
	network, err := readNetworkPolicy(ctx, s.DB)
	if err != nil {
		return result, err
	}
	result.Zone, result.PolicyRevision, result.NetworkRevision = networkZone(clientIP, network.CIDRs), policy.Revision, network.Revision
	if platform == 2 && !policy.AllowMobile || (platform == 3 || platform == 5) && !policy.AllowPC {
		return result, ErrDenied
	}
	switch result.Zone {
	case "lan":
		if !policy.AllowLAN {
			return result, ErrDenied
		}
	case "internet":
		if !policy.AllowInternet || action == "download" && !policy.AllowInternetDownload {
			return result, ErrDenied
		}
	default:
		return result, ErrDenied
	}
	result.Allowed = true
	return result, nil
}

func (s *Service) checkSessionAccess(ctx context.Context, uid int64, platform int) error {
	ip, _ := ctx.Value(accessContextKey{}).(string)
	_, err := s.Store.CheckAccess(ctx, uid, platform, ip, "connect")
	return err
}
