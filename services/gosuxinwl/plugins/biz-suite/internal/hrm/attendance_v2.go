package hrm

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// One worker owns these records. Transactions plus this mutex serialize review,
// publishing and ledger operations, including jobs started by synchronization.
var attMu sync.Mutex

type AttCycle struct {
	Mode     string           `json:"mode"`
	Anchor   string           `json:"anchor"`
	Days     []string         `json:"days"`
	Weekdays []AttCalendarDay `json:"weekdays"`
}
type AttCalendarDay struct {
	CandidateShiftIDs *[]string    `json:"candidate_shift_ids,omitempty"`
	ShiftID           string       `json:"shift_id"`
	Category          eng.Category `json:"category"`
	ScheduledWork     bool         `json:"scheduled_work"`
}
type AttConfig struct {
	WorkSchedule   *AttWorkSchedule          `json:"work_schedule,omitempty"`
	ShiftSelection string                    `json:"shift_selection,omitempty"`
	Rule           eng.Rule                  `json:"rule"`
	Shifts         []eng.Shift               `json:"shifts"`
	Cycle          AttCycle                  `json:"cycle"`
	Calendar       map[string]AttCalendarDay `json:"calendar"`
	Category       string                    `json:"category"`
}
type AttScope struct {
	EmployeeIDs     []string `json:"employee_ids"`
	DepartmentIDs   []int64  `json:"department_ids"`
	IncludeChildren bool     `json:"include_children"`
	IsDefault       bool     `json:"is_default"`
}
type AttRule struct {
	ID            string          `json:"id"`
	FamilyID      string          `json:"family_id"`
	Version       int             `json:"version"`
	Name          string          `json:"name"`
	Status        string          `json:"status"`
	EffectiveFrom string          `json:"effective_from"`
	EffectiveTo   string          `json:"effective_to"`
	Config        AttConfig       `json:"config"`
	Scope         AttScope        `json:"scope"`
	Conflicts     []string        `json:"conflicts"`
	Source        json.RawMessage `json:"source"`
}
type AttEmployee struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	EmployeeNo string `json:"employee_no"`
	DeptID     int64  `json:"dept_id"`
	Department string `json:"department"`
	Category   string `json:"category"`
}
type AttRoster struct {
	Issue         string       `json:"issue,omitempty"`
	ID            string       `json:"id"`
	EmployeeID    string       `json:"employee_id"`
	WorkDate      string       `json:"work_date"`
	RuleID        string       `json:"rule_id"`
	ShiftID       string       `json:"shift_id"`
	Status        string       `json:"status"`
	Source        string       `json:"source"`
	Category      eng.Category `json:"category"`
	Employee      AttEmployee  `json:"employee"`
	ScheduledWork bool         `json:"scheduled_work"`
}

func attJSON(v any) string                 { b, _ := json.Marshal(v); return string(b) }
func attError(r *ghttp.Request, err error) { r.Response.WriteJson(gf.Failed().SetMsg(err.Error())) }
func attOK(r *ghttp.Request, v any)        { r.Response.WriteJson(gf.Success().SetData(v)) }
func attDate(s string) (time.Time, error) {
	t, e := time.ParseInLocation("2006-01-02", s, core.Location)
	if e != nil {
		return t, fmt.Errorf("日期必须为YYYY-MM-DD")
	}
	return t, nil
}
func attRange(start, end string) ([]string, error) {
	a, e := attDate(start)
	if e != nil {
		return nil, e
	}
	b, e := attDate(end)
	if e != nil || b.Before(a) || b.Sub(a) > 366*24*time.Hour {
		return nil, fmt.Errorf("日期范围无效或超过一年")
	}
	out := []string{}
	for d := a; !d.After(b); d = d.AddDate(0, 0, 1) {
		out = append(out, d.Format("2006-01-02"))
	}
	return out, nil
}
func attAudit(ctx context.Context, entity, id, action, reason string, before, after any) error {
	_, e := core.Model("biz_att_audit").Ctx(ctx).Data(g.Map{"id": guid.S(), "entity": entity, "entity_id": id, "action": action, "before_json": attJSON(before), "after_json": attJSON(after), "reason": reason, "created_by": core.UID(ctx), "created_at": core.Now()}).Insert()
	return e
}
func attLocked(ctx context.Context, date string) bool {
	v, e := core.Model("biz_att_month").Ctx(ctx).Where("month", date[:7]).Value("status")
	return e != nil || v.String() == "locked"
}
func attManager(r *ghttp.Request) bool {
	if !core.Has(r.Context(), "biz-suite.hrm.write") {
		r.Response.WriteStatus(http.StatusForbidden)
		return false
	}
	return true
}

