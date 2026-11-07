package hrm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

func employeeListRecords(r *ghttp.Request) {
	page, size := r.GetQuery("page", 1).Int(), r.GetQuery("page_size", 100).Int()
	if page < 1 || page > 100000 || size < 1 || size > 500 {
		r.Response.WriteJson(gf.Failed().SetMsg("page >= 1, page_size 应在 1 至 500 之间"))
		return
	}
	ctx := r.Context()
	directory, err := loadClockDirectory(ctx)
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg("无法读取员工目录"))
		return
	}
	query := queryScope(ctx).apply(core.Model("biz_hrm_employee").Ctx(ctx), "id")
	query, err = currentEmployees(ctx, query)
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg("无法读取员工过滤规则"))
		return
	}
	if term := strings.TrimSpace(r.GetQuery("search", r.GetQuery("q").String()).String()); term != "" {
		// Directory fallback includes a verified WeCom employee number even before
		// the next contact sync persists it. Internal IDs are never search aliases.
		ids := []string{}
		for _, employee := range directory.Employees {
			if strings.Contains(strings.ToLower(employee.Name), strings.ToLower(term)) || strings.Contains(strings.ToLower(employee.EmployeeNo), strings.ToLower(term)) {
				ids = append(ids, employee.ID)
			}
		}
		if len(ids) == 0 {
			query = query.Where("1=0")
		} else {
			query = query.WhereIn("id", ids)
		}
	}
	total, err := query.Clone().Count()
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg("无法读取员工数量"))
		return
	}
	rows, err := query.OrderDesc("updated_at").OrderDesc("id").Page(page, size).All()
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg("无法读取员工档案"))
		return
	}
	userIDs := []int64{}
	for _, row := range rows {
		if id := row["user_id"].Int64(); id > 0 {
			userIDs = append(userIDs, id)
		}
	}
	userQuery := core.HostModel("admin").Ctx(ctx).Fields("id,username,name,nickname,dept_id,status")
	if len(userIDs) == 0 {
		userQuery = userQuery.Where("1=0")
	} else {
		userQuery = userQuery.WhereIn("id", userIDs)
	}
	users, err := userQuery.All()
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg("无法读取关联账号"))
		return
	}
	hosts := map[int64]g.Map{}
	for _, user := range users {
		hosts[user["id"].Int64()] = user.Map()
	}
	results := []g.Map{}
	for _, row := range rows {
		item := row.Map()
		item["host_username"], item["host_display_name"], item["host_department_name"], item["host_department_path"] = "", "", "", ""
		item["department_names"] = []string{}
		item["mapping_status"], item["mapping_message"] = "unmapped", "尚未关联本地账号"
		if employee, ok := directory.employees[row["id"].String()]; ok {
			item["name"], item["employee_no"], item["department_names"] = employee.Name, employee.EmployeeNo, employee.DepartmentNames
		}
		if userID := row["user_id"].Int64(); userID > 0 {
			if user, exists := hosts[userID]; exists {
				item["host_username"], item["host_display_name"] = user["username"], user["name"]
				if strings.TrimSpace(fmt.Sprint(user["name"])) == "" {
					item["host_display_name"] = user["nickname"]
				}
				// Use the directory's explicit host department key rather than a
				// similarly named WeCom secondary department.
				hostDept := gconv.Int64(user["dept_id"])
				if dept, ok := directory.departments[departmentKey("host", hostDept)]; ok {
					item["host_department_name"], item["host_department_path"] = dept.Name, dept.Path
				}
				item["mapping_status"], item["mapping_message"] = "mapped", "本地账号已关联"
				if gconv.Int64(user["status"]) != 0 {
					item["mapping_status"], item["mapping_message"] = "disabled", "本地账号已关联，尚未启用"
				}
				if row["dept_id"].Int64() != hostDept {
					item["mapping_status"], item["mapping_message"] = "conflict", "员工与本地账号的主部门不一致"
				}
			} else {
				item["mapping_status"], item["mapping_message"] = "conflict", "关联的本地账号不存在或已删除"
			}
		}
		results = append(results, item)
	}
	r.Response.Header().Set("X-Total-Count", fmt.Sprint(total))
	r.Response.WriteJson(gf.Success().SetData(results))
}

