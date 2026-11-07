package hrm

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

type Employee struct {
	ID          string `json:"id"`
	UserID      int64  `json:"user_id"`
	WeComUserID string `json:"wecom_userid"`
	EmployeeNo  string `json:"employee_no"`
	Name        string `json:"name"`
	DeptID      int64  `json:"dept_id"`
	Mobile      string `json:"mobile"`
	Email       string `json:"email"`
	Position    string `json:"position"`
	Status      string `json:"status"`
	EntryDate   string `json:"entry_date"`
	QuitDate    string `json:"quit_date"`
}

type Clock struct {
	ID          string `json:"id"`
	EmployeeID  string `json:"employee_id"`
	WeComUserID string `json:"wecom_userid"`
	ClockTime   string `json:"clock_time"`
	ClockType   string `json:"clock_type"`
	Status      string `json:"status"`
	Source      string `json:"source"`
	ExternalRef string `json:"external_ref"`
	Latitude    string `json:"latitude"`
	Longitude   string `json:"longitude"`
	Address     string `json:"address"`
	RawJSON     string `json:"raw_json"`
}

type Leave struct {
	ID             string  `json:"id"`
	EmployeeID     string  `json:"employee_id"`
	Source         string  `json:"source"`
	ExternalRef    string  `json:"external_ref"`
	ApprovalStatus string  `json:"approval_status"`
	LeaveType      string  `json:"leave_type"`
	StartTime      string  `json:"start_time"`
	EndTime        string  `json:"end_time"`
	Duration       float64 `json:"duration"`
	Reason         string  `json:"reason"`
	RawJSON        string  `json:"raw_json"`
}

type Out struct {
	ID             string  `json:"id"`
	EmployeeID     string  `json:"employee_id"`
	Source         string  `json:"source"`
	ExternalRef    string  `json:"external_ref"`
	ApprovalStatus string  `json:"approval_status"`
	StartTime      string  `json:"start_time"`
	EndTime        string  `json:"end_time"`
	Duration       float64 `json:"duration"`
	Destination    string  `json:"destination"`
	Reason         string  `json:"reason"`
	RawJSON        string  `json:"raw_json"`
}

type AttendanceGroup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DeptIDs     string `json:"dept_ids"`
	EmployeeIDs string `json:"employee_ids"`
	Shifts      string `json:"shifts"`
	RuleJSON    string `json:"rule_json"`
	Status      int    `json:"status"`
}

type Holiday struct {
	ID          string `json:"id"`
	HolidayDate string `json:"holiday_date"`
	Name        string `json:"name"`
	IsWorkday   int    `json:"is_workday"`
	Remark      string `json:"remark"`
}

func Bind(ctx context.Context, group *ghttp.RouterGroup) {
	group.Group("/api/admin/hrm", func(api *ghttp.RouterGroup) {
		api.Middleware(hrmReadScope)
		api.GET("/summary", summary)
		api.GET("/options", options)
		api.GET("/employees", listEmployees)
		api.POST("/employees", createEmployee)
		api.GET("/attendance/groups", listGroups)
		api.POST("/attendance/groups", createGroup)
		api.GET("/attendance/holidays", listHolidays)
		api.POST("/attendance/holidays", createHoliday)
		api.GET("/attendance/clocks", listClocks)
		api.GET("/attendance/clocks/options", clockOptions)
		api.POST("/attendance/clocks", createClock)
		api.GET("/attendance/leaves", listLeaves)
		api.POST("/attendance/leaves", createLeave)
		api.GET("/attendance/outs", listOuts)
		api.POST("/attendance/outs", createOut)
		api.GET("/attendance/trips", func(r *ghttp.Request) { listAbsences(r, "trip") })
		api.POST("/attendance/trips", createOut)
		for _, entry := range []struct{ path, kind string }{{"leaves", "leave"}, {"outs", "out"}, {"trips", "trip"}} {
			kind := entry.kind
			api.GET("/attendance/"+entry.path+"/:id", func(r *ghttp.Request) { absenceDetail(r, kind) })
		}
		api.GET("/attendance/month", monthSummary)
		bindAttendanceV2(api)
	})
}