func bindAttendanceV2(api *ghttp.RouterGroup) {
	api.Group("/attendance/v2", func(v *ghttp.RouterGroup) {
		v.Middleware(func(r *ghttp.Request) {
			scope, e := loadHRMScope(r.Context())
			if e != nil {
				r.Response.WriteStatus(403)
				r.ExitAll()
				return
			}
			r.SetCtx(context.WithValue(r.Context(), hrmScopeKey{}, scope))
			if r.Method != "GET" && !attManager(r) {
				r.ExitAll()
				return
			}
			r.Middleware.Next()
		})
		v.GET("/rules", attListRules)
		v.POST("/rules", attSaveRule)
		v.POST("/rules/:id/publish", attPublishRule)
		v.POST("/rules/:id/preview", attRulePreview)
		v.POST("/rules/:id/apply", attRuleApply)
		v.GET("/rosters", attListRosters)
		v.POST("/rosters", attSaveRosters)
		v.POST("/rosters/generate", attGenerateRosters)
		v.POST("/rosters/candidates", attCandidates)
		v.GET("/batches", attListBatches)
		v.POST("/batches", attCreateBatch)
		v.GET("/batches/:id", attBatchDetail)
		v.GET("/results", attResults)
		v.GET("/results/:id", attResultDetail)
		v.GET("/months", attMonths)
		v.POST("/months/:month/action", attMonthAction)
		v.POST("/months/:month/close", attMonthClose)
		v.POST("/adjustments", attAdjustment)
		v.GET("/adjustments", attAdjustments)
		v.GET("/ledger", attLedger)
		v.POST("/ledger", attLedgerWrite)
		v.GET("/imports", attImports)
		v.POST("/imports", attImportUpload)
		v.POST("/imports/wecom", attImportWeCom)
		v.GET("/imports/:id", attImportDetail)
		v.POST("/imports/:id/mappings", attImportMappings)
		v.POST("/imports/:id/rules", attImportRules)
		v.GET("/imports/:id/compare", attImportCompare)
		v.GET("/export", attExport)
		v.GET("/audit", attAuditList)
		v.GET("/overtime", func(r *ghttp.Request) { listRecords(r, "biz_hrm_attendance_overtime", "start_time", "reason") })
		v.POST("/overtime", func(r *ghttp.Request) {
			var in Out
			if e := r.Parse(&in); e != nil {
				attError(r, e)
				return
			}
			if in.Source != "" && in.Source != "manual" {
				attError(r, fmt.Errorf("只能维护人工加班凭据"))
				return
			}
			if in.ID != "" {
				old, _ := core.Model("biz_hrm_attendance_overtime").Ctx(r.Context()).Where("id", in.ID).One()
				if len(old) > 0 && old["source"].String() != "manual" {
					attError(r, fmt.Errorf("同步审批需在企业微信修改"))
					return
				}
			}
			if e := manualEmployeeAllowed(r.Context(), in.EmployeeID); e != nil {
				attError(r, e)
				return
			}
			in.Source = "manual"
			in.ExternalRef = ""
			id, e := UpsertOvertime(r.Context(), in)
			if e != nil {
				attError(r, e)
				return
			}
			attOK(r, g.Map{"id": id})
		})
	})
}
func attDecodeRule(row gdb.Record) AttRule {
	var a AttRule
	a.ID = row["id"].String()
	a.FamilyID = row["family_id"].String()
	a.Version = row["version"].Int()
	a.Name = row["name"].String()
	a.Status = row["status"].String()
	a.EffectiveFrom = row["effective_from"].String()
	a.EffectiveTo = row["effective_to"].String()
	_ = json.Unmarshal([]byte(row["config_json"].String()), &a.Config)
	_ = json.Unmarshal([]byte(row["scope_json"].String()), &a.Scope)
	_ = json.Unmarshal([]byte(row["conflicts_json"].String()), &a.Conflicts)
	a.Source = json.RawMessage(row["source_json"].String())
	return a
}
func attLoadRule(ctx context.Context, id string) (AttRule, error) {
	row, e := core.Model("biz_att_rule").Ctx(ctx).Where("id", id).One()
	if e != nil {
		return AttRule{}, e
	}
	if len(row) == 0 {
		return AttRule{}, fmt.Errorf("考勤制度不存在")
	}
	return attDecodeRule(row), nil
}
func attRuleSourceEditable(a AttRule) error {
	var source map[string]any
	_ = json.Unmarshal(a.Source, &source)
	if a.Status == "archived" || a.Status == "superseded" || source["snapshot_id"] != nil {
		return fmt.Errorf("旧来源制度仅保留审计，不可编辑或重新生效；请在考勤设置建立新的本地制度")
	}
	return nil
}
func attListRules(r *ghttp.Request) {
	if !queryScope(r.Context()).All {
		r.Response.WriteStatus(403)
		return
	}
	rows, e := core.Model("biz_att_rule").Ctx(r.Context()).OrderDesc("created_at").All()
	if e != nil {
		attError(r, e)
		return
	}
	out := []AttRule{}
	for _, x := range rows {
		out = append(out, attDecodeRule(x))
	}
	attOK(r, g.Map{"items": out})
}
func validateAttConfig(a AttRule) error {
	if a.Config.ShiftSelection != "" && a.Config.ShiftSelection != "fixed" && a.Config.ShiftSelection != "auto" {
		return fmt.Errorf("班次识别方式无效")
	}
	if a.Config.Category != "员工" && a.Config.Category != "生产职员" && a.Config.Category != "职员" && a.Config.Category != "" {
		return fmt.Errorf("考勤分类必须为员工、生产职员或职员")
	}
	if strings.TrimSpace(a.Name) == "" {
		return fmt.Errorf("请填写制度名称")
	}
	if strings.TrimSpace(a.EffectiveFrom) == "" {
		return fmt.Errorf("制度尚未填写生效日期，请先在“编辑 / 新版本”中完善后再发布")
	}
	if _, e := attDate(a.EffectiveFrom); e != nil {
		return fmt.Errorf("制度生效日期无效，请在“编辑 / 新版本”中填写有效日期（YYYY-MM-DD）")
	}
	if a.EffectiveTo != "" {
		if _, e := attDate(a.EffectiveTo); e != nil {
			return fmt.Errorf("制度结束日期无效，请填写有效日期（YYYY-MM-DD）或留空")
		}
		if a.EffectiveTo < a.EffectiveFrom {
			return fmt.Errorf("制度结束日期不能早于生效日期，请先编辑制度")
		}
	}
	if len(a.Conflicts) > 0 {
		return fmt.Errorf("制度仍有待确认事项，请在“编辑 / 新版本”中逐项核对后再发布")
	}
	if len(a.Config.Shifts) == 0 {
		return fmt.Errorf("请设置班次")
	}
	if e := eng.ValidateRule(a.Config.Rule); e != nil {
		return e
	}
	ids := map[string]bool{}
	for _, s := range a.Config.Shifts {
		if s.ID == "" || ids[s.ID] {
			return fmt.Errorf("班次编号不能为空或重复")
		}
		if e := eng.ValidateShift(s); e != nil {
			return e
		}
		if a.Config.Rule.EvidenceMode == "local" {
			if e := eng.ValidateLocalShift(s); e != nil {
				return fmt.Errorf("待配置：%w", e)
			}
		}
		ids[s.ID] = true
	}
	if e := attValidateWorkSchedule(a.Config, ids); e != nil {
		return e
	}
	cycle, e := attWorkScheduleCycle(a.Config)
	if e != nil {
		return e
	}
	a.Config.Cycle = cycle
	switch a.Config.Cycle.Mode {
	case "weekly":
		if len(a.Config.Cycle.Weekdays) != 7 {
			return fmt.Errorf("按星期排班必须完整配置周一至周日7天")
		}
		weekdayNames := []string{"周一", "周二", "周三", "周四", "周五", "周六", "周日"}
		for i, day := range a.Config.Cycle.Weekdays {
			if err := attValidateDayCandidates(day, ids); err != nil {
				return err
			}
			if day.Category != eng.Workday && day.Category != eng.Restday && day.Category != eng.Holiday {
				return fmt.Errorf("%s的日期类型无效，请选择工作日、休息日或节假日", weekdayNames[i])
			}
			if day.ShiftID != "" && !ids[day.ShiftID] {
				return fmt.Errorf("%s引用了不存在的班次，请重新选择", weekdayNames[i])
			}
			if (day.Category == eng.Workday || day.ScheduledWork) && day.ShiftID == "" && a.Config.ShiftSelection != "auto" && (day.CandidateShiftIDs == nil || len(*day.CandidateShiftIDs) == 0) {
				return fmt.Errorf("%s要求出勤，请先选择班次", weekdayNames[i])
			}
		}
	case "", "cycle":
		if strings.TrimSpace(a.Config.Cycle.Anchor) == "" {
			return fmt.Errorf("排班周期尚未填写周期第一天，请先在“编辑 / 新版本”中完善")
		}
		if _, e := attDate(a.Config.Cycle.Anchor); e != nil {
			return fmt.Errorf("排班周期的周期第一天无效，请填写有效日期（YYYY-MM-DD）")
		}
		if len(a.Config.Cycle.Days) == 0 || len(a.Config.Cycle.Days) > 366 {
			return fmt.Errorf("排班周期须配置1至366天，请在“编辑 / 新版本”中设置周期及每天的班次")
		}
		for _, id := range a.Config.Cycle.Days {
			if id != "" && !ids[id] {
				return fmt.Errorf("周期引用了不存在的班次")
			}
		}
	default:
		return fmt.Errorf("排班模式无效，请选择按星期排班或按周期轮班")
	}
	for day, c := range a.Config.Calendar {
		if err := attValidateDayCandidates(c, ids); err != nil {
			return err
		}
		if _, e := attDate(day); e != nil {
			return fmt.Errorf("节假日 / 单日覆盖中有无效日期，请填写有效日期（YYYY-MM-DD）")
		}
		if c.ShiftID != "" && !ids[c.ShiftID] {
			return fmt.Errorf("日期覆盖引用了不存在的班次")
		}
		if c.Category != "workday" && c.Category != "restday" && c.Category != "holiday" {
			return fmt.Errorf("日历类型无效")
		}
	}
	if len(a.Scope.EmployeeIDs) == 0 && len(a.Scope.DepartmentIDs) == 0 && !a.Scope.IsDefault {
		return fmt.Errorf("制度尚未配置适用范围，请先编辑并选择人员、部门或明确设置为默认制度")
	}
	if attHasWorkSchedule(a.Config) && a.Config.WorkSchedule.Mode == "after_sales" && (len(a.Scope.EmployeeIDs) == 0 || len(a.Scope.DepartmentIDs) > 0 || a.Scope.IsDefault) {
		return fmt.Errorf("售后留守须明确选择本组人员，不使用整部门或默认范围")
	}
	if attHasWorkSchedule(a.Config) && a.Config.WorkSchedule.Mode == "after_sales" {
		members := map[string]bool{}
		for _, id := range a.Scope.EmployeeIDs {
			if strings.TrimSpace(id) != "" {
				members[id] = true
			}
		}
		if len(members) != a.Config.WorkSchedule.DutyExpectedMembers || len(members) != len(a.Scope.EmployeeIDs) {
			return fmt.Errorf("售后留守组应有%d名不同人员，请确认完整人员后生效", a.Config.WorkSchedule.DutyExpectedMembers)
		}
	}
	return nil
}
func attSaveRule(r *ghttp.Request) {
	if !queryScope(r.Context()).All {
		r.Response.WriteStatus(http.StatusForbidden)
		return
	}
	var in AttRule
	// Decode the JSON body directly: the generic request converter drops
	// json.RawMessage objects, which would erase imported source evidence.
	if e := json.Unmarshal(r.GetBody(), &in); e != nil {
		attError(r, fmt.Errorf("制度请求必须为有效的JSON对象"))
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		attError(r, fmt.Errorf("请填写制度名称"))
		return
	}
	attMu.Lock()
	defer attMu.Unlock()
	ctx := r.Context()
	var old AttRule
	if in.ID != "" {
		var e error
		old, e = attLoadRule(ctx, in.ID)
		if e != nil {
			attError(r, e)
			return
		}
		if e = attRuleSourceEditable(old); e != nil {
			attError(r, e)
			return
		}
		if old.Status == "published" {
			in.ID = ""
			in.FamilyID = old.FamilyID
		} else {
			in.FamilyID = old.FamilyID
			in.Version = old.Version
		}
	}
	if in.ID == "" {
		if in.FamilyID != "" {
			latest, e := core.Model("biz_att_rule").Ctx(ctx).Where("family_id", in.FamilyID).OrderDesc("version").One()
			if e != nil {
				attError(r, e)
				return
			}
			if len(latest) > 0 {
				if e = attRuleSourceEditable(attDecodeRule(latest)); e != nil {
					attError(r, e)
					return
				}
			}
		}
		in.ID = guid.S()
		if in.FamilyID == "" {
			in.FamilyID = in.ID
		}
		v, e := core.Model("biz_att_rule").Ctx(ctx).Where("family_id", in.FamilyID).Max("version")
		if e != nil {
			attError(r, e)
			return
		}
		in.Version = int(v) + 1
	}
	in.Status = "draft"
	if in.Config.Category == "" {
		in.Config.Category = "职员"
		if attHasWorkSchedule(in.Config) && attWorkScheduleCategory(in.Config.WorkSchedule.Mode) != "" {
			in.Config.Category = attWorkScheduleCategory(in.Config.WorkSchedule.Mode)
		}
	}
	cycle, e := attWorkScheduleCycle(in.Config)
	if e != nil {
		attError(r, e)
		return
	}
	in.Config.Cycle = cycle
	if attHasWorkSchedule(in.Config) {
		// The simplified work week promises automatic local calculation. A
		// migrated legacy rule must first complete its local punch windows.
		in.Config.Rule.EvidenceMode = "local"
	}
	in.Config.Rule.ID = in.ID
	in.Config.Rule.Version = fmt.Sprint(in.Version)
	if len(in.Source) == 0 {
		in.Source = old.Source
		if len(in.Source) == 0 {
			in.Source = json.RawMessage(`{}`)
		}
	}
	if e := attRuleSourceEditable(in); e != nil {
		attError(r, e)
		return
	}
	data := g.Map{"id": in.ID, "family_id": in.FamilyID, "version": in.Version, "name": in.Name, "status": "draft", "effective_from": in.EffectiveFrom, "effective_to": in.EffectiveTo, "config_json": attJSON(in.Config), "scope_json": attJSON(in.Scope), "conflicts_json": attJSON(in.Conflicts), "source_json": string(in.Source), "created_by": core.UID(ctx), "updated_at": core.Now()}
	e = core.DB().Transaction(ctx, func(c context.Context, tx gdb.TX) error {
		if old.ID == in.ID {
			if _, e := core.Model("biz_att_rule").Ctx(c).Where("id", in.ID).Update(data); e != nil {
				return e
			}
		} else {
			data["created_at"] = core.Now()
			if _, e := core.Model("biz_att_rule").Ctx(c).Data(data).Insert(); e != nil {
				return e
			}
		}
		return attAudit(c, "rule", in.ID, "save", "保存草稿", old, in)
	})
	if e != nil {
		attError(r, e)
		return
	}
	attOK(r, in)
}
func attPublishRule(r *ghttp.Request) {
	a, _, _, e := attApplyPublishedRule(r.Context(), r.GetRouter("id").String(), attRuleApplyRequest{Reason: "人事确认发布并自动计算"})
	if e != nil {
		attError(r, e)
		return
	}
	attOK(r, a)
}

