package wecom

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/hrm"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/organization"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/wecomprofile"
)

type Department struct {
	ID       int64  `json:"id"`
	ParentID int64  `json:"parentid"`
	Name     string `json:"name"`
	Order    int    `json:"order"`
}
type User struct {
	UserID         string  `json:"userid"`
	Name           string  `json:"name"`
	Department     []int64 `json:"department"`
	MainDepartment int64   `json:"main_department"`
	Position       string  `json:"position"`
	Mobile         string  `json:"mobile"`
	Email          string  `json:"email"`
	Status         int     `json:"status"`
	employeeNo     string
}

// A missing member can mean a reduced application visibility range, not a
// resignation. Keep the imported profile/history and mark eligibility unknown.
const contactScopeUnobserved = "最近一次通讯录同步未观察到该成员，暂停后续同步；请核对应用可见范围或部门归属并重新同步通讯录"

func (c *Client) SyncContact(ctx context.Context) (int, error) {
	var deps struct {
		apiError
		Department []Department `json:"department"`
	}
	if err := c.api(ctx, "contact", http.MethodGet, "/department/list", nil, nil, &deps); err != nil {
		return 0, err
	}
	// A restricted visibility range is not evidence that previously known staff
	// left the company. Never delete or disable people missing from this snapshot.
	seen := map[string]bool{}
	count, failed := 0, 0
	for _, d := range deps.Department {
		if d.ID <= 0 {
			return count, errors.New("invalid department returned by wecom")
		}
		if err := upsert(ctx, "wecom_department", "wecom_id", d.ID, g.Map{"id": stableID("department:" + strconv.FormatInt(d.ID, 10)), "wecom_id": d.ID, "parent_id": d.ParentID, "name": d.Name, "order_no": d.Order, "raw_json": mustJSON(d), "synced_at": now()}); err != nil {
			return count, err
		}
	}
	filter, err := c.filter(ctx)
	if err != nil {
		return count, err
	}
	// Read allowed departments first. If an existing directory member is no
	// longer there, also check the remaining visible departments for a move into
	// an excluded subtree. Refreshing their scope prevents later jobs from using
	// the previous allowed department; it must not import other excluded staff.
	knownRows, err := core.Model("wecom_user").Ctx(ctx).Fields("wecom_userid").All()
	if err != nil {
		return count, dbError("contact known members")
	}
	unseenKnown := map[string]bool{}
	for _, row := range knownRows {
		if uid := row["wecom_userid"].String(); uid != "" {
			unseenKnown[uid] = true
		}
	}
	orderedDepartments := make([]Department, 0, len(deps.Department))
	for _, d := range deps.Department {
		if filter.departmentAllowed(d.ID) {
			orderedDepartments = append(orderedDepartments, d)
		}
	}
	for _, d := range deps.Department {
		if !filter.departmentAllowed(d.ID) {
			orderedDepartments = append(orderedDepartments, d)
		}
	}
	for _, d := range orderedDepartments {
		refreshOnly := !filter.departmentAllowed(d.ID)
		if refreshOnly && len(unseenKnown) == 0 {
			continue
		}
		var users struct {
			apiError
			UserList []jsonRecord `json:"userlist"`
		}
		// fetch_child has been removed from the official member-details API. Fetch
		// every visible department and deduplicate multi-department users explicitly.
		if err := c.api(ctx, "contact", http.MethodGet, "/user/list", map[string]string{"department_id": strconv.FormatInt(d.ID, 10)}, nil, &users); err != nil {
			return count, err
		}
		for _, raw := range users.UserList {
			var u User
			if raw.decode(&u) != nil {
				return count, errors.New("invalid enterprise WeChat member record")
			}
			u.employeeNo = wecomprofile.EmployeeNoFromRaw(string(raw))
			if seen[u.UserID] {
				continue
			}
			seen[u.UserID] = true
			if u.UserID == "" {
				failed++
				continue
			}
			delete(unseenKnown, u.UserID)
			if refreshOnly || !filter.allowsNamed(u.UserID, u.Name, u.Department) {
				// Refresh only an already-known directory member's scope metadata.
				// Otherwise a rename/department move into an exclusion would leave
				// stale names for later checkin and approval jobs. This never inserts
				// a filtered member or modifies their existing HRM history.
				if _, err := core.Model("wecom_user").Ctx(ctx).Where("wecom_userid", u.UserID).Update(g.Map{"name": u.Name, "department_ids": mustJSON(u.Department), "raw_json": string(raw), "synced_at": now()}); err != nil {
					return count, dbError("contact scope metadata")
				}
				if _, err := core.Model("wecom_user").Ctx(ctx).Where("wecom_userid", u.UserID).Where("processing_error", contactScopeUnobserved).Update(g.Map{"processing_error": ""}); err != nil {
					return count, dbError("contact scope confirmation")
				}
				continue
			}
			data := g.Map{"id": stableID("user:" + u.UserID), "wecom_userid": u.UserID, "name": u.Name, "department_ids": mustJSON(u.Department), "position": u.Position, "mobile": u.Mobile, "email": u.Email, "status": u.Status, "raw_json": string(raw), "processing_error": "", "synced_at": now()}
			if err := upsert(ctx, "wecom_user", "wecom_userid", u.UserID, data); err != nil {
				return count, err
			}
			employee, err := mapEmployee(ctx, u)
			if err != nil {
				failed++
				if _, e := core.Model("wecom_user").Ctx(ctx).Where("wecom_userid", u.UserID).Update(g.Map{"processing_error": err.Error()}); e != nil {
					return count, dbError("contact exception")
				}
				continue
			}
			if _, err = core.Model("wecom_user").Ctx(ctx).Where("wecom_userid", u.UserID).Update(g.Map{"employee_id": employee.ID, "user_id": employee.UserID, "processing_error": ""}); err != nil {
				return count, dbError("contact mapping")
			}
			count++
		}
	}
	for uid := range unseenKnown {
		if _, err := core.Model("wecom_user").Ctx(ctx).Where("wecom_userid", uid).Update(g.Map{"processing_error": contactScopeUnobserved}); err != nil {
			return count, dbError("contact scope missing")
		}
	}
	exclusions, err := core.ExplicitExclusionsForRules(ctx, c.Settings().exclusionRules())
	if err != nil {
		return count, err
	}
	if _, err = organization.Prune(ctx, exclusions, false); err != nil {
		return count, err
	}
	return count, partial(failed)
}

