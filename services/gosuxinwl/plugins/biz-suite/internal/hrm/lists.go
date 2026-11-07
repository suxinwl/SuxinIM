package hrm

import (
	"fmt"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"strings"
	"time"
)

func listRecords(r *ghttp.Request, table, order, dateField string) {
	if !auth(r) {
		return
	}
	page, size := r.GetQuery("page", 1).Int(), r.GetQuery("page_size", 100).Int()
	if page < 1 || page > 100000 || size < 1 || size > 500 {
		r.Response.WriteJson(gf.Failed().SetMsg("page >= 1, page_size 应在 1 至 500 之间"))
		return
	}
	query := core.Model(table).Ctx(r.Context())
	if scope := queryScope(r.Context()); !scope.All && len(scope.EmployeeIDs) == 0 {
		query = query.Where("1=0")
	}
	term := strings.TrimSpace(r.GetQuery("search", r.GetQuery("q").String()).String())
	if term != "" {
		// Column names come only from this fixed list, never from the request.
		fields := map[string][]string{
			"biz_hrm_employee":           {"name", "employee_no", "wecom_userid", "position", "mobile", "email", "status"},
			"biz_hrm_attendance_group":   {"name"},
			"biz_hrm_attendance_holiday": {"holiday_date", "name", "remark"},
			"biz_hrm_attendance_clock":   {"employee_id", "clock_time", "clock_type", "status", "source", "address"},
			"biz_hrm_attendance_leave":   {"employee_id", "leave_type", "approval_status", "source", "reason"},
			"biz_hrm_attendance_out":     {"employee_id", "destination", "approval_status", "source", "reason"},
			"biz_hrm_attendance_trip":    {"employee_id", "destination", "approval_status", "source", "reason"},
		}[table]
		var conditions []string
		var args []any
		for _, field := range fields {
			conditions = append(conditions, field+" LIKE ?")
			args = append(args, "%"+term+"%")
		}
		if len(conditions) > 0 {
			query = query.Where("("+strings.Join(conditions, " OR ")+")", args...)
		}
	}
	if table == "biz_hrm_attendance_clock" || table == "biz_hrm_attendance_leave" || table == "biz_hrm_attendance_out" || table == "biz_hrm_attendance_trip" {
		if employee := r.GetQuery("employee_id").String(); employee != "" {
			query = query.Where("employee_id", employee)
		}
	}
	if dateField != "" {
		format := "2006-01-02 15:04:05"
		if dateField == "holiday_date" {
			format = "2006-01-02"
		}
		for _, key := range []string{"start_time", "end_time"} {
			raw := r.GetQuery(key).String()
			if raw == "" {
				continue
			}
			stamp, err := core.ParseTime(raw)
			if err != nil {
				r.Response.WriteJson(gf.Failed().SetMsg("日期筛选无效"))
				return
			}
			if key == "start_time" {
				query = query.WhereGTE(dateField, stamp.Format(format))
			} else {
				if len(raw) == 10 {
					stamp = stamp.Add(24 * time.Hour)
					query = query.WhereLT(dateField, stamp.Format(format))
				} else {
					query = query.WhereLTE(dateField, stamp.Format(format))
				}
			}
		}
	}
	total, err := query.Clone().Count()
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg("无法读取记录数量"))
		return
	}
	rows, err := query.OrderDesc(order).OrderDesc("id").Page(page, size).All()
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg("无法读取记录"))
		return
	}
	r.Response.Header().Set("X-Total-Count", fmt.Sprint(total))
	if rows == nil {
		rows = gdb.Result{}
	}
	r.Response.WriteJson(gf.Success().SetData(rows))
}
