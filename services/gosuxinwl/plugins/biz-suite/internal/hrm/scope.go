package hrm

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

type hrmScopeKey struct{}

type hrmScope struct {
	All           bool
	EmployeeIDs   []string
	employees     map[string]bool
	DepartmentIDs map[int64]bool
	OwnUserID     int64
}

// HTTP reads always install a scope in Bind. Internal calculations/sync jobs
// without that private context value retain their existing service semantics.
func queryScope(ctx context.Context) *hrmScope {
	if scope, ok := ctx.Value(hrmScopeKey{}).(*hrmScope); ok {
		return scope
	}
	return &hrmScope{All: true}
}

func hrmReadScope(r *ghttp.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		r.Middleware.Next()
		return
	}
	if !auth(r) {
		r.ExitAll()
		return
	}
	scope, err := loadHRMScope(r.Context())
	if err != nil {
		r.Response.WriteHeader(http.StatusForbidden)
		r.Response.WriteJson(gf.Failed().SetMsg("无法确认人事数据访问范围"))
		r.ExitAll()
		return
	}
	r.SetCtx(context.WithValue(r.Context(), hrmScopeKey{}, scope))
	r.Middleware.Next()
}

func loadHRMScope(ctx context.Context) (*hrmScope, error) {
	scope := &hrmScope{OwnUserID: core.UID(ctx), employees: map[string]bool{}, EmployeeIDs: []string{}, DepartmentIDs: map[int64]bool{}}
	if scope.OwnUserID <= 0 {
		return nil, fmt.Errorf("missing host identity")
	}
	user, err := core.HostModel("admin").Ctx(ctx).Where("id", scope.OwnUserID).Where("status", 0).Fields("id,dept_id").One()
	if err != nil || len(user) == 0 {
		return nil, fmt.Errorf("host account unavailable")
	}
	permissions, err := core.Permissions(ctx)
	if err != nil {
		return nil, err
	}
	self, department := false, false
	for _, permission := range permissions {
		switch permission {
		case "biz-suite.hrm.read", "biz-suite.hrm.write", "biz-suite.hrm.manage":
			scope.All = true
		case "biz-suite.hrm.self":
			self = true
		case "biz-suite.hrm.department":
			department = true
		}
	}
	if scope.All {
		return scope, nil
	}
	if !self && !department {
		return nil, fmt.Errorf("missing HRM read permission")
	}
	boundQuery, err := currentEmployees(ctx, core.Model("biz_hrm_employee").Ctx(ctx).Where("user_id", scope.OwnUserID))
	if err != nil {
		return nil, err
	}
	bound, err := boundQuery.Fields("id,dept_id").Limit(2).All()
	if err != nil {
		return nil, err
	}
	if len(bound) != 1 {
		return scope, nil // Unbound or ambiguous accounts never inherit a department.
	}
	myID := bound[0]["id"].String()
	scope.employees[myID] = true
	scope.EmployeeIDs = append(scope.EmployeeIDs, myID)
	primary := bound[0]["dept_id"].Int64()
	depts, err := core.HostModel("auth_dept").Ctx(ctx).Where("status", 0).Fields("id,pid").All()
	if err != nil {
		return nil, err
	}
	valid := map[int64]int64{}
	for _, row := range depts {
		valid[row["id"].Int64()] = row["pid"].Int64()
	}
	if _, ok := valid[primary]; primary <= 0 || !ok {
		return scope, nil
	}
	scope.DepartmentIDs[primary] = true
	// A stale disagreement between host and HRM primary departments must not
	// grant either tree. Organization sync updates both sides atomically.
	if !department || user["dept_id"].Int64() != primary {
		return scope, nil
	}
	for changed := true; changed; {
		changed = false
		for id, parent := range valid {
			if !scope.DepartmentIDs[id] && scope.DepartmentIDs[parent] {
				scope.DepartmentIDs[id], changed = true, true
			}
		}
	}
	ids := []int64{}
	for id := range scope.DepartmentIDs {
		ids = append(ids, id)
	}
	employeeQuery, err := currentEmployees(ctx, core.Model("biz_hrm_employee").Ctx(ctx).WhereIn("dept_id", ids))
	if err != nil {
		return nil, err
	}
	employees, err := employeeQuery.Fields("id").All()
	if err != nil {
		return nil, err
	}
	for _, row := range employees {
		id := row["id"].String()
		if !scope.employees[id] {
			scope.employees[id] = true
			scope.EmployeeIDs = append(scope.EmployeeIDs, id)
		}
	}
	sort.Strings(scope.EmployeeIDs)
	return scope, nil
}

func (s *hrmScope) allowsEmployee(id string) bool { return s.All || s.employees[id] }

func (s *hrmScope) apply(query *gdb.Model, field string) *gdb.Model {
	if s.All {
		return query
	}
	if len(s.EmployeeIDs) == 0 {
		return query.Where("1=0")
	}
	return query.WhereIn(field, s.EmployeeIDs)
}

func (s *hrmScope) filterDirectory(d *clockDirectory) {
	if s.All {
		return
	}
	visible := map[string]bool{}
	employees := []clockEmployeeOption{}
	for _, employee := range d.Employees {
		if !s.allowsEmployee(employee.ID) {
			delete(d.employees, employee.ID)
			continue
		}
		employees = append(employees, employee)
		for _, start := range employee.DepartmentKeys {
			key, seen := start, map[string]bool{}
			for key != "" && !seen[key] {
				seen[key] = true
				department, ok := d.departments[key]
				if !ok {
					break
				}
				visible[key] = true
				if department.ParentID <= 0 {
					break
				}
				key = departmentKey(department.Source, department.ParentID)
			}
		}
	}
	d.Employees = employees
	departments := []clockDepartmentOption{}
	for _, department := range d.Departments {
		if visible[department.Key] {
			departments = append(departments, department)
		} else {
			delete(d.departments, department.Key)
		}
	}
	d.Departments = departments
}
