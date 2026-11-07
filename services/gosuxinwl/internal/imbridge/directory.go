package imbridge

import (
	"context"
	"database/sql"
	"errors"
	"encoding/json"
	"sort"
	"strings"
	"unicode"
)

type DirectoryPerson struct {
	UserID        string `json:"userID"`
	Nickname      string `json:"nickname"`
	Name          string `json:"name"`
	EmployeeNo    string `json:"employeeNo"`
	Department    string `json:"department"`
	DepartmentID  string `json:"departmentID"`
	Position      string `json:"position"`
	FaceURL       string `json:"faceURL"`
	CanDirectChat bool   `json:"canDirectChat"`
}

func directoryPerson(ctx context.Context, p Projection, departmentID string, canChat bool) DirectoryPerson {
	profile := profileForEndpoint(ctx, p.Profile)
	return DirectoryPerson{UserID: p.UserID, Nickname: p.Nickname, Name: p.Nickname, EmployeeNo: p.Account,
		Department: p.DepartmentName, DepartmentID: departmentID, Position: p.Profile.Position, FaceURL: profile.FaceURL, CanDirectChat: canChat}
}

func (s *SQLStore) directoryDepartmentsForScope(ctx context.Context, scope *int64) ([]DirectoryDepartment, error) {
	query := `SELECT id,COALESCE(pid,0),name,COALESCE(im_group_enabled,0),COALESCE(leader_uid,0) FROM ` + s.Prefix + `auth_dept WHERE status=0 AND deletetime IS NULL`
	args := []any{}
	if scope != nil {
		query += ` AND COALESCE(business_id,0)=?`
		args = append(args, *scope)
	}
	query += ` ORDER BY weigh DESC,id`
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	type entry struct {
		dep    DirectoryDepartment
		leader int64
	}
	entries := []entry{}
	for rows.Next() {
		var id, parent, leader int64
		var name string
		var enabled int
		if err = rows.Scan(&id, &parent, &name, &enabled, &leader); err != nil {
			rows.Close()
			return nil, err
		}
		entries = append(entries, entry{DirectoryDepartment{ID: intString(id), ParentID: intString(parent), Name: name, GroupEnabled: enabled == 1}, leader})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []DirectoryDepartment{}
	known := map[string]bool{}
	for _, v := range entries {
		known[v.dep.ID] = true
	}
	for _, v := range entries {
		if v.leader > 0 {
			if p, e := s.Eligible(ctx, v.leader); e == nil {
				if business, e := s.organizationScope(ctx, s.DB, p); e == nil && (scope == nil || business == *scope) {
					v.dep.LeaderUserID = p.UserID
					v.dep.LeaderName = p.Nickname
				}
			}
		}
		if !known[v.dep.ParentID] {
			v.dep.ParentID = "0"
		}
		out = append(out, v.dep)
	}
	return out, nil
}

func (s *SQLStore) DirectoryForUID(ctx context.Context, uid int64, keyword, departmentID string, page, size int) (any, error) {
	viewer, err := s.Eligible(ctx, uid)
	if err != nil {
		return nil, ErrDenied
	}
	return s.directory(ctx, &viewer, keyword, departmentID, page, size)
}

func digits(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, value)
}

func matchesDirectory(p Projection, keyword, mobile, hostMobile string) bool {
	if containsDirectory(p, keyword) {
		return true
	}
	needle := digits(keyword)
	return len(needle) >= 3 && (strings.Contains(digits(mobile), needle) || strings.Contains(digits(hostMobile), needle))
}

func (s *SQLStore) directory(ctx context.Context, viewer *Projection, keyword, departmentID string, page, size int) (any, error) {
	return s.directoryWithLiveBatch(ctx, viewer, keyword, departmentID, page, size, s.directoryLiveBatch)
}

func (s *SQLStore) directoryWithEligibility(ctx context.Context, viewer *Projection, keyword, departmentID string, page, size int, eligible func(context.Context, int64) (Projection, error)) (any, error) {
	// Preserve the focused prefilter/race-test seam. Production uses the batch
	// implementation above and never runs per-employee SQL through this adapter.
	return s.directoryWithLiveBatch(ctx, viewer, keyword, departmentID, page, size, func(ctx context.Context, uids []int64) (map[int64]directoryLiveProfile, error) {
		live := map[int64]directoryLiveProfile{}
		for _, uid := range uids {
			check := eligible
			if viewer != nil && uid == viewer.HostUID {
				check = s.Eligible
			}
			p, err := check(ctx, uid)
			if err != nil {
				continue
			}
			business, err := s.organizationScope(ctx, s.DB, p)
			if err != nil {
				continue
			}
			v := directoryLiveProfile{p: p, business: business}
			err = s.DB.QueryRowContext(ctx, `SELECT COALESCE(e.mobile,''),COALESCE(a.mobile,'') FROM biz_hrm_employee e LEFT JOIN `+s.Prefix+`admin a ON a.id=e.user_id WHERE e.id=? AND e.user_id=?`, p.EmployeeID, uid).Scan(&v.mobile, &v.hostMobile)
			if err == nil {
				live[uid] = v
			}
		}
		return live, nil
	})
}