func auth(r *ghttp.Request) bool {
	if core.UID(r.Context()) <= 0 {
		r.Response.WriteStatus(http.StatusUnauthorized)
		return false
	}
	return true
}

func summary(r *ghttp.Request) {
	if !auth(r) {
		return
	}
	ctx := r.Context()
	counts := map[string]int{}
	for key, table := range map[string]string{"employees": "biz_hrm_employee", "clocks": "biz_hrm_attendance_clock", "leaves": "biz_hrm_attendance_leave", "outs": "biz_hrm_attendance_out", "trips": "biz_hrm_attendance_trip"} {
		field := "employee_id"
		if key == "employees" {
			field = "id"
		}
		query := queryScope(ctx).apply(core.Model(table).Ctx(ctx), field)
		if key == "employees" {
			var err error
			query, err = currentEmployees(ctx, query)
			if err != nil {
				r.Response.WriteJson(gf.Failed().SetMsg("无法读取员工过滤规则"))
				return
			}
		}
		n, err := query.Count()
		if err != nil {
			r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
			return
		}
		counts[key] = n
	}
	r.Response.WriteJson(gf.Success().SetData(counts))
}

func listEmployees(r *ghttp.Request) { employeeListRecords(r) }

func createEmployee(r *ghttp.Request) {
	if !auth(r) {
		return
	}
	var in Employee
	if err := r.Parse(&in); err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	if err := manualEmployeeChangeAllowed(r.Context(), in); err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	id, err := UpsertEmployee(r.Context(), in)
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	r.Response.WriteJson(gf.Success().SetData(g.Map{"id": id}))
}

func listGroups(r *ghttp.Request) { scopedGroupRecords(r) }

func createGroup(r *ghttp.Request) {
	if !auth(r) {
		return
	}
	var in AttendanceGroup
	if err := r.Parse(&in); err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	if in.Name == "" {
		r.Response.WriteJson(gf.Failed().SetMsg("name is required"))
		return
	}
	if in.ID == "" {
		in.ID = guid.S()
	}
	if err := validateGroup(in); err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	var employeeIDs []string
	_ = json.Unmarshal([]byte(in.EmployeeIDs), &employeeIDs)
	for _, id := range employeeIDs {
		if err := manualEmployeeAllowed(r.Context(), id); err != nil {
			r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
			return
		}
	}
	stamp := now()
	data := g.Map{"id": in.ID, "name": in.Name, "dept_ids": in.DeptIDs, "employee_ids": in.EmployeeIDs, "shifts": in.Shifts, "rule_json": in.RuleJSON, "status": in.Status, "updated_at": stamp}
	n, err := core.Model("biz_hrm_attendance_group").Ctx(r.Context()).Where("id", in.ID).Count()
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	if n > 0 {
		_, err = core.Model("biz_hrm_attendance_group").Ctx(r.Context()).Where("id", in.ID).Update(data)
	} else {
		data["created_at"] = stamp
		_, err = core.Model("biz_hrm_attendance_group").Ctx(r.Context()).Data(data).Insert()
	}
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	r.Response.WriteJson(gf.Success().SetData(data))
}

func listHolidays(r *ghttp.Request) {
	listRecords(r, "biz_hrm_attendance_holiday", "holiday_date", "holiday_date")
}

