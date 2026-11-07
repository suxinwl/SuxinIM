package hrm

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/approvalview"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

var absenceTitles = map[string]string{"leave": "请假", "out": "外出", "trip": "出差"}

func absenceTable(kind string) (string, error) {
	if _, ok := absenceTitles[kind]; !ok {
		return "", fmt.Errorf("不支持的考勤业务类型")
	}
	return "biz_hrm_attendance_" + kind, nil
}

func absenceFields(kind string) string {
	fields := "id,employee_id,source,external_ref,approval_status,start_time,end_time,duration,reason,created_at,updated_at,raw_json"
	if kind == "leave" {
		return fields + ",leave_type"
	}
	return fields + ",destination"
}

// Explicit projection keeps raw source documents out of list and detail APIs.
func projectAbsence(row gdb.Record, kind string, directory *clockDirectory) g.Map {
	item := g.Map{}
	for _, key := range []string{"id", "employee_id", "source", "external_ref", "approval_status", "start_time", "end_time", "reason", "created_at", "updated_at"} {
		item[key] = row[key].String()
	}
	item["duration"] = row["duration"].Float64()
	item["employee_name"], item["employee_no"], item["department_names"] = "", "", []string{}
	if employee, ok := directory.employees[row["employee_id"].String()]; ok {
		item["employee_name"], item["employee_no"], item["department_names"] = employee.Name, employee.EmployeeNo, employee.DepartmentNames
	}
	if kind == "leave" {
		stored := row["leave_type"].String()
		item["leave_type"], item["leave_type_name"] = stored, absenceLeaveLabel(row["raw_json"].String(), stored, row["source"].String())
	} else {
		item["destination"] = row["destination"].String()
	}
	return item
}

func absenceLeaveLabel(raw, stored, source string) string {
	if source == "wecom" {
		if label := approvalview.LeaveTypeLabel(raw, stored); label != "" {
			return label
		}
	}
	// Chinese text stored by the manual editor or a newer synchronizer is already
	// a label. Opaque numeric/English selector keys are never guessed as a type.
	for _, char := range stored {
		if unicode.Is(unicode.Han, char) {
			return stored
		}
	}
	if source == "manual" && strings.TrimSpace(stored) != "" {
		return stored
	}
	return "未提供假期名称"
}

func queryAbsences(ctx context.Context, kind string, f clockFilter, directory *clockDirectory) ([]g.Map, int, error) {
	table, err := absenceTable(kind)
	if err != nil {
		return nil, 0, err
	}
	query := queryScope(ctx).apply(core.Model(table).Ctx(ctx), "employee_id")
	if f.EmployeeID != "" {
		query = query.Where("employee_id", f.EmployeeID)
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
		fields := []string{"approval_status", "source", "reason"}
		if kind == "leave" {
			fields = append(fields, "leave_type")
		} else {
			fields = append(fields, "destination")
		}
		for _, field := range fields {
			conditions = append(conditions, field+" LIKE ?")
			args = append(args, "%"+f.Search+"%")
		}
		ids := []string{}
		term := strings.ToLower(f.Search)
		for _, employee := range directory.employees {
			if strings.Contains(strings.ToLower(employee.Name), term) || strings.Contains(strings.ToLower(employee.EmployeeNo), term) {
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
		query = query.WhereGTE("start_time", f.Start.Format(format))
	}
	if !f.End.IsZero() {
		if f.EndExclusive {
			query = query.WhereLT("start_time", f.End.Format(format))
		} else {
			query = query.WhereLTE("start_time", f.End.Format(format))
		}
	}
	total, err := query.Clone().Count()
	if err != nil {
		return nil, 0, err
	}
	rows, err := query.Fields(absenceFields(kind)).OrderDesc("start_time").OrderDesc("id").Page(f.Page, f.Size).All()
	if err != nil {
		return nil, 0, err
	}
	items := make([]g.Map, 0, len(rows))
	for _, row := range rows {
		items = append(items, projectAbsence(row, kind, directory))
	}
	return items, total, nil
}

func listAbsences(r *ghttp.Request, kind string) {
	if !auth(r) {
		return
	}
	values := map[string]string{}
	for _, key := range []string{"page", "page_size", "search", "q", "employee_id", "department", "include_subdepartments", "date", "month", "start_time", "end_time"} {
		values[key] = r.GetQuery(key).String()
	}
	filter, err := parseClockFilter(values)
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	directory, err := loadClockDirectory(r.Context())
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg("无法读取员工和部门映射"))
		return
	}
	rows, total, err := queryAbsences(r.Context(), kind, filter, directory)
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg("无法读取考勤审批记录"))
		return
	}
	r.Response.Header().Set("X-Total-Count", strconv.Itoa(total))
	r.Response.WriteJson(gf.Success().SetData(rows))
}

func absenceDetail(r *ghttp.Request, kind string) {
	if !auth(r) {
		return
	}
	result, err := readAbsenceDetail(r.Context(), kind, strings.TrimSpace(r.GetRouter("id").String()))
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	r.Response.WriteJson(gf.Success().SetData(result))
}

func readAbsenceDetail(ctx context.Context, kind, id string) (g.Map, error) {
	table, err := absenceTable(kind)
	if err != nil {
		return nil, err
	}
	if id == "" || len(id) > 64 {
		return nil, fmt.Errorf("记录编号无效")
	}
	row, err := queryScope(ctx).apply(core.Model(table).Ctx(ctx), "employee_id").Where("id", id).Fields(absenceFields(kind)).One()
	if err != nil {
		return nil, fmt.Errorf("无法读取考勤审批记录")
	}
	if len(row) == 0 {
		return nil, fmt.Errorf("记录不存在")
	}
	directory, err := loadClockDirectory(ctx)
	if err != nil {
		return nil, fmt.Errorf("无法读取员工和部门映射")
	}
	item := projectAbsence(row, kind, directory)
	form, err := buildApprovalForm(ctx, row, kind, item)
	if err != nil {
		return nil, err
	}
	item["approval_form"] = form
	return item, nil
}