func (s *SQLStore) directoryWithLiveBatch(ctx context.Context, viewer *Projection, keyword, departmentID string, page, size int, check directoryLiveCheck) (any, error) {
	if page < 1 {
		page = 1
	}
	if page > 1000000 {
		page = 1000000
	}
	if size < 1 {
		size = 50
	}
	if size > 100 {
		size = 100
	}
	var scope *int64
	if viewer != nil {
		business, err := s.organizationScope(ctx, s.DB, *viewer)
		if err != nil {
			return nil, ErrDenied
		}
		scope = &business
	}
	departments, err := s.directoryDepartmentsForScope(ctx, scope)
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, d := range departments {
		known[d.ID] = true
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT b.projection_json,COALESCE(e.mobile,''),COALESCE(a.mobile,''),COALESCE(d.business_id,0) FROM biz_openim_employee b JOIN biz_hrm_employee e ON e.id=b.employee_id LEFT JOIN `+s.Prefix+`admin a ON a.id=b.uid LEFT JOIN `+s.Prefix+`auth_dept d ON d.id=e.dept_id WHERE b.enabled=1 AND b.applied_version=b.desired_version AND b.status='ready' ORDER BY b.employee_id`)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		p                  Projection
		mobile, hostMobile string
		business           int64
	}
	candidates := []candidate{}
	for rows.Next() {
		var raw string
		var v candidate
		if err = rows.Scan(&raw, &v.mobile, &v.hostMobile, &v.business); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &v.p); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	selected := []candidate{}
	uids := []int64{}
	for _, v := range candidates {
		// Ready projections only preselect candidates. Unrelated employees do
		// not need multiple live RBAC/password lookups for each exact search.
		if scope != nil && v.business != *scope {
			continue
		}
		dept := v.p.DepartmentID
		if !known[dept] {
			dept = "unassigned"
		}
		if departmentID != "" && departmentID != dept || keyword != "" && !matchesDirectory(v.p, keyword, v.mobile, v.hostMobile) {
			continue
		}
		selected = append(selected, v)
		uids = append(uids, v.p.HostUID)
	}
	if viewer != nil {
		uids = append(uids, viewer.HostUID)
	}
	first, err := check(ctx, uids)
	if err != nil {
		return nil, err
	}
	viewerCurrent := func(live map[int64]directoryLiveProfile) bool {
		if viewer == nil {
			return true
		}
		v, ok := live[viewer.HostUID]
		return ok && v.p.UserID == viewer.UserID && scope != nil && v.business == *scope
	}
	if !viewerCurrent(first) {
		return nil, ErrDenied
	}
	match := func(v directoryLiveProfile) (string, bool) {
		if scope != nil && v.business != *scope {
			return "", false
		}
		dept := v.p.DepartmentID
		if !known[dept] {
			dept = "unassigned"
		}
		return dept, (departmentID == "" || departmentID == dept) && (keyword == "" || matchesDirectory(v.p, keyword, v.mobile, v.hostMobile))
	}
	// Re-read the surviving set before computing the total and slicing a page.
	// Both passes use current data, without a process/session lifetime cache.
	// A concurrent disable, password/RBAC or business change cannot survive by
	// relying on the ready projection selected at the start of the request.
	matched := []int64{}
	for _, v := range selected {
		if current, ok := first[v.p.HostUID]; ok {
			if _, ok = match(current); ok {
				matched = append(matched, v.p.HostUID)
			}
		}
	}
	finalUIDs := append([]int64{}, matched...)
	if viewer != nil {
		finalUIDs = append(finalUIDs, viewer.HostUID)
	}
	last, err := check(ctx, finalUIDs)
	if err != nil {
		return nil, err
	}
	if !viewerCurrent(last) {
		return nil, ErrDenied
	}
	list := []DirectoryPerson{}
	unassigned := false
	for _, uid := range matched {
		if current, ok := last[uid]; ok {
			if dept, ok := match(current); ok {
				unassigned = unassigned || dept == "unassigned"
				list = append(list, directoryPerson(ctx, current.p, dept, viewer != nil && viewer.UserID != current.p.UserID))
			}
		}
	}
	if unassigned {
		departments = append(departments, DirectoryDepartment{ID: "unassigned", ParentID: "0", Name: "未分配部门"})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].EmployeeNo < list[j].EmployeeNo })
	total := len(list)
	start := min((page-1)*size, total)
	end := min(start+size, total)
	return map[string]any{"items": list[start:end], "total": total, "departments": departments}, nil
}

func (s *SQLStore) DirectChatPerson(ctx context.Context, uid int64, peer string) (DirectoryPerson, error) {
	if len(peer) > 64 || peer == "" {
		return DirectoryPerson{}, ErrDenied
	}
	sender, err := s.Eligible(ctx, uid)
	if err != nil {
		return DirectoryPerson{}, err
	}
	var targetUID int64
	if err = s.DB.QueryRowContext(ctx, `SELECT uid FROM biz_openim_employee WHERE openim_user_id=? AND uid>0`, peer).Scan(&targetUID); err != nil {
		if errors.Is(err,sql.ErrNoRows){return DirectoryPerson{},ErrDenied};return DirectoryPerson{},fault("peer.target",err)
	}
	target, err := s.Eligible(ctx, targetUID)
	if err!=nil{return DirectoryPerson{},err}
	if target.UserID != peer {
		return DirectoryPerson{}, ErrDenied
	}
	left, e := s.organizationScope(ctx, s.DB, sender)
	if e != nil {
		return DirectoryPerson{}, e
	}
	right, e := s.organizationScope(ctx, s.DB, target)
	if e!=nil{return DirectoryPerson{},e}
	if left != right {
		return DirectoryPerson{}, ErrDenied
	}
	departmentID := target.DepartmentID
	var known int
	if err = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+s.Prefix+`auth_dept WHERE id=? AND status=0 AND deletetime IS NULL AND COALESCE(business_id,0)=?`, departmentID, left).Scan(&known); err != nil {
		return DirectoryPerson{},fault("peer.department",err)
	}
	if known == 0 {
		departmentID = "unassigned"
	}
	return directoryPerson(ctx, target, departmentID, sender.UserID != target.UserID), nil
}