func employeeOptions(r *ghttp.Request) {
	ctx := r.Context()
	scope := queryScope(ctx)
	query := core.HostModel("admin").Ctx(ctx).Fields("id,name,username,status")
	if !scope.All {
		if len(scope.EmployeeIDs) == 0 {
			query = query.Where("1=0")
		} else {
			query = query.Where("id", scope.OwnUserID)
		}
	}
	users, err := query.All()
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg("无法读取本地账号选项"))
		return
	}
	userOptions := []g.Map{}
	for _, user := range users {
		userOptions = append(userOptions, user.Map())
	}
	directory, err := loadClockDirectory(ctx)
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg("无法读取员工选项"))
		return
	}
	departments := []g.Map{}
	for _, department := range directory.Departments {
		if department.Source == "host" {
			departments = append(departments, g.Map{"id": department.ID, "name": department.Name, "pid": department.ParentID, "path": department.Path})
		}
	}
	employeeQuery, err := currentEmployees(ctx, scope.apply(core.Model("biz_hrm_employee").Ctx(ctx), "id"))
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg("无法读取员工过滤规则"))
		return
	}
	employees, err := employeeQuery.Fields("id,name,employee_no,wecom_userid,dept_id").All()
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg("无法读取员工选项"))
		return
	}
	items := []g.Map{}
	for _, employee := range employees {
		item := employee.Map()
		// Attendance rule scopes resolve against the local primary department,
		// not the multiple memberships in the upstream WeCom contact record.
		item["dept_id"] = employee["dept_id"].Int64()
		item["department"] = ""
		if department, ok := directory.departments[departmentKey("host", employee["dept_id"].Int64())]; ok {
			item["department"] = department.Path
		}
		if value, ok := directory.employees[employee["id"].String()]; ok {
			item["name"], item["employee_no"] = value.Name, value.EmployeeNo
		}
		items = append(items, item)
	}
	r.Response.WriteJson(gf.Success().SetData(g.Map{"users": userOptions, "departments": departments, "employees": items}))
}

func scopedGroupRecords(r *ghttp.Request) {
	scope := queryScope(r.Context())
	if scope.All {
		listRecords(r, "biz_hrm_attendance_group", "updated_at", "")
		return
	}
	page, size := r.GetQuery("page", 1).Int(), r.GetQuery("page_size", 100).Int()
	if page < 1 || page > 100000 || size < 1 || size > 500 {
		r.Response.WriteJson(gf.Failed().SetMsg("page >= 1, page_size 应在 1 至 500 之间"))
		return
	}
	query := core.Model("biz_hrm_attendance_group").Ctx(r.Context())
	if search := strings.TrimSpace(r.GetQuery("search", r.GetQuery("q").String()).String()); search != "" {
		query = query.WhereLike("name", "%"+search+"%")
	}
	rows, err := query.OrderDesc("updated_at").OrderDesc("id").All()
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg("无法读取考勤组"))
		return
	}
	items := []g.Map{}
	for _, row := range rows {
		var employees []string
		var departments []int64
		if json.Unmarshal([]byte(row["employee_ids"].String()), &employees) != nil || json.Unmarshal([]byte(row["dept_ids"].String()), &departments) != nil {
			continue
		}
		visibleEmployees, visibleDepartments := []string{}, []int64{}
		for _, id := range employees {
			if scope.allowsEmployee(id) {
				visibleEmployees = append(visibleEmployees, id)
			}
		}
		for _, id := range departments {
			if scope.DepartmentIDs[id] && len(scope.EmployeeIDs) > 0 {
				visibleDepartments = append(visibleDepartments, id)
			}
		}
		if len(visibleEmployees) == 0 && len(visibleDepartments) == 0 {
			continue
		}
		item := row.Map()
		employeeJSON, _ := json.Marshal(visibleEmployees)
		departmentJSON, _ := json.Marshal(visibleDepartments)
		item["employee_ids"], item["dept_ids"] = string(employeeJSON), string(departmentJSON)
		items = append(items, item)
	}
	total := len(items)
	start := (page - 1) * size
	if start >= total {
		items = []g.Map{}
	} else {
		end := start + size
		if end > total {
			end = total
		}
		items = items[start:end]
	}
	r.Response.Header().Set("X-Total-Count", fmt.Sprint(total))
	r.Response.WriteJson(gf.Success().SetData(items))
}