func mapEmployee(ctx context.Context, u User) (hrm.Employee, error) {
	var in hrm.Employee
	rows, err := core.Model("biz_hrm_employee").Ctx(ctx).Where("wecom_userid", u.UserID).Limit(2).All()
	if err != nil {
		return in, dbError("employee mapping")
	}
	if len(rows) > 1 {
		return in, errors.New("multiple employees have this enterprise WeChat userid")
	}
	if len(rows) == 0 && u.Mobile != "" {
		rows, err = core.Model("biz_hrm_employee").Ctx(ctx).Where("mobile", u.Mobile).Limit(2).All()
		if err != nil {
			return in, dbError("employee matching")
		}
		if len(rows) > 1 {
			return in, errors.New("mobile matches multiple employees; bind userid manually")
		}
		if len(rows) == 1 && rows[0]["wecom_userid"].String() != "" && rows[0]["wecom_userid"].String() != u.UserID {
			return in, errors.New("mobile belongs to an employee linked to a different enterprise WeChat userid")
		}
	}
	if len(rows) == 1 {
		if err = rows[0].Struct(&in); err != nil {
			return in, dbError("employee mapping")
		}
		// A removed exclusion or a later upstream department move is not approval
		// to restore a deliberately archived employee or its host identity.
		if in.Status == "filtered" {
			return in, errors.New("员工已按同步过滤规则归档，不会自动恢复；请管理员核对并人工恢复员工档案及相关部门、账号")
		}
	}
	if in.ID == "" {
		in.ID = stableID("employee:" + u.UserID)
	}
	in.WeComUserID = u.UserID
	if u.employeeNo != "" {
		in.EmployeeNo = u.employeeNo
	}
	// Missing sensitive profile fields may indicate visibility restrictions. Do
	// not erase an existing local value when WeCom omits it.
	if u.Name != "" {
		in.Name = u.Name
	}
	if in.Name == "" {
		in.Name = u.UserID
	}
	if u.Mobile != "" {
		in.Mobile = u.Mobile
	}
	if u.Email != "" {
		in.Email = u.Email
	}
	if u.Position != "" {
		in.Position = u.Position
	}
	in.Status = userStatus(u.Status)
	primary := u.MainDepartment
	if primary == 0 && len(u.Department) > 0 {
		primary = u.Department[0]
	}
	if primary > 0 {
		dep, e := core.Model("wecom_department").Ctx(ctx).Where("wecom_id", primary).One()
		if e != nil {
			return in, dbError("department mapping")
		}
		if len(dep) > 0 && dep["host_dept_id"].Int64() > 0 {
			in.DeptID = dep["host_dept_id"].Int64()
		}
	}
	id, err := hrm.UpsertEmployee(ctx, in)
	if err != nil {
		return in, errors.New("employee validation or persistence failed; inspect HRM mapping")
	}
	in.ID = id
	return in, nil
}
func userStatus(status int) string {
	switch status {
	case 1:
		return "active"
	case 5:
		return "resigned"
	default:
		return "inactive"
	}
}