func createHoliday(r *ghttp.Request) {
	if !auth(r) {
		return
	}
	var in Holiday
	if err := r.Parse(&in); err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	if _, err := time.Parse("2006-01-02", in.HolidayDate); err != nil || (in.IsWorkday != 0 && in.IsWorkday != 1) {
		r.Response.WriteJson(gf.Failed().SetMsg("日期必须为 YYYY-MM-DD，is_workday 为 0 或 1"))
		return
	}
	if in.ID == "" {
		in.ID = guid.S()
	}
	stamp := now()
	data := g.Map{"id": in.ID, "holiday_date": in.HolidayDate, "name": in.Name, "is_workday": in.IsWorkday, "remark": in.Remark, "updated_at": stamp}
	row, err := core.Model("biz_hrm_attendance_holiday").Ctx(r.Context()).Where("holiday_date", in.HolidayDate).One()
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg("节假日读取失败"))
		return
	}
	if row != nil && len(row) > 0 {
		data["id"] = row["id"].String()
		_, err = core.Model("biz_hrm_attendance_holiday").Ctx(r.Context()).Where("holiday_date", in.HolidayDate).Update(data)
	} else {
		data["created_at"] = stamp
		_, err = core.Model("biz_hrm_attendance_holiday").Ctx(r.Context()).Data(data).Insert()
	}
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	r.Response.WriteJson(gf.Success().SetData(data))
}

func createClock(r *ghttp.Request) {
	if !auth(r) {
		return
	}
	var in Clock
	if err := r.Parse(&in); err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	if !manualEditAllowed(r, in.ID) {
		return
	}
	if err := manualEmployeeAllowed(r.Context(), in.EmployeeID); err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	in.Source = "manual"
	in.ExternalRef = ""
	id, err := UpsertClock(r.Context(), in)
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	r.Response.WriteJson(gf.Success().SetData(g.Map{"id": id}))
}

func listLeaves(r *ghttp.Request) {
	listAbsences(r, "leave")
}

func createLeave(r *ghttp.Request) {
	if !auth(r) {
		return
	}
	var in Leave
	if err := r.Parse(&in); err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	if !manualEditAllowed(r, in.ID) {
		return
	}
	if err := manualEmployeeAllowed(r.Context(), in.EmployeeID); err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	in.Source = "manual"
	in.ExternalRef = ""
	if in.ApprovalStatus == "" {
		in.ApprovalStatus = "approved"
	}
	id, err := UpsertLeave(r.Context(), in)
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	r.Response.WriteJson(gf.Success().SetData(g.Map{"id": id}))
}

func listOuts(r *ghttp.Request) { listAbsences(r, "out") }

func createOut(r *ghttp.Request) {
	if !auth(r) {
		return
	}
	var in Out
	if err := r.Parse(&in); err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	if !manualEditAllowed(r, in.ID) {
		return
	}
	if err := manualEmployeeAllowed(r.Context(), in.EmployeeID); err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	in.Source = "manual"
	in.ExternalRef = ""
	if in.ApprovalStatus == "" {
		in.ApprovalStatus = "approved"
	}
	save := UpsertOut
	if strings.HasSuffix(r.URL.Path, "/trips") {
		save = UpsertTrip
	}
	id, err := save(r.Context(), in)
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	r.Response.WriteJson(gf.Success().SetData(g.Map{"id": id}))
}

func options(r *ghttp.Request) {
	employeeOptions(r)
}
func monthSummary(r *ghttp.Request) {
	if !auth(r) {
		return
	}
	month := r.GetQuery("month").String()
	if month == "" {
		month = time.Now().In(core.Location).Format("2006-01")
	}
	if published, exists, e := attPublishedMonth(r.Context(), month, r.GetQuery("employee_id").String()); e != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(e.Error()))
		return
	} else if exists {
		r.Response.WriteJson(gf.Success().SetData(published))
		return
	}
	result, err := attLocalTrialMonth(r.Context(), month, r.GetQuery("employee_id").String())
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg(err.Error()))
		return
	}
	r.Response.WriteJson(gf.Success().SetData(result))
}
func now() string { return core.Now() }
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func externalID(kind, ref string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(kind+":"+ref)))
}

