package imbridge

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// This is a request-local view, never a cache. It applies the same account,
// credential fingerprint, HRM and active-role checks as eligible(), including
// the existing JSON/CSV button parsing and exact super-role semantics.
type directoryLiveProfile struct {
	p                  Projection
	mobile, hostMobile string
	business           int64
}

type directoryLiveCheck func(context.Context, []int64) (map[int64]directoryLiveProfile, error)

func (s *SQLStore) directoryLiveBatch(ctx context.Context, uids []int64) (map[int64]directoryLiveProfile, error) {
	unique := map[int64]bool{}
	for _, uid := range uids {
		if uid > 0 {
			unique[uid] = true
		}
	}
	ids := make([]int64, 0, len(unique))
	for uid := range unique {
		ids = append(ids, uid)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := map[int64]directoryLiveProfile{}
	if len(ids) == 0 {
		return out, nil
	}
	const chunkSize = 400 // Also stays below SQLite's parameter bound in tests.
	for start := 0; start < len(ids); start += chunkSize {
		chunk := ids[start:min(start+chunkSize, len(ids))]
		marks, args := directoryUIDArgs(chunk)
		args = append(args, time.Now().Unix())
		rows, err := s.DB.QueryContext(ctx, `SELECT b.uid,b.employee_id,b.projection_json,COALESCE(a.password,''),COALESCE(a.salt,''),COALESCE(e.mobile,''),COALESCE(a.mobile,''),COALESCE(d.business_id,0) FROM biz_openim_employee b JOIN `+s.Prefix+`admin a ON a.id=b.uid JOIN biz_hrm_employee e ON e.id=b.employee_id AND e.user_id=b.uid LEFT JOIN `+s.Prefix+`auth_dept d ON d.id=e.dept_id LEFT JOIN `+s.Prefix+`account_security sec ON sec.uid=a.id WHERE b.uid IN (`+marks+`) AND b.enabled=1 AND b.applied_version=b.desired_version AND b.status='ready' AND a.status=0 AND a.deletetime IS NULL AND e.status='active' AND COALESCE(sec.must_change,0)=0 AND COALESCE(sec.locked_until,0)<=?`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var uid int64
			var employeeID, raw, password, salt string
			var live directoryLiveProfile
			if err = rows.Scan(&uid, &employeeID, &raw, &password, &salt, &live.mobile, &live.hostMobile, &live.business); err != nil {
				rows.Close()
				return nil, err
			}
			if json.Unmarshal([]byte(raw), &live.p) != nil || live.p.EmployeeID != employeeID || live.p.HostUID != uid || digest(password+":"+salt) != live.p.AuthFingerprint {
				continue
			}
			out[uid] = live
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		return out, nil
	}
	// Load the three exact active button permissions once, rather than asking
	// the database for the same rule records for every employee and permission.
	rules, err := s.DB.QueryContext(ctx, `SELECT id,permission FROM `+s.Prefix+`auth_rule WHERE status=0 AND type=2 AND permission IN (?,?,?)`, "runtime.plugin.biz-suite.admin", "biz-suite.im.read", "biz-suite.im.write")
	if err != nil {
		return nil, err
	}
	bits := map[string]uint8{"runtime.plugin.biz-suite.admin": 1, "biz-suite.im.read": 2, "biz-suite.im.write": 4}
	ruleBits := map[int64]uint8{}
	for rules.Next() {
		var id int64
		var permission string
		if err = rules.Scan(&id, &permission); err != nil {
			rules.Close()
			return nil, err
		}
		ruleBits[id] = bits[permission]
	}
	err = rules.Err()
	rules.Close()
	if err != nil {
		return nil, err
	}
	allowed := map[int64]uint8{}
	for start := 0; start < len(ids); start += chunkSize {
		marks, args := directoryUIDArgs(ids[start:min(start+chunkSize, len(ids))])
		roles, err := s.DB.QueryContext(ctx, `SELECT x.uid,COALESCE(r.rules,''),COALESCE(r.btns,'') FROM `+s.Prefix+`auth_role r JOIN `+s.Prefix+`auth_role_access x ON x.role_id=r.id WHERE x.uid IN (`+marks+`) AND r.status=0`, args...)
		if err != nil {
			return nil, err
		}
		for roles.Next() {
			var uid int64
			var roleRules, buttons string
			if err = roles.Scan(&uid, &roleRules, &buttons); err != nil {
				roles.Close()
				return nil, err
			}
			if roleRules == "*" {
				allowed[uid] = 7
			}
			for _, id := range parseIDs(buttons) {
				allowed[uid] |= ruleBits[id]
			}
		}
		err = roles.Err()
		roles.Close()
		if err != nil {
			return nil, err
		}
	}
	for uid := range out {
		if allowed[uid] != 7 {
			delete(out, uid)
		}
	}
	return out, nil
}

func directoryUIDArgs(ids []int64) (string, []any) {
	marks := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		marks[i], args[i] = "?", id
	}
	return strings.Join(marks, ","), args
}