func attEmployees(ctx context.Context) (map[string]AttEmployee, error) {
	q, e := currentEmployees(ctx, core.Model("biz_hrm_employee").Ctx(ctx))
	if e != nil {
		return nil, e
	}
	rows, e := queryScope(ctx).apply(q, "id").All()
	if e != nil {
		return nil, e
	}
	depts, e := core.HostModel("auth_dept").Ctx(ctx).Fields("id,name").All()
	if e != nil {
		return nil, e
	}
	names := map[int64]string{}
	for _, d := range depts {
		names[d["id"].Int64()] = d["name"].String()
	}
	out := map[string]AttEmployee{}
	for _, x := range rows {
		out[x["id"].String()] = AttEmployee{ID: x["id"].String(), Name: x["name"].String(), EmployeeNo: x["employee_no"].String(), DeptID: x["dept_id"].Int64(), Department: names[x["dept_id"].Int64()]}
	}
	return out, nil
}
func attDecodeRoster(row gdb.Record) AttRoster {
	var a AttRoster
	_ = json.Unmarshal([]byte(row["roster_json"].String()), &a)
	return a
}
func attListRosters(r *ghttp.Request) {
	q := queryScope(r.Context()).apply(core.Model("biz_att_roster").Ctx(r.Context()), "employee_id")
	if s := r.GetQuery("start_date").String(); s != "" {
		q = q.WhereGTE("work_date", s)
	}
	if s := r.GetQuery("end_date").String(); s != "" {
		q = q.WhereLTE("work_date", s)
	}
	if s := r.GetQuery("employee_id").String(); s != "" {
		q = q.Where("employee_id", s)
	}
	rows, e := q.Order("work_date,employee_id").Limit(10000).All()
	if e != nil {
		attError(r, e)
		return
	}
	out := []AttRoster{}
	for _, x := range rows {
		out = append(out, attDecodeRoster(x))
	}
	attOK(r, g.Map{"items": out})
}
func attPutRosters(ctx context.Context, items []AttRoster, reason string) error {
	employees, e := attEmployees(ctx)
	if e != nil {
		return e
	}
	return core.DB().Transaction(ctx, func(c context.Context, tx gdb.TX) error {
		for _, a := range items {
			if _, e := attDate(a.WorkDate); e != nil {
				return e
			}
			if attLocked(c, a.WorkDate) {
				return fmt.Errorf("%s已月结锁定", a.WorkDate[:7])
			}
			person, ok := employees[a.EmployeeID]
			if !ok {
				return fmt.Errorf("人员不存在、已过滤或无权限")
			}
			rule, e := attLoadRule(c, a.RuleID)
			if e != nil {
				return e
			}
			if rule.Status != "published" || a.WorkDate < rule.EffectiveFrom || (rule.EffectiveTo != "" && a.WorkDate > rule.EffectiveTo) {
				return fmt.Errorf("制度未发布或不在生效日期内")
			}
			valid := a.ShiftID == ""
			for _, s := range rule.Config.Shifts {
				valid = valid || s.ID == a.ShiftID
			}
			if !valid {
				return fmt.Errorf("班次不存在")
			}
			if a.Status != "candidate" {
				a.Status = "confirmed"
			}
			if a.Category == "" {
				if a.ShiftID == "" {
					a.Category = eng.Restday
				} else {
					a.Category = eng.Workday
				}
			}
			if a.Category != eng.Workday && a.Category != eng.Restday && a.Category != eng.Holiday {
				return fmt.Errorf("排班日期类型无效")
			}
			if a.Status == "confirmed" && a.ShiftID == "" && (a.Category == eng.Workday || a.ScheduledWork) {
				return fmt.Errorf("要求出勤的日期必须明确班次，不能保存成空班次")
			}
			a.ID = externalID("roster", a.EmployeeID+":"+a.WorkDate)
			previous, e := core.Model("biz_att_roster").Ctx(c).Where("id", a.ID).One()
			if e != nil {
				return e
			}
			if attGeneratedRosterSource(a.Source) && len(previous) > 0 && !attGeneratedRosterSource(previous["source"].String()) {
				continue
			}
			// A later transfer must not rewrite an already evidenced historical
			// department. Never accept employee snapshots supplied by a browser.
			a.Employee = person
			if old := attDecodeRoster(previous); old.Employee.ID == a.EmployeeID {
				a.Employee = old.Employee
			}
			a.Employee.Category = rule.Config.Category
			if len(previous) > 0 && attJSON(attDecodeRoster(previous)) == attJSON(a) {
				continue
			}
			data := g.Map{"id": a.ID, "employee_id": a.EmployeeID, "work_date": a.WorkDate, "rule_id": a.RuleID, "shift_id": a.ShiftID, "status": a.Status, "source": defaultString(a.Source, "manual"), "roster_json": attJSON(a), "updated_by": core.UID(ctx), "updated_at": core.Now()}
			if len(previous) == 0 {
				_, e = core.Model("biz_att_roster").Ctx(c).Data(data).Insert()
			} else {
				_, e = core.Model("biz_att_roster").Ctx(c).Where("id", a.ID).Update(data)
			}
			if e != nil {
				return e
			}
			if e = attAudit(c, "roster", a.ID, "assign", reason, attDecodeRoster(previous), a); e != nil {
				return e
			}
			if _, e = core.Model("biz_att_month").Ctx(c).Where("month", a.WorkDate[:7]).Update(g.Map{"change_pending": 1}); e != nil {
				return e
			}
		}
		return nil
	})
}
func attSaveRosters(r *ghttp.Request) {
	var in struct {
		Items  []AttRoster `json:"items"`
		Reason string      `json:"reason"`
	}
	if e := r.Parse(&in); e != nil {
		attError(r, e)
		return
	}
	if len(in.Items) == 0 || len(in.Items) > 10000 || strings.TrimSpace(in.Reason) == "" {
		attError(r, fmt.Errorf("请填写调整原因并选择1至10000条排班"))
		return
	}
	attMu.Lock()
	defer attMu.Unlock()
	if e := attPutRosters(r.Context(), in.Items, in.Reason); e != nil {
		attError(r, e)
		return
	}
	attOK(r, g.Map{"count": len(in.Items)})
}
func attGenerateRosters(r *ghttp.Request) {
	var in struct {
		StartDate   string   `json:"start_date"`
		EndDate     string   `json:"end_date"`
		EmployeeIDs []string `json:"employee_ids"`
		Preview     bool     `json:"preview"`
	}
	if e := r.Parse(&in); e != nil {
		attError(r, e)
		return
	}
	dates, e := attRange(in.StartDate, in.EndDate)
	if e != nil {
		attError(r, e)
		return
	}
	ctx := r.Context()
	people, e := attEmployees(ctx)
	if e != nil {
		attError(r, e)
		return
	}
	rows, e := core.Model("biz_att_rule").Ctx(ctx).Where("status", "published").All()
	if e != nil {
		attError(r, e)
		return
	}
	rules := []AttRule{}
	for _, x := range rows {
		rules = append(rules, attDecodeRule(x))
	}
	parents := map[int64]int64{}
	depts, e := core.HostModel("auth_dept").Ctx(ctx).Fields("id,pid").All()
	if e != nil {
		attError(r, e)
		return
	}
	for _, d := range depts {
		parents[d["id"].Int64()] = d["pid"].Int64()
	}
	wanted := map[string]bool{}
	for _, id := range in.EmployeeIDs {
		wanted[id] = true
		if _, ok := people[id]; !ok {
			attError(r, fmt.Errorf("无效人员或无权限"))
			return
		}
	}
	items := []AttRoster{}
	issues := []g.Map{}
	holidayRows, e := core.Model("biz_hrm_attendance_holiday").Ctx(ctx).WhereBetween("holiday_date", in.StartDate, in.EndDate).All()
	if e != nil {
		attError(r, e)
		return
	}
	holidays := map[string]gdb.Record{}
	for _, h := range holidayRows {
		holidays[h["holiday_date"].String()] = h
	}
	for id, p := range people {
		if len(wanted) > 0 && !wanted[id] {
			continue
		}
		for _, date := range dates {
			a, err := resolveAttRule(rules, p, date, parents)
			if err != nil {
				issues = append(issues, g.Map{"employee_id": id, "name": p.Name, "date": date, "message": err.Error()})
				continue
			}
			day, err := attCycleDay(a.Config, date)
			if err != nil {
				issues = append(issues, g.Map{"employee_id": id, "date": date, "message": err.Error()})
				continue
			}
			if _, explicit := a.Config.Calendar[date]; !explicit {
				h := holidays[date]
				if len(h) > 0 {
					if h["is_workday"].Int() == 0 {
						day.Category = eng.Holiday
						day.ScheduledWork = false
					} else {
						day.Category = eng.Workday
						day.ScheduledWork = true
						if day.ShiftID == "" {
							issues = append(issues, g.Map{"employee_id": id, "name": p.Name, "date": date, "message": "调休工作日需明确选择班次"})
							continue
						}
					}
				}
			}
			source := "cycle"
			if a.Config.Cycle.Mode == "weekly" {
				source = "weekly"
			}
			items = append(items, AttRoster{EmployeeID: id, WorkDate: date, RuleID: a.ID, ShiftID: day.ShiftID, Category: day.Category, ScheduledWork: day.ScheduledWork, Status: "confirmed", Source: source})
		}
	}
	if len(items) > 10000 {
		attError(r, fmt.Errorf("单次排班超过10000条，请缩小人员或日期范围"))
		return
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].WorkDate+items[i].EmployeeID < items[j].WorkDate+items[j].EmployeeID
	})
	if !in.Preview {
		if len(issues) > 0 {
			attOK(r, g.Map{"items": items, "issues": issues, "saved": false})
			return
		}
		attMu.Lock()
		e = attPutRosters(ctx, items, "按已发布制度生成排班")
		attMu.Unlock()
		if e != nil {
			attError(r, e)
			return
		}
	}
	attOK(r, g.Map{"items": items, "issues": issues, "saved": !in.Preview})
}
func attCycleDay(c AttConfig, date string) (AttCalendarDay, error) {
	cycle, err := attWorkScheduleCycle(c)
	if err != nil {
		return AttCalendarDay{}, err
	}
	c.Cycle = cycle
	if d, ok := c.Calendar[date]; ok {
		return d, nil
	}
	if c.Cycle.Mode == "weekly" {
		d, e := attDate(date)
		if e != nil {
			return AttCalendarDay{}, e
		}
		if len(c.Cycle.Weekdays) != 7 {
			return AttCalendarDay{}, fmt.Errorf("按星期排班必须完整配置周一至周日7天")
		}
		// Go's Sunday is zero; the persisted configuration starts on Monday.
		return c.Cycle.Weekdays[(int(d.Weekday())+6)%7], nil
	}
	if c.Cycle.Mode != "" && c.Cycle.Mode != "cycle" {
		return AttCalendarDay{}, fmt.Errorf("排班模式无效，请选择按星期排班或按周期轮班")
	}
	a, e := attDate(c.Cycle.Anchor)
	if e != nil {
		return AttCalendarDay{}, e
	}
	b, e := attDate(date)
	if e != nil || len(c.Cycle.Days) == 0 {
		return AttCalendarDay{}, fmt.Errorf("周期未配置")
	}
	n := int(b.Sub(a) / (24 * time.Hour))
	idx := ((n % len(c.Cycle.Days)) + len(c.Cycle.Days)) % len(c.Cycle.Days)
	id := c.Cycle.Days[idx]
	cat := eng.Workday
	if id == "" {
		cat = eng.Restday
	}
	return AttCalendarDay{ShiftID: id, Category: cat}, nil
}

