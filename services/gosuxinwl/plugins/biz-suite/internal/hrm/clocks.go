package hrm

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/wecomprofile"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

type clockEmployeeOption struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	EmployeeNo      string   `json:"employee_no"`
	DepartmentNames []string `json:"department_names"`
	DepartmentKeys  []string `json:"department_keys"`
}

type clockDepartmentOption struct {
	Key      string `json:"key"`
	Source   string `json:"source"`
	ID       int64  `json:"id"`
	ParentID int64  `json:"parent_id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
}

type clockDirectory struct {
	Employees   []clockEmployeeOption   `json:"employees"`
	Departments []clockDepartmentOption `json:"departments"`
	employees   map[string]clockEmployeeOption
	departments map[string]clockDepartmentOption
}

func departmentKey(source string, id int64) string { return source + ":" + strconv.FormatInt(id, 10) }

// Build the mapping once per request. The JSON department membership is decoded
// in Go so MySQL and SQLite use the same rules without database-specific JSON SQL.
func loadClockDirectory(ctx context.Context) (*clockDirectory, error) {
	d := &clockDirectory{Employees: []clockEmployeeOption{}, Departments: []clockDepartmentOption{}, employees: map[string]clockEmployeeOption{}, departments: map[string]clockDepartmentOption{}}
	exclusions, err := core.LoadExplicitExclusions(ctx)
	if err != nil {
		return nil, err
	}
	wdepts, err := core.Model("wecom_department").Ctx(ctx).Fields("wecom_id,parent_id,name").All()
	if err != nil {
		return nil, err
	}
	hdepts, err := core.HostModel("auth_dept").Ctx(ctx).Unscoped().Fields("id,pid,name,deletetime").All()
	if err != nil {
		return nil, err
	}
	for _, row := range wdepts {
		id := row["wecom_id"].Int64()
		if id > 0 {
			item := clockDepartmentOption{Key: departmentKey("wecom", id), Source: "wecom", ID: id, ParentID: row["parent_id"].Int64(), Name: row["name"].String()}
			d.departments[item.Key] = item
		}
	}
	hiddenDepartments := map[string]bool{}
	for id := range exclusions.Departments {
		hiddenDepartments[departmentKey("wecom", id)] = exclusions.Departments[id]
	}
	for _, row := range hdepts {
		id := row["id"].Int64()
		if id > 0 {
			item := clockDepartmentOption{Key: departmentKey("host", id), Source: "host", ID: id, ParentID: row["pid"].Int64(), Name: row["name"].String()}
			d.departments[item.Key] = item
			if !row["deletetime"].IsEmpty() {
				hiddenDepartments[item.Key] = true
			}
		}
	}
	for key, item := range d.departments {
		item.Path = d.departmentPath(key)
		d.departments[key] = item
		if !hiddenDepartments[key] {
			d.Departments = append(d.Departments, item)
		}
	}
	sort.Slice(d.Departments, func(i, j int) bool {
		a, b := d.Departments[i], d.Departments[j]
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.ID < b.ID
	})
	users, err := core.Model("wecom_user").Ctx(ctx).Fields("wecom_userid,employee_id,name,department_ids,raw_json").All()
	if err != nil {
		return nil, err
	}
	type member struct {
		userID, name, employeeNo string
		departments              []int64
	}
	byUser := map[string]member{}
	byEmployee := map[string]member{}
	boundCount := map[string]int{}
	for _, row := range users {
		m := member{userID: row["wecom_userid"].String(), name: row["name"].String(), employeeNo: wecomprofile.EmployeeNoFromRaw(row["raw_json"].String())}
		if m.userID == "" {
			continue
		}
		// Invalid membership never broadens the department selection.
		if json.Unmarshal([]byte(row["department_ids"].String()), &m.departments) != nil {
			m.departments = nil
		}
		byUser[m.userID] = m
		if id := row["employee_id"].String(); id != "" {
			byEmployee[id] = m
			boundCount[id]++
		}
	}
	employees, err := core.Model("biz_hrm_employee").Ctx(ctx).Fields("id,name,employee_no,wecom_userid,dept_id,status").All()
	if err != nil {
		return nil, err
	}
	for _, row := range employees {
		id, userid := row["id"].String(), row["wecom_userid"].String()
		m, found := member{}, false
		if userid != "" {
			m, found = byUser[userid]
		}
		if userid == "" && boundCount[id] == 1 {
			m, found = byEmployee[id], true
		}
		name := strings.TrimSpace(row["name"].String())
		if name == id || name == userid || (found && name == m.userID) {
			name = ""
		}
		if name == "" && found && strings.TrimSpace(m.name) != m.userID && strings.TrimSpace(m.name) != id {
			name = strings.TrimSpace(m.name)
		}
		item := clockEmployeeOption{ID: id, Name: name, EmployeeNo: row["employee_no"].String(), DepartmentNames: []string{}, DepartmentKeys: []string{}}
		if strings.TrimSpace(item.EmployeeNo) == "" && found {
			item.EmployeeNo = m.employeeNo
		}
		keys := map[string]bool{}
		if found {
			for _, dept := range m.departments {
				if dept > 0 {
					keys[departmentKey("wecom", dept)] = true
				}
			}
		}
		if dept := row["dept_id"].Int64(); dept > 0 {
			keys[departmentKey("host", dept)] = true
		}
		for key := range keys {
			item.DepartmentKeys = append(item.DepartmentKeys, key)
		}
		sort.Strings(item.DepartmentKeys)
		names := map[string]bool{}
		for _, key := range item.DepartmentKeys {
			if dept, ok := d.departments[key]; ok && dept.Name != "" && !names[dept.Name] {
				item.DepartmentNames = append(item.DepartmentNames, dept.Name)
				names[dept.Name] = true
			}
		}
		d.employees[id] = item
		if row["status"].String() != "filtered" && !exclusions.Users[userid] && !(found && exclusions.Users[m.userID]) {
			d.Employees = append(d.Employees, item)
		}
	}
	sort.Slice(d.Employees, func(i, j int) bool {
		a, b := d.Employees[i], d.Employees[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.EmployeeNo != b.EmployeeNo {
			return a.EmployeeNo < b.EmployeeNo
		}
		return a.ID < b.ID
	})
	queryScope(ctx).filterDirectory(d)
	return d, nil
}

func (d *clockDirectory) departmentPath(key string) string {
	parts, seen := []string{}, map[string]bool{}
	for key != "" && !seen[key] {
		seen[key] = true
		item, ok := d.departments[key]
		if !ok {
			break
		}
		parts = append(parts, item.Name)
		if item.ParentID <= 0 {
			break
		}
		key = departmentKey(item.Source, item.ParentID)
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, " / ")
}

func (d *clockDirectory) inDepartment(employee clockEmployeeOption, selected string, descendants bool) bool {
	for _, key := range employee.DepartmentKeys {
		seen := map[string]bool{}
		for key != "" && !seen[key] {
			if key == selected {
				return true
			}
			seen[key] = true
			item, ok := d.departments[key]
			if !descendants || !ok || item.ParentID <= 0 {
				break
			}
			key = departmentKey(item.Source, item.ParentID)
		}
	}
	return false
}

type clockFilter struct {
	Page, Size                                     int
	Search, EmployeeID, EmployeeSearch, Department string
	Status                                         string
	Basis                                          string
	IncludeSubdepartments                          bool
	Start, End                                     time.Time
	EndExclusive                                   bool
}

func parseClockFilter(values map[string]string) (clockFilter, error) {
	f := clockFilter{Page: 1, Size: 100, IncludeSubdepartments: true}
	for _, p := range []struct {
		key   string
		value *int
		max   int
	}{{"page", &f.Page, 100000}, {"page_size", &f.Size, 500}} {
		if raw := values[p.key]; raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > p.max {
				return f, fmt.Errorf("page >= 1，page_size 应在 1 至 500 之间")
			}
			*p.value = n
		}
	}
	f.Search = strings.TrimSpace(values["search"])
	if f.Search == "" {
		f.Search = strings.TrimSpace(values["q"])
	}
	f.EmployeeID = strings.TrimSpace(values["employee_id"])
	f.EmployeeSearch = strings.TrimSpace(values["employee_search"])
	f.Status = strings.TrimSpace(values["status"])
	f.Basis = values["basis"]
	if f.Basis != "" && f.Basis != "source" && f.Basis != "local" {
		return f, fmt.Errorf("状态依据无效")
	}
	switch f.Status {
	case "", "normal", "late", "early", "missing", "abnormal":
	case "uncomputed", "pending_review", "configuration_pending", "in_progress":
		if f.Basis != "local" {
			return f, fmt.Errorf("该状态仅适用于本地核算")
		}
	default:
		return f, fmt.Errorf("打卡状态无效，请选择全部、正常、迟到、早退、缺卡或异常")
	}
	f.Department = strings.TrimSpace(values["department"])
	if f.Department != "" {
		parts := strings.Split(f.Department, ":")
		if len(parts) != 2 || (parts[0] != "wecom" && parts[0] != "host") {
			return f, fmt.Errorf("部门筛选应为 wecom:部门ID 或 host:部门ID")
		}
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || id <= 0 || strconv.FormatInt(id, 10) != parts[1] {
			return f, fmt.Errorf("部门ID必须为正整数")
		}
	}
	if raw := values["include_subdepartments"]; raw != "" {
		switch raw {
		case "true", "1":
			f.IncludeSubdepartments = true
		case "false", "0":
			f.IncludeSubdepartments = false
		default:
			return f, fmt.Errorf("include_subdepartments 应为 true 或 false")
		}
	}
	date, month := strings.TrimSpace(values["date"]), strings.TrimSpace(values["month"])
	start, end := strings.TrimSpace(values["start_time"]), strings.TrimSpace(values["end_time"])
	modes := 0
	for _, active := range []bool{date != "", month != "", start != "" || end != ""} {
		if active {
			modes++
		}
	}
	if modes > 1 {
		return f, fmt.Errorf("按日期、按月度和自定义日期范围只能选择一种")
	}
	if date != "" || month != "" {
		raw, layout := date, "2006-01-02"
		if month != "" {
			raw, layout = month, "2006-01"
		}
		stamp, err := time.ParseInLocation(layout, raw, core.Location)
		if err != nil || stamp.Format(layout) != raw || stamp.Year() < 1000 {
			return f, fmt.Errorf("日期应为 YYYY-MM-DD，月度应为 YYYY-MM")
		}
		f.Start, f.EndExclusive = stamp, true
		if month != "" {
			f.End = stamp.AddDate(0, 1, 0)
		} else {
			f.End = stamp.AddDate(0, 0, 1)
		}
	} else {
		if start != "" {
			stamp, err := core.ParseTime(start)
			if err != nil || stamp.Year() < 1000 {
				return f, fmt.Errorf("开始日期筛选无效")
			}
			f.Start = stamp
		}
		if end != "" {
			stamp, err := core.ParseTime(end)
			if err != nil || stamp.Year() < 1000 {
				return f, fmt.Errorf("结束日期筛选无效")
			}
			if len(end) == 10 {
				stamp = stamp.AddDate(0, 0, 1)
				f.EndExclusive = true
			}
			f.End = stamp
		}
	}
	if f.Start.Year() > 9999 || f.End.Year() > 9999 {
		return f, fmt.Errorf("日期超出支持范围")
	}
	if !f.Start.IsZero() && !f.End.IsZero() && (f.End.Before(f.Start) || (f.EndExclusive && f.End.Equal(f.Start))) {
		return f, fmt.Errorf("结束日期不能早于开始日期")
	}
	return f, nil
}

func queryClocks(ctx context.Context, f clockFilter, directory *clockDirectory) ([]g.Map, int, error) {
	query := queryScope(ctx).apply(core.Model("biz_hrm_attendance_clock").Ctx(ctx), "employee_id")
	if f.Status != "" {
		if f.Basis == "local" {
			query = query.Where(attClockLocalStatusSQL()+"=?", f.Status)
		} else {
			query = query.Where("status", f.Status)
		}
	}
	if f.EmployeeID != "" {
		query = query.Where("employee_id", f.EmployeeID)
	}
	if f.EmployeeSearch != "" {
		ids := []string{}
		term := strings.ToLower(f.EmployeeSearch)
		for _, employee := range directory.employees {
			if strings.Contains(strings.ToLower(employee.Name), term) || strings.Contains(strings.ToLower(employee.EmployeeNo), term) {
				ids = append(ids, employee.ID)
			}
		}
		if len(ids) == 0 {
			query = query.Where("1=0")
		} else {
			query = query.WhereIn("employee_id", ids)
		}
	}
	if f.Department != "" {
		if _, exists := directory.departments[f.Department]; !exists {
			return nil, 0, fmt.Errorf("所选部门不存在或尚未同步")
		}
		ids := []string{}
		for _, employee := range directory.employees {
			if directory.inDepartment(employee, f.Department, f.IncludeSubdepartments) {
				ids = append(ids, employee.ID)
			}
		}
		if len(ids) == 0 {
			query = query.Where("1=0")
		} else {
			query = query.WhereIn("employee_id", ids)
		}
	}
	if f.Search != "" {
		conditions, args := []string{}, []any{}
		// Search visible record values only. Internal employee identifiers are not
		// employee numbers and must never make an unrelated person match a number.
		for _, field := range []string{"clock_time", "clock_type", "status", "source", "address"} {
			conditions = append(conditions, field+" LIKE ?")
			args = append(args, "%"+f.Search+"%")
		}
		ids := []string{}
		for _, employee := range directory.employees {
			if strings.Contains(strings.ToLower(employee.Name), strings.ToLower(f.Search)) || strings.Contains(strings.ToLower(employee.EmployeeNo), strings.ToLower(f.Search)) {
				ids = append(ids, employee.ID)
			}
		}
		if len(ids) > 0 {
			conditions = append(conditions, "employee_id IN (?)")
			args = append(args, ids)
		}
		query = query.Where("("+strings.Join(conditions, " OR ")+")", args...)
	}
	const format = "2006-01-02 15:04:05"
	if !f.Start.IsZero() {
		query = query.WhereGTE("clock_time", f.Start.Format(format))
	}
	if !f.End.IsZero() {
		if f.EndExclusive {
			query = query.WhereLT("clock_time", f.End.Format(format))
		} else {
			query = query.WhereLTE("clock_time", f.End.Format(format))
		}
	}
	total, err := query.Clone().Count()
	if err != nil {
		return nil, 0, err
	}
	rows, err := query.Fields("biz_hrm_attendance_clock.*, "+attClockDayResultSQL("status")+" AS attendance_day_state, "+attClockDayResultSQL("id")+" AS attendance_day_result_id").OrderDesc("clock_time").OrderDesc("id").Page(f.Page, f.Size).All()
	if err != nil {
		return nil, 0, err
	}
	result := make([]g.Map, 0, len(rows))
	states := map[string]gdb.Record{}
	ids := []string{}
	for _, row := range rows {
		ids = append(ids, row["id"].String())
	}
	if len(ids) > 0 {
		records, e := core.Model("biz_att_clock_state").Ctx(ctx).WhereIn("clock_id", ids).All()
		if e != nil {
			return nil, 0, e
		}
		for _, row := range records {
			states[row["clock_id"].String()] = row
		}
	}
	for _, row := range rows {
		item := row.Map()
		item["source_status"] = row["status"].String()
		item["local_status"] = "uncomputed"
		item["local_basis"] = ""
		item["local_result_id"] = ""
		switch row["attendance_day_state"].String() {
		case "configuration_pending":
			item["local_status"], item["local_basis"], item["local_result_id"] = "configuration_pending", "unassigned", row["attendance_day_result_id"].String()
		case "pending":
			item["local_status"], item["local_basis"], item["local_result_id"] = "in_progress", "unassigned", row["attendance_day_result_id"].String()
		}
		delete(item, "attendance_day_state")
		delete(item, "attendance_day_result_id")
		if state, ok := states[row["id"].String()]; ok {
			item["local_status"] = state["local_status"].String()
			item["local_basis"] = state["basis"].String()
			item["local_result_id"] = state["result_id"].String()
		}
		employee, matched := directory.employees[row["employee_id"].String()]
		item["employee_name"], item["employee_no"], item["department_names"] = "", "", []string{}
		if matched {
			item["employee_name"], item["employee_no"], item["department_names"] = employee.Name, employee.EmployeeNo, employee.DepartmentNames
		}
		result = append(result, item)
	}
	return result, total, nil
}

func clockOptions(r *ghttp.Request) {
	if !auth(r) {
		return
	}
	directory, err := loadClockDirectory(r.Context())
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg("无法读取员工和部门选项"))
		return
	}
	r.Response.WriteJson(gf.Success().SetData(directory))
}

func listClocks(r *ghttp.Request) {
	if !auth(r) {
		return
	}
	values := map[string]string{}
	for _, key := range []string{"page", "page_size", "search", "q", "employee_id", "employee_search", "department", "include_subdepartments", "date", "month", "start_time", "end_time", "status", "basis"} {
		values[key] = r.GetQuery(key).String()
	}
	filter, err := parseClockFilter(values)
	if err != nil {
		r.Response.WriteHeader(http.StatusBadRequest)
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	directory, err := loadClockDirectory(r.Context())
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg("无法读取员工和部门映射"))
		return
	}
	rows, total, err := queryClocks(r.Context(), filter, directory)
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	r.Response.Header().Set("X-Total-Count", strconv.Itoa(total))
	r.Response.WriteJson(gf.Success().SetData(rows))
}
