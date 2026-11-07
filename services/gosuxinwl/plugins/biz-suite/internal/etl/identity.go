package etl

import (
	"errors"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/hrm"
)

func (r *runner) findEmployee(identity Identity) (hrm.Employee, bool, error) {
	keys := []struct{ field, value string }{{"wecom_userid", identity.WeComUserID}, {"employee_no", identity.EmployeeNo}, {"mobile", identity.Mobile}}
	for _, key := range keys {
		if key.value == "" {
			continue
		}
		rows, err := core.Model("biz_hrm_employee").Ctx(r.ctx).Where(key.field, key.value).Limit(2).All()
		if err != nil {
			return hrm.Employee{}, false, databaseError()
		}
		found := map[string]hrm.Employee{}
		for _, row := range rows {
			var employee hrm.Employee
			if row.Struct(&employee) != nil {
				return employee, false, databaseError()
			}
			found[employee.ID] = employee
		}
		// Newly planned employees exist in memory during dry run. This lets their
		// attendance and IM mappings be checked without inserting temporary rows.
		for _, employee := range r.employees {
			value := ""
			switch key.field {
			case "wecom_userid":
				value = employee.WeComUserID
			case "employee_no":
				value = employee.EmployeeNo
			case "mobile":
				value = employee.Mobile
			}
			if value == key.value {
				found[employee.ID] = employee
			}
		}
		if len(found) > 1 {
			return hrm.Employee{}, false, errors.New("identity matches multiple employees; provide an exact userid/employee number")
		}
		for _, employee := range found {
			if identity.WeComUserID != "" && employee.WeComUserID != "" && identity.WeComUserID != employee.WeComUserID {
				return employee, false, errors.New("identity would overwrite a different enterprise WeChat userid")
			}
			return employee, true, nil
		}
	}
	return hrm.Employee{}, false, nil
}

func (r *runner) resolveUser(identity Identity) (int64, error) {
	return r.resolveUserPolicy(identity, false)
}

// IM historical mappings may reference a disabled account without enabling it.
// Employee provisioning retains its existing active-account validation.
func (r *runner) resolveHistoricalUser(identity Identity) (int64, error) {
	return r.resolveUserPolicy(identity, true)
}

func (r *runner) resolveUserPolicy(identity Identity, allowInactive bool) (int64, error) {
	employee, found, err := r.findEmployee(identity)
	if err != nil {
		return 0, err
	}
	id := identity.HostUserID
	if id < 0 {
		return 0, errors.New("host_user_id must be positive")
	}
	if found && employee.UserID > 0 {
		if id > 0 && id != employee.UserID {
			return 0, errors.New("explicit host user conflicts with the employee mapping")
		}
		id = employee.UserID
	}
	if id == 0 && identity.Mobile != "" {
		query := core.HostModel("admin").Ctx(r.ctx).Where("mobile", identity.Mobile)
		if !allowInactive {
			query = query.Where("status", 0)
		}
		rows, e := query.Fields("id").Limit(2).All()
		if e != nil {
			return 0, databaseError()
		}
		if len(rows) > 1 {
			return 0, errors.New("mobile matches multiple host users; bind explicitly")
		}
		if len(rows) == 1 {
			id = rows[0]["id"].Int64()
		}
	}
	if id == 0 && identity.Username != "" {
		if !identity.ConfirmUsername {
			return 0, errors.New("username fallback requires confirm_username=true after manual review")
		}
		query := core.HostModel("admin").Ctx(r.ctx).Where("username", identity.Username)
		if !allowInactive {
			query = query.Where("status", 0)
		}
		rows, e := query.Fields("id").Limit(2).All()
		if e != nil {
			return 0, databaseError()
		}
		if len(rows) != 1 {
			return 0, errors.New("confirmed username has no unique eligible host user")
		}
		id = rows[0]["id"].Int64()
	}
	if id <= 0 {
		return 0, errors.New("identity has no host user mapping; provision and bind the account separately")
	}
	query := core.HostModel("admin").Ctx(r.ctx).Where("id", id)
	if !allowInactive {
		query = query.Where("status", 0)
	}
	ok, err := query.Exist()
	if err != nil {
		return 0, databaseError()
	}
	if !ok {
		return 0, errors.New("host user does not exist or is not eligible for this mapping")
	}
	return id, nil
}