func attGeneratedRosterSource(source string) bool {
	return source == "cycle" || source == "weekly" || source == "automatic"
}
func resolveAttRule(rules []AttRule, p AttEmployee, date string, parents map[int64]int64) (AttRule, error) {
	// New versions supersede their own family only from the new effective date.
	families := map[string]AttRule{}
	for _, a := range rules {
		if a.Status != "published" || a.EffectiveFrom > date || (a.EffectiveTo != "" && a.EffectiveTo < date) {
			continue
		}
		old, ok := families[a.FamilyID]
		if !ok || a.Version > old.Version {
			families[a.FamilyID] = a
		}
	}
	best := 100000
	matches := []AttRule{}
	for _, a := range families {
		score := 100000
		for _, id := range a.Scope.EmployeeIDs {
			if id == p.ID {
				score = 0
			}
		}
		seen := map[int64]bool{}
		dep := p.DeptID
		for distance := 0; dep > 0 && !seen[dep]; distance++ {
			seen[dep] = true
			for _, id := range a.Scope.DepartmentIDs {
				if id == dep && (distance == 0 || a.Scope.IncludeChildren) && distance+1 < score {
					score = distance + 1
				}
			}
			dep = parents[dep]
		}
		if a.Scope.IsDefault && score == 100000 {
			score = 99999
		}
		if score < best {
			best = score
			matches = []AttRule{a}
		} else if score == best && score < 100000 {
			matches = append(matches, a)
		}
	}
	if len(matches) == 0 {
		return AttRule{}, fmt.Errorf("没有适用制度")
	}
	if len(matches) > 1 {
		return AttRule{}, fmt.Errorf("同级制度冲突，需人事确认")
	}
	return matches[0], nil
}
func attAuditList(r *ghttp.Request) {
	if !queryScope(r.Context()).All {
		r.Response.WriteStatus(403)
		return
	}
	q := core.Model("biz_att_audit").Ctx(r.Context())
	if id := r.GetQuery("entity_id").String(); id != "" {
		q = q.Where("entity_id", id)
	}
	rows, e := q.OrderDesc("created_at").Limit(200).All()
	if e != nil {
		attError(r, e)
		return
	}
	attOK(r, g.Map{"items": rows})
}
