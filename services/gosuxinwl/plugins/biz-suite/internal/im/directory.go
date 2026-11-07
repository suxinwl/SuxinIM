package im

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/wecomprofile"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

type directoryDepartment struct {
	ID       int64  `json:"id"`
	ParentID int64  `json:"parent_id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
}
type directoryPerson struct {
	ID                          *int64  `json:"id"`
	UserID                      *int64  `json:"user_id"`
	EntryID                     string  `json:"entry_id"`
	EmployeeID                  string  `json:"employee_id"`
	Name                        string  `json:"name"`
	EmployeeNo                  string  `json:"employee_no"`
	Avatar                      string  `json:"avatar"`
	Position                    string  `json:"position"`
	DepartmentID                int64   `json:"department_id"`
	DepartmentName              string  `json:"department_name"`
	DepartmentPath              string  `json:"department_path"`
	DepartmentIDs               []int64 `json:"department_ids"`
	DisplayName                 string  `json:"display_name"`
	CanChat                     bool    `json:"can_chat"`
	CanSend                     bool    `json:"can_send"`
	CanRead                     bool    `json:"can_read"`
	CanCall                     bool    `json:"can_call"`
	ReadonlyReason              string  `json:"readonly_reason"`
	UnavailableReason           string  `json:"unavailable_reason"`
	AccountStatus               string  `json:"account_status"`
	BlockedByMe                 bool    `json:"blocked_by_me"`
	active, read, write, listed bool
}
type accountGrant struct{ gate, read, write bool }
type directory struct {
	people      []*directoryPerson
	byUser      map[int64]*directoryPerson
	departments map[int64]directoryDepartment
}

// Resolve the entire role union in four queries, not one permission query per
// employee. Match the host's enabled-role and superRoleAuth semantics.
func accountGrants(ctx context.Context) (map[int64]accountGrant, error) {
	access, err := core.HostModel("auth_role_access").Ctx(ctx).Fields("uid,role_id").All()
	if err != nil {
		return nil, err
	}
	roles, err := core.HostModel("auth_role").Ctx(ctx).Where("status", 0).Fields("id,rules,btns").All()
	if err != nil {
		return nil, err
	}
	rules, err := core.HostModel("auth_rule").Ctx(ctx).Where("status", 0).Where("type", 2).Fields("id,permission,path").All()
	if err != nil {
		return nil, err
	}
	byRule := map[string]accountGrant{}
	all := accountGrant{}
	for _, row := range rules {
		p := row["permission"].String()
		grant := accountGrant{gate: row["path"].String() == "/plugins/biz-suite/admin", read: p == "biz-suite.im.read" || p == "biz-suite.im.manage", write: p == "biz-suite.im.write" || p == "biz-suite.im.manage"}
		byRule[row["id"].String()] = grant
		all.gate, all.read, all.write = all.gate || grant.gate, all.read || grant.read, all.write || grant.write
	}
	checkSuper, err := g.Cfg("app").Get(ctx, "app.superRoleAuth")
	if err != nil {
		return nil, err
	}
	byRole := map[int64]accountGrant{}
	for _, row := range roles {
		grant := accountGrant{}
		if row["rules"].String() == "*" {
			grant = all
			if !checkSuper.Bool() {
				grant = accountGrant{true, true, true}
			}
		} else {
			for _, id := range gf.ArrayMerge([]*gvar.Var{row["btns"]}) {
				x := byRule[fmt.Sprint(id)]
				grant.gate, grant.read, grant.write = grant.gate || x.gate, grant.read || x.read, grant.write || x.write
			}
		}
		byRole[row["id"].Int64()] = grant
	}
	out := map[int64]accountGrant{}
	for _, row := range access {
		id := row["uid"].Int64()
		a, b := out[id], byRole[row["role_id"].Int64()]
		out[id] = accountGrant{a.gate || b.gate, a.read || b.read, a.write || b.write}
	}
	return out, nil
}

func loadDirectory(ctx context.Context) (*directory, error) {
	d := &directory{byUser: map[int64]*directoryPerson{}, departments: map[int64]directoryDepartment{}, people: []*directoryPerson{}}
	exclusions, err := core.LoadExplicitExclusions(ctx)
	if err != nil {
		return nil, err
	}
	wcDeps, err := model(ctx, "wecom_department").Fields("wecom_id,host_dept_id").All()
	if err != nil {
		return nil, err
	}
	wcToHost, hiddenDepartments := map[int64]int64{}, map[int64]bool{}
	for _, row := range wcDeps {
		wid, hid := row["wecom_id"].Int64(), row["host_dept_id"].Int64()
		wcToHost[wid] = hid
		if exclusions.Departments[wid] && hid > 0 {
			hiddenDepartments[hid] = true
		}
	}
	deps, err := core.HostModel("auth_dept").Ctx(ctx).Where("status", 0).WhereNull("deletetime").Fields("id,pid,name").All()
	if err != nil {
		return nil, err
	}
	for _, r := range deps {
		id := r["id"].Int64()
		if hiddenDepartments[id] {
			continue
		}
		d.departments[id] = directoryDepartment{ID: id, ParentID: r["pid"].Int64(), Name: r["name"].String()}
	}
	for id, dep := range d.departments {
		parts := []string{}
		seen := map[int64]bool{}
		for n := id; n > 0 && !seen[n]; {
			seen[n] = true
			v, ok := d.departments[n]
			if !ok {
				break
			}
			parts = append([]string{v.Name}, parts...)
			n = v.ParentID
		}
		dep.Path = strings.Join(parts, " / ")
		d.departments[id] = dep
	}
	grants, err := accountGrants(ctx)
	if err != nil {
		return nil, err
	}
	accounts, err := core.HostModel("admin").Ctx(ctx).Unscoped().Fields("id,name,nickname,username,avatar,dept_id,status,deletetime").All()
	if err != nil {
		return nil, err
	}
	for _, row := range accounts {
		uid := row["id"].Int64()
		grant := grants[uid]
		p := &directoryPerson{ID: &uid, UserID: &uid, EntryID: "user:" + strconv.FormatInt(uid, 10), Name: strings.TrimSpace(row["name"].String()), Avatar: row["avatar"].String(), DepartmentID: row["dept_id"].Int64(), DepartmentIDs: []int64{}, listed: row["deletetime"].IsEmpty()}
		p.active = p.listed && row["status"].Int() == 0
		p.read = grant.gate && grant.read
		p.write = grant.gate && grant.write
		p.AccountStatus = "active"
		if !p.listed {
			p.AccountStatus = "deleted"
			p.UnavailableReason = "账号已删除"
		} else if !p.active {
			p.AccountStatus = "disabled"
			p.UnavailableReason = "账号尚未启用或已停用"
		} else if !p.read {
			p.AccountStatus = "no_permission"
			p.UnavailableReason = "账号尚未开通企业即时通讯"
		}
		d.people = append(d.people, p)
		d.byUser[uid] = p
	}
	wcUsers, err := model(ctx, "wecom_user").Fields("wecom_userid,employee_id,name,department_ids,position,status,raw_json").All()
	if err != nil {
		return nil, err
	}
	type external struct {
		name, position, no string
		departments        []int64
		status             int
	}
	externalByID := map[string]external{}
	for _, r := range wcUsers {
		x := external{name: r["name"].String(), position: r["position"].String(), no: wecomprofile.EmployeeNoFromRaw(r["raw_json"].String()), status: r["status"].Int()}
		var ids []int64
		_ = json.Unmarshal([]byte(r["department_ids"].String()), &ids)
		for _, id := range ids {
			if h := wcToHost[id]; h > 0 {
				x.departments = append(x.departments, h)
			}
		}
		externalByID[r["wecom_userid"].String()] = x
	}
	employees, err := model(ctx, "biz_hrm_employee").Fields("id,user_id,wecom_userid,employee_no,name,dept_id,position,status").All()
	if err != nil {
		return nil, err
	}
	linked := map[string]bool{}
	for _, r := range employees {
		id, uid, wid := r["id"].String(), r["user_id"].Int64(), r["wecom_userid"].String()
		linked[wid] = true
		x, hasExternal := externalByID[wid]
		p := d.byUser[uid]
		if p == nil {
			p = &directoryPerson{EntryID: "employee:" + id, DepartmentIDs: []int64{}, UnavailableReason: "尚未关联本地账号", AccountStatus: "unlinked", listed: true}
			d.people = append(d.people, p)
		}
		p.EmployeeID = id
		p.EmployeeNo = r["employee_no"].String()
		p.Position = r["position"].String()
		p.DepartmentID = r["dept_id"].Int64()
		name := strings.TrimSpace(r["name"].String())
		if name == id || name == wid {
			name = ""
		}
		if name == "" && x.name != wid {
			name = strings.TrimSpace(x.name)
		}
		if name != "" {
			p.Name = name
		}
		if p.EmployeeNo == "" {
			p.EmployeeNo = x.no
		}
		if p.Position == "" {
			p.Position = x.position
		}
		p.DepartmentIDs = append(p.DepartmentIDs, x.departments...)
		if r["status"].String() == "filtered" || exclusions.Users[wid] {
			p.active, p.listed = false, false
			p.UnavailableReason = "员工已按同步过滤规则移除"
			p.AccountStatus = "filtered"
		} else if r["status"].String() != "active" || (hasExternal && x.status != 1) {
			p.active = false
			if r["status"].String() == "resigned" || (hasExternal && x.status == 5) {
				p.listed = false
				p.UnavailableReason = "员工已离职"
				p.AccountStatus = "resigned"
			} else if p.AccountStatus != "deleted" {
				p.UnavailableReason = "员工尚未激活或已停用"
				if p.ID != nil {
					p.AccountStatus = "disabled"
				}
			}
		}
	}
	for wid, x := range externalByID {
		if linked[wid] || x.status == 5 || exclusions.Users[wid] {
			continue
		}
		sum := sha256.Sum256([]byte(wid))
		name := x.name
		if name == wid {
			name = ""
		}
		p := &directoryPerson{EntryID: fmt.Sprintf("directory:%x", sum[:12]), Name: name, EmployeeNo: x.no, Position: x.position, DepartmentIDs: x.departments, UnavailableReason: "尚未关联本地账号", AccountStatus: "unlinked", listed: true}
		if len(x.departments) > 0 {
			p.DepartmentID = x.departments[0]
		}
		d.people = append(d.people, p)
	}
	for _, p := range d.people {
		seen := map[int64]bool{}
		ids := []int64{}
		for _, id := range append(p.DepartmentIDs, p.DepartmentID) {
			if _, ok := d.departments[id]; ok && !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
		p.DepartmentIDs = ids
		dep := d.departments[p.DepartmentID]
		p.DepartmentName, p.DepartmentPath = dep.Name, dep.Path
		p.CanChat = p.active && p.read
		p.Name = strings.TrimSpace(p.Name)
	}
	sort.Slice(d.people, func(i, j int) bool {
		a, b := d.people[i], d.people[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.EmployeeNo != b.EmployeeNo {
			return a.EmployeeNo < b.EmployeeNo
		}
		return a.EntryID < b.EntryID
	})
	return d, nil
}

func (d *directory) personalized(ctx context.Context, viewer int64) error {
	rows, err := model(ctx, "biz_im_friend").Where("user_id = ? OR friend_user_id = ?", viewer, viewer).Fields("user_id,friend_user_id,status,display_name").All()
	if err != nil {
		return err
	}
	blocked := map[int64]bool{}
	own := map[int64]bool{}
	names := map[int64]string{}
	for _, r := range rows {
		a, b := r["user_id"].Int64(), r["friend_user_id"].Int64()
		if a == viewer {
			names[b] = r["display_name"].String()
			own[b] = r["status"].String() == "blocked"
			if own[b] {
				blocked[b] = true
			}
		} else if r["status"].String() == "blocked" {
			blocked[a] = true
		}
	}
	for uid, p := range d.byUser {
		p.DisplayName = names[uid]
		p.BlockedByMe = own[uid]
		if blocked[uid] {
			p.CanChat = false
			p.UnavailableReason = "暂时无法与该同事通信"
		}
		if uid == viewer {
			p.CanChat = false
			p.UnavailableReason = "不能与自己聊天"
		}
		me := d.byUser[viewer]
		if p.CanChat && (me == nil || !me.active || !me.write || !me.read) {
			p.CanChat = false
			p.UnavailableReason = "当前账号没有发送消息权限"
		}
		p.CanSend, p.CanCall = p.CanChat, p.CanChat
		p.CanRead = me != nil && me.read && uid != viewer
		p.ReadonlyReason = p.UnavailableReason
	}
	for _, p := range d.people {
		if p.ID == nil {
			p.ReadonlyReason = p.UnavailableReason
		}
	}
	return nil
}

func bindDirectory(api *ghttp.RouterGroup) {
	api.GET("/directory/departments", func(r *ghttp.Request) {
		d, err := loadDirectory(r.Context())
		if err != nil {
			reply(r, nil, err)
			return
		}
		out := []directoryDepartment{}
		for _, dep := range d.departments {
			out = append(out, dep)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Path != out[j].Path {
				return out[i].Path < out[j].Path
			}
			return out[i].ID < out[j].ID
		})
		reply(r, out, nil)
	})
	api.GET("/directory/users", directoryUsers)
	api.GET("/private/peer", func(r *ghttp.Request) {
		raw := r.GetQuery("receiver_id").String()
		legacy := r.GetQuery("user_id").String()
		if raw == "" {
			raw = legacy
		}
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			reply(r, nil, ErrInvalid)
			return
		}
		if legacy != "" {
			alias, e := strconv.ParseInt(legacy, 10, 64)
			if e != nil || alias != id {
				reply(r, nil, ErrInvalid)
				return
			}
		}
		d, err := loadDirectory(r.Context())
		if err == nil {
			err = d.personalized(r.Context(), core.UID(r.Context()))
		}
		if err != nil {
			reply(r, nil, err)
			return
		}
		p := d.byUser[id]
		if p == nil || !p.listed {
			if !hasPrivateHistory(r.Context(), core.UID(r.Context()), id) {
				reply(r, nil, ErrForbidden)
				return
			}
			if p == nil {
				p = &directoryPerson{ID: &id, UserID: &id, Name: "账号已删除", UnavailableReason: "账号已删除", AccountStatus: "deleted", ReadonlyReason: "账号已删除", CanRead: true, DepartmentIDs: []int64{}}
			}
		}
		reply(r, p, nil)
	})
	api.POST("/contacts/:user_id/action", contactAction)
}
func directoryUsers(r *ghttp.Request) {
	q := strings.TrimSpace(r.GetQuery("q").String())
	if len(q) > 128 {
		reply(r, nil, ErrInvalid)
		return
	}
	page, size := 1, 50
	for key, dest := range map[string]*int{"page": &page, "page_size": &size} {
		if raw := r.GetQuery(key).String(); raw != "" {
			v, e := strconv.Atoi(raw)
			if e != nil || v <= 0 {
				reply(r, nil, ErrInvalid)
				return
			}
			*dest = v
		}
	}
	if size > 100 || page > 1000000 {
		reply(r, nil, ErrInvalid)
		return
	}
	depID := int64(0)
	if raw := r.GetQuery("department_id").String(); raw != "" {
		v, e := strconv.ParseInt(raw, 10, 64)
		if e != nil || v < 0 {
			reply(r, nil, ErrInvalid)
			return
		}
		depID = v
	}
	children := true
	if raw := r.GetQuery("include_subdepartments").String(); raw != "" {
		v, e := strconv.ParseBool(raw)
		if e != nil {
			reply(r, nil, ErrInvalid)
			return
		}
		children = v
	}
	unassigned := false
	if raw := r.GetQuery("unassigned").String(); raw != "" {
		v, e := strconv.ParseBool(raw)
		if e != nil {
			reply(r, nil, ErrInvalid)
			return
		}
		unassigned = v
	}
	if unassigned && depID > 0 {
		reply(r, nil, ErrInvalid)
		return
	}
	d, err := loadDirectory(r.Context())
	if err == nil {
		err = d.personalized(r.Context(), core.UID(r.Context()))
	}
	if err != nil {
		reply(r, nil, err)
		return
	}
	if _, ok := d.departments[depID]; depID > 0 && !ok {
		reply(r, nil, ErrInvalid)
		return
	}
	matches := func(p *directoryPerson) bool {
		if unassigned {
			return len(p.DepartmentIDs) == 0
		}
		if depID == 0 {
			return true
		}
		for _, n := range p.DepartmentIDs {
			seen := map[int64]bool{}
			for n > 0 && !seen[n] {
				if n == depID {
					return true
				}
				if !children {
					break
				}
				seen[n] = true
				n = d.departments[n].ParentID
			}
		}
		return false
	}
	items := []*directoryPerson{}
	query := strings.ToLower(q)
	for _, p := range d.people {
		if p.listed && matches(p) && (q == "" || strings.Contains(strings.ToLower(p.Name), query) || strings.Contains(strings.ToLower(p.EmployeeNo), query)) {
			items = append(items, p)
		}
	}
	total := len(items)
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	reply(r, g.Map{"items": items[start:end], "total": total, "page": page, "page_size": size}, nil)
}