var writeMu sync.Mutex

func UpsertEmployee(ctx context.Context, in Employee) (string, error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	in.Name = strings.TrimSpace(in.Name)
	in.WeComUserID = strings.TrimSpace(in.WeComUserID)
	if in.UserID < 0 || in.DeptID < 0 {
		return "", fmt.Errorf("宿主用户和部门ID不可为负数")
	}
	if in.Name == "" {
		return "", fmt.Errorf("员工姓名不能为空")
	}
	row, err := core.Model("biz_hrm_employee").Ctx(ctx).Where("id", in.ID).One()
	if err != nil {
		return "", err
	}
	if in.WeComUserID != "" {
		matched, e := core.Model("biz_hrm_employee").Ctx(ctx).Where("wecom_userid", in.WeComUserID).One()
		if e != nil {
			return "", e
		}
		if len(matched) > 0 {
			if in.ID != "" && in.ID != matched["id"].String() {
				return "", fmt.Errorf("企业微信账号已绑定其他员工")
			}
			row = matched
		}
	}
	if len(row) > 0 {
		in.ID = row["id"].String()
		if in.WeComUserID != "" {
			if in.UserID == 0 {
				in.UserID = row["user_id"].Int64()
			}
			if in.DeptID == 0 {
				in.DeptID = row["dept_id"].Int64()
			}
			if in.EmployeeNo == "" {
				in.EmployeeNo = row["employee_no"].String()
			}
			if in.EntryDate == "" {
				in.EntryDate = row["entry_date"].String()
			}
			if in.QuitDate == "" {
				in.QuitDate = row["quit_date"].String()
			}
		}
	}
	if in.ID == "" {
		if in.WeComUserID != "" {
			in.ID = externalID("employee", in.WeComUserID)
		} else {
			in.ID = guid.S()
		}
	}
	if in.UserID > 0 {
		ok, e := core.HostModel("admin").Ctx(ctx).Where("id", in.UserID).Exist()
		if e != nil {
			return "", e
		}
		if !ok {
			return "", fmt.Errorf("宿主用户不存在或已删除")
		}
		existing, e := core.Model("biz_hrm_employee").Ctx(ctx).Where("user_id", in.UserID).WhereNot("id", in.ID).Count()
		if e != nil {
			return "", e
		}
		if existing > 0 {
			return "", fmt.Errorf("宿主用户已绑定其他员工")
		}
	}
	if in.DeptID > 0 {
		ok, e := core.HostModel("auth_dept").Ctx(ctx).Where("id", in.DeptID).Exist()
		if e != nil {
			return "", e
		}
		if !ok {
			return "", fmt.Errorf("宿主部门不存在")
		}
	}
	for _, date := range []string{in.EntryDate, in.QuitDate} {
		if date != "" {
			if _, e := time.ParseInLocation("2006-01-02", date, core.Location); e != nil {
				return "", fmt.Errorf("入职离职日期格式应为 YYYY-MM-DD")
			}
		}
	}
	in.Status = defaultString(in.Status, "active")
	if in.Status != "active" && in.Status != "inactive" && in.Status != "resigned" {
		return "", fmt.Errorf("无效员工状态")
	}
	if in.EntryDate != "" && in.QuitDate != "" && in.QuitDate < in.EntryDate {
		return "", fmt.Errorf("离职日期不能早于入职日期")
	}
	var userID any
	if in.UserID > 0 {
		userID = in.UserID
	}
	data := g.Map{"id": in.ID, "user_id": userID, "wecom_userid": nullable(in.WeComUserID), "employee_no": nullable(in.EmployeeNo), "name": in.Name, "dept_id": in.DeptID, "mobile": in.Mobile, "email": in.Email, "position": in.Position, "status": in.Status, "entry_date": in.EntryDate, "quit_date": in.QuitDate, "updated_at": now()}
	return in.ID, saveByID(ctx, "biz_hrm_employee", in.ID, data)
}
func employeeExists(ctx context.Context, id string) error {
	ok, err := core.Model("biz_hrm_employee").Ctx(ctx).Where("id", id).Exist()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("员工未匹配，先同步/绑定通讯录")
	}
	return nil
}
func UpsertClock(ctx context.Context, in Clock) (string, error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	if err := employeeExists(ctx, in.EmployeeID); err != nil {
		return "", err
	}
	t, err := core.ParseTime(in.ClockTime)
	if err != nil {
		return "", err
	}
	if in.ClockType != "in" && in.ClockType != "out" && in.ClockType != "outside" {
		return "", fmt.Errorf("clock_type 应为 in/out/outside")
	}
	in.Status = defaultString(in.Status, "normal")
	if in.Status != "normal" && in.Status != "late" && in.Status != "early" && in.Status != "missing" && in.Status != "abnormal" {
		return "", fmt.Errorf("无效打卡状态")
	}
	in.Source = defaultString(in.Source, "manual")
	if in.Source != "manual" && in.Source != "wecom" && in.Source != "ruoyi" {
		return "", fmt.Errorf("无效打卡来源")
	}
	if in.ExternalRef != "" {
		in.ID = externalID("clock:"+in.Source, in.ExternalRef)
	} else if in.Source != "manual" {
		return "", fmt.Errorf("外部打卡需 external_ref")
	} else if in.ID == "" {
		in.ID = guid.S()
	}
	data := g.Map{"id": in.ID, "employee_id": in.EmployeeID, "wecom_userid": in.WeComUserID, "clock_time": t.Format("2006-01-02 15:04:05"), "clock_type": in.ClockType, "status": defaultString(in.Status, "normal"), "source": in.Source, "external_ref": nullable(in.ExternalRef), "latitude": in.Latitude, "longitude": in.Longitude, "address": in.Address, "raw_json": in.RawJSON, "updated_at": now()}
	return in.ID, attSaveEvidence(ctx, "biz_hrm_attendance_clock", in.ID, data, in.Source, t, t)
}
func UpsertLeave(ctx context.Context, in Leave) (string, error) {
	return upsertAbsence(ctx, "leave", in.ID, in.EmployeeID, in.Source, in.ExternalRef, in.ApprovalStatus, in.StartTime, in.EndTime, in.Duration, in.Reason, in.RawJSON, g.Map{"leave_type": in.LeaveType})
}
func UpsertOut(ctx context.Context, in Out) (string, error) {
	return upsertAbsence(ctx, "out", in.ID, in.EmployeeID, in.Source, in.ExternalRef, in.ApprovalStatus, in.StartTime, in.EndTime, in.Duration, in.Reason, in.RawJSON, g.Map{"destination": in.Destination})
}
func UpsertTrip(ctx context.Context, in Out) (string, error) {
	return upsertAbsence(ctx, "trip", in.ID, in.EmployeeID, in.Source, in.ExternalRef, in.ApprovalStatus, in.StartTime, in.EndTime, in.Duration, in.Reason, in.RawJSON, g.Map{"destination": in.Destination})
}
func UpsertOvertime(ctx context.Context, in Out) (string, error) {
	return upsertAbsence(ctx, "overtime", in.ID, in.EmployeeID, in.Source, in.ExternalRef, in.ApprovalStatus, in.StartTime, in.EndTime, in.Duration, in.Reason, in.RawJSON, g.Map{"destination": in.Destination})
}
func upsertAbsence(ctx context.Context, kind, id, employee, source, ref, status, start, end string, duration float64, reason, raw string, extra g.Map) (string, error) {
	writeMu.Lock()
	defer writeMu.Unlock()
	if err := employeeExists(ctx, employee); err != nil {
		return "", err
	}
	from, e := core.ParseTime(start)
	if e != nil {
		return "", e
	}
	to, e := core.ParseTime(end)
	if e != nil {
		return "", e
	}
	if !to.After(from) {
		return "", fmt.Errorf("结束时间必须晚于开始时间")
	}
	if math.IsNaN(duration) || math.IsInf(duration, 0) || duration < 0 || duration > to.Sub(from).Hours()+0.01 {
		return "", fmt.Errorf("时长必须为有效小时数且不超过申请区间")
	}
	source = defaultString(source, "manual")
	if source != "manual" && source != "wecom" && source != "ruoyi" {
		return "", fmt.Errorf("无效记录来源")
	}
	status = defaultString(status, "approved")
	if status != "pending" && status != "approved" && status != "rejected" && status != "canceled" {
		return "", fmt.Errorf("无效审批状态")
	}
	if ref != "" {
		id = externalID(kind+":"+source, ref)
	} else if source != "manual" {
		return "", fmt.Errorf("外部审批需 external_ref")
	} else if id == "" {
		id = guid.S()
	}
	data := g.Map{"id": id, "employee_id": employee, "source": source, "external_ref": nullable(ref), "approval_status": status, "start_time": from.Format("2006-01-02 15:04:05"), "end_time": to.Format("2006-01-02 15:04:05"), "duration": duration, "reason": reason, "raw_json": raw, "updated_at": now()}
	for k, v := range extra {
		data[k] = v
	}
	return id, attSaveEvidence(ctx, "biz_hrm_attendance_"+kind, id, data, source, from, to)
}
func saveByID(ctx context.Context, table, id string, data g.Map) error {
	row, err := core.Model(table).Ctx(ctx).Where("id", id).One()
	if err != nil {
		return err
	}
	if len(row) > 0 {
		_, err = core.Model(table).Ctx(ctx).Where("id", id).Update(data)
	} else {
		data["created_at"] = now()
		_, err = core.Model(table).Ctx(ctx).Data(data).Insert()
	}
	return err
}
func parseTime(value string) string {
	parsed, err := core.ParseTime(value)
	if err != nil {
		return ""
	}
	return parsed.Format("2006-01-02 15:04:05")
}
func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
func validateGroup(in AttendanceGroup) error {
	var depts []int64
	var employees []string
	var shifts []Shift
	var rule Rule
	for _, field := range []struct {
		raw  string
		dest any
	}{{in.DeptIDs, &depts}, {in.EmployeeIDs, &employees}, {in.Shifts, &shifts}, {in.RuleJSON, &rule}} {
		if err := json.Unmarshal([]byte(field.raw), field.dest); err != nil {
			return fmt.Errorf("考勤组JSON格式无效: %w", err)
		}
	}
	if in.Status != 0 && in.Status != 1 {
		return fmt.Errorf("无效考勤组状态")
	}
	if len(shifts) == 0 {
		return fmt.Errorf("请配置至少一个班次")
	}
	if rule.LateGraceMinutes < 0 || rule.EarlyGraceMinutes < 0 {
		return fmt.Errorf("宽限分钟不可为负数")
	}
	for _, shift := range shifts {
		if err := shift.Validate(); err != nil {
			return err
		}
	}
	// Reject overlapping shifts; otherwise the same work interval is counted twice.
	base := time.Date(2026, 1, 5, 0, 0, 0, 0, core.Location)
	for day := 0; day < 7; day++ {
		windows := []span{}
		for offset := -1; offset <= 0; offset++ {
			for _, shift := range shifts {
				for _, weekday := range shift.Weekdays {
					if weekday == ((day+offset+7)%7)+1 {
						windows = append(windows, shift.window(base.AddDate(0, 0, day+offset)))
						break
					}
				}
			}
		}
		for i := range windows {
			for j := 0; j < i; j++ {
				if _, ok := intersect(windows[i], windows[j]); ok {
					return fmt.Errorf("班次时间不可重叠")
				}
			}
		}
	}
	return nil
}