func (r *runner) employee(source Employee) (hrm.Employee, error) {
	out, found, err := r.findEmployee(source.Identity)
	if err != nil {
		return out, err
	}
	if !found {
		// A source with no public identifier remains repeatable via its trace ID.
		previous, e := core.Model("biz_migration_record").Ctx(r.ctx).Where(g.Map{"entity": "employee", "source_id": source.SourceID, "status": "success"}).One()
		if e != nil {
			return out, databaseError()
		}
		target := idFor("employee", source.SourceID)
		if len(previous) > 0 {
			target = previous["target_id"].String()
		}
		row, e := core.Model("biz_hrm_employee").Ctx(r.ctx).Where("id", target).One()
		if e != nil {
			return out, databaseError()
		}
		if len(row) > 0 {
			if row.Struct(&out) != nil {
				return out, databaseError()
			}
		}
		out.ID = target
	}
	if source.Name != "" {
		out.Name = strings.TrimSpace(source.Name)
	}
	if out.Name == "" {
		return out, errors.New("employee name is required")
	}
	if source.WeComUserID != "" {
		out.WeComUserID = source.WeComUserID
	}
	if source.EmployeeNo != "" {
		out.EmployeeNo = source.EmployeeNo
	}
	if source.Mobile != "" {
		out.Mobile = source.Mobile
	}
	if source.Email != "" {
		out.Email = source.Email
	}
	if source.Position != "" {
		out.Position = source.Position
	}
	if source.Status != "" {
		out.Status = source.Status
	}
	out.Status = defaultString(out.Status, "active")
	if out.Status != "active" && out.Status != "inactive" && out.Status != "resigned" {
		return out, errors.New("invalid employee status")
	}
	if source.EntryDate != "" {
		out.EntryDate = source.EntryDate
	}
	if source.QuitDate != "" {
		out.QuitDate = source.QuitDate
	}
	if err = validateDates(out.EntryDate, out.QuitDate); err != nil {
		return out, err
	}
	if source.HostDeptID < 0 {
		return out, errors.New("host_dept_id must not be negative")
	}
	if source.HostDeptID > 0 {
		out.DeptID = source.HostDeptID
	}
	if out.DeptID > 0 {
		ok, e := core.HostModel("auth_dept").Ctx(r.ctx).Where("id", out.DeptID).Exist()
		if e != nil {
			return out, databaseError()
		}
		if !ok {
			return out, errors.New("host department is not mapped")
		}
	}
	if source.HostUserID != 0 || source.Username != "" {
		id, e := r.resolveUser(source.Identity)
		if e != nil {
			return out, e
		}
		if out.UserID > 0 && out.UserID != id {
			return out, errors.New("host user conflicts with employee binding")
		}
		out.UserID = id
	}
	if out.UserID > 0 {
		ok, e := core.HostUserExists(r.ctx, out.UserID)
		if e != nil {
			return out, databaseError()
		}
		if !ok {
			return out, errors.New("mapped host user is missing or disabled")
		}
		n, e := core.Model("biz_hrm_employee").Ctx(r.ctx).Where("user_id", out.UserID).WhereNot("id", out.ID).Count()
		if e != nil {
			return out, databaseError()
		}
		if n > 0 {
			return out, errors.New("host user is already bound to another employee")
		}
		for _, planned := range r.employees {
			if planned.ID != out.ID && planned.UserID == out.UserID {
				return out, errors.New("host user is already bound to another source employee")
			}
		}
	}
	for field, value := range map[string]string{"wecom_userid": out.WeComUserID, "employee_no": out.EmployeeNo} {
		if value == "" {
			continue
		}
		n, e := core.Model("biz_hrm_employee").Ctx(r.ctx).Where(field, value).WhereNot("id", out.ID).Count()
		if e != nil {
			return out, databaseError()
		}
		if n > 0 {
			return out, errors.New("employee identifier is already bound to another employee")
		}
	}
	return out, nil
}
