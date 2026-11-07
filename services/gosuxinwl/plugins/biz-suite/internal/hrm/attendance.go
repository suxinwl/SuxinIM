package hrm

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"math"
	"sort"
	"strings"
	"time"
)

type Shift struct {
	Name       string `json:"name"`
	Start      string `json:"start"`
	End        string `json:"end"`
	Weekdays   []int  `json:"weekdays"`
	BreakStart string `json:"break_start"`
	BreakEnd   string `json:"break_end"`
}
type Rule struct {
	LateGraceMinutes  int `json:"late_grace_minutes"`
	EarlyGraceMinutes int `json:"early_grace_minutes"`
}
type Day struct {
	Date          string  `json:"date"`
	ExpectedHours float64 `json:"expected_hours"`
	LeaveHours    float64 `json:"leave_hours"`
	OutHours      float64 `json:"out_hours"`
	TripHours     float64 `json:"trip_hours"`
	LateCount     int     `json:"late_count"`
	EarlyCount    int     `json:"early_count"`
	MissingCount  int     `json:"missing_count"`
	Absent        bool    `json:"absent"`
	Worked        bool    `json:"worked"`
	State         string  `json:"state"`
}
type MonthEmployee struct {
	EmployeeID           string  `json:"employee_id"`
	Name                 string  `json:"name"`
	EmployeeNo           string  `json:"employee_no"`
	Department           string  `json:"department"`
	ExpectedHours        float64 `json:"expected_hours"`
	ActualHours          float64 `json:"actual_hours"`
	CreditedHours        float64 `json:"credited_hours"`
	OvertimeHours        float64 `json:"overtime_hours"`
	OvertimeWorkdayHours float64 `json:"overtime_workday_hours"`
	OvertimeRestdayHours float64 `json:"overtime_restday_hours"`
	OvertimeHolidayHours float64 `json:"overtime_holiday_hours"`
	ExpectedDays         int     `json:"expected_days"`
	WorkedDays           int     `json:"worked_days"`
	LateCount            int     `json:"late_count"`
	EarlyCount           int     `json:"early_count"`
	MissingCount         int     `json:"missing_count"`
	AbsentDays           int     `json:"absent_days"`
	LeaveHours           float64 `json:"leave_hours"`
	OutHours             float64 `json:"out_hours"`
	TripHours            float64 `json:"trip_hours"`
	Days                 []Day   `json:"days"`
	Warning              string  `json:"warning"`
}
type MonthResult struct {
	Month string          `json:"month"`
	Items []MonthEmployee `json:"items"`
}
type span struct{ start, end time.Time }

func minute(s string) (int, error) {
	t, e := time.Parse("15:04", s)
	if e != nil {
		return 0, fmt.Errorf("班次时刻应为 HH:mm: %s", s)
	}
	return t.Hour()*60 + t.Minute(), nil
}
func (s Shift) Validate() error {
	a, e := minute(s.Start)
	if e != nil {
		return e
	}
	b, e := minute(s.End)
	if e != nil {
		return e
	}
	if a == b {
		return fmt.Errorf("班次起止不能相同")
	}
	if len(s.Weekdays) == 0 {
		return fmt.Errorf("班次须配置工作日（周一=1，周日=7）")
	}
	for _, d := range s.Weekdays {
		if d < 1 || d > 7 {
			return fmt.Errorf("工作日必须为1至7")
		}
	}
	if (s.BreakStart == "") != (s.BreakEnd == "") {
		return fmt.Errorf("休息起止需同时填写")
	}
	if s.BreakStart != "" {
		c, e := minute(s.BreakStart)
		if e != nil {
			return e
		}
		d, e := minute(s.BreakEnd)
		if e != nil {
			return e
		}
		if b < a {
			b += 1440
		}
		if c < a {
			c += 1440
		}
		if d <= c {
			d += 1440
		}
		if c < a || d > b {
			return fmt.Errorf("休息区间必须包含在班次内")
		}
	}
	return nil
}
func (s Shift) window(date time.Time) span {
	a, _ := minute(s.Start)
	b, _ := minute(s.End)
	if b < a {
		b += 1440
	}
	return span{date.Add(time.Duration(a) * time.Minute), date.Add(time.Duration(b) * time.Minute)}
}
func (s Shift) work(date time.Time) []span {
	w := s.window(date)
	if s.BreakStart == "" {
		return []span{w}
	}
	a, _ := minute(s.BreakStart)
	b, _ := minute(s.BreakEnd)
	begin := date.Add(time.Duration(a) * time.Minute)
	if begin.Before(w.start) {
		begin = begin.AddDate(0, 0, 1)
	}
	end := date.Add(time.Duration(b) * time.Minute)
	if !end.After(begin) {
		end = end.AddDate(0, 0, 1)
	}
	return []span{{w.start, begin}, {end, w.end}}
}
func covers(list []span, t time.Time) bool {
	for _, x := range list {
		if !t.Before(x.start) && t.Before(x.end) {
			return true
		}
	}
	return false
}
func merged(list []span) []span {
	if len(list) == 0 {
		return nil
	}
	sort.Slice(list, func(i, j int) bool { return list[i].start.Before(list[j].start) })
	out := []span{list[0]}
	for _, v := range list[1:] {
		p := &out[len(out)-1]
		if !v.start.After(p.end) {
			if v.end.After(p.end) {
				p.end = v.end
			}
		} else {
			out = append(out, v)
		}
	}
	return out
}
func intersect(a, b span) (span, bool) {
	if b.start.After(a.start) {
		a.start = b.start
	}
	if b.end.Before(a.end) {
		a.end = b.end
	}
	return a, a.end.After(a.start)
}
func hours(work, items []span) float64 {
	total := 0.0
	for _, w := range work {
		list := []span{}
		for _, x := range items {
			if part, ok := intersect(w, x); ok {
				list = append(list, part)
			}
		}
		for _, x := range merged(list) {
			total += x.end.Sub(x.start).Hours()
		}
	}
	return total
}
func round(v float64) float64 { return math.Round(v*100) / 100 }
func scheduledOn(s Shift, date time.Time) bool {
	w := int(date.Weekday())
	if w == 0 {
		w = 7
	}
	for _, d := range s.Weekdays {
		if d == w {
			return true
		}
	}
	return false
}

// Assign each punch to its nearest shift boundary to prevent reusing it.
func belongs(t time.Time, kind string, selected span, date time.Time, shifts []Shift, holiday *bool) bool {
	boundary := selected.start
	if kind == "out" {
		boundary = selected.end
	}
	distance := math.Abs(t.Sub(boundary).Seconds())
	for offset := -1; offset <= 1; offset++ {
		for _, s := range shifts {
			day := date.AddDate(0, 0, offset)
			if !scheduledOn(s, day) && !(offset == 0 && holiday != nil && *holiday) {
				continue
			}
			w := s.window(day)
			b := w.start
			if kind == "out" {
				b = w.end
			}
			other := math.Abs(t.Sub(b).Seconds())
			if other < distance || (other == distance && b.Before(boundary)) {
				return false
			}
		}
	}
	return true
}
func calculateDay(date time.Time, shifts []Shift, rule Rule, clocks []Clock, leaves, outs []span, holiday *bool, at time.Time) Day {
	return calculateDayWithTrips(date, shifts, rule, clocks, leaves, outs, nil, holiday, at)
}
func calculateDayWithTrips(date time.Time, shifts []Shift, rule Rule, clocks []Clock, leaves, outs, trips []span, holiday *bool, at time.Time) Day {
	d := Day{Date: date.Format("2006-01-02"), State: "rest"}
	weekday := int(date.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	// A make-up workday cannot infer which of mutually exclusive weekday
	// schedules the employer intends. Report ambiguity instead of double-counting.
	if holiday != nil && *holiday {
		for i, shift := range shifts {
			for j := 0; j < i; j++ {
				if _, ok := intersect(shift.window(date), shifts[j].window(date)); ok {
					d.State = "ambiguous_schedule"
					return d
				}
			}
		}
	}
	anyClock := false
	needed := 0.0
	pending := false
	for _, shift := range shifts {
		scheduled := false
		for _, day := range shift.Weekdays {
			if day == weekday {
				scheduled = true
			}
		}
		if holiday != nil {
			scheduled = *holiday
		}
		if !scheduled {
			continue
		}
		work := shift.work(date)
		window := shift.window(date)
		expected := hours(work, work)
		leave := hours(work, leaves)
		both := append(append([]span{}, leaves...), outs...)
		baseCovered := hours(work, both)
		outside := baseCovered - leave
		both = append(both, trips...)
		covered := hours(work, both)
		d.TripHours += covered - baseCovered
		d.ExpectedHours += expected
		d.LeaveHours += leave
		d.OutHours += outside
		remaining := math.Max(0, expected-covered)
		needed += remaining
		if at.Before(window.end) {
			pending = true
			continue
		}
		d.State = "calculated"
		if remaining < 0.001 {
			continue
		}
		effectiveStart := window.start
		effectiveEnd := window.end
		for effectiveStart.Before(effectiveEnd) && (covers(both, effectiveStart) || !covers(work, effectiveStart)) {
			effectiveStart = effectiveStart.Add(time.Minute)
		}
		for effectiveEnd.After(effectiveStart) && (covers(both, effectiveEnd.Add(-time.Minute)) || !covers(work, effectiveEnd.Add(-time.Minute))) {
			effectiveEnd = effectiveEnd.Add(-time.Minute)
		}
		var first, last time.Time
		for _, clock := range clocks {
			if clock.Status == "missing" {
				continue
			}
			t, e := core.ParseTime(clock.ClockTime)
			if e != nil {
				continue
			}
			if t.Before(window.start.Add(-4*time.Hour)) || t.After(window.end.Add(4*time.Hour)) {
				continue
			}
			if !belongs(t, clock.ClockType, window, date, shifts, holiday) {
				continue
			}
			if clock.ClockType == "in" && (first.IsZero() || t.Before(first)) {
				first = t
			}
			if clock.ClockType == "out" && (last.IsZero() || t.After(last)) {
				last = t
			}
		}
		if !first.IsZero() || !last.IsZero() {
			anyClock = true
		}
		if first.IsZero() {
			d.MissingCount++
		} else if first.After(effectiveStart.Add(time.Duration(rule.LateGraceMinutes) * time.Minute)) {
			d.LateCount++
		}
		if last.IsZero() {
			d.MissingCount++
		} else if last.Before(effectiveEnd.Add(-time.Duration(rule.EarlyGraceMinutes) * time.Minute)) {
			d.EarlyCount++
		}
		if !first.IsZero() && !last.IsZero() && last.After(first) {
			d.Worked = true
		}
	}
	if pending {
		d.State = "pending"
	}
	if d.OutHours > 0 || d.TripHours > 0 {
		d.Worked = true
	}
	d.Absent = d.State == "calculated" && needed > 0 && !anyClock
	d.ExpectedHours = round(d.ExpectedHours)
	d.LeaveHours = round(d.LeaveHours)
	d.OutHours = round(d.OutHours)
	d.TripHours = round(d.TripHours)
	return d
}
func Month(ctx context.Context, month, employeeID string) (MonthResult, error) {
	result := MonthResult{Month: month, Items: []MonthEmployee{}}
	start, err := time.ParseInLocation("2006-01", month, core.Location)
	if err != nil {
		return result, fmt.Errorf("month 应为 YYYY-MM")
	}
	end := start.AddDate(0, 1, 0)
	model := queryScope(ctx).apply(core.Model("biz_hrm_employee").Ctx(ctx), "id")
	if employeeID == "" {
		model, err = currentEmployees(ctx, model)
		if err != nil {
			return result, err
		}
	}
	if employeeID != "" {
		model = model.Where("id", employeeID)
	}
	employees, err := model.All()
	if err != nil {
		return result, err
	}
	groups, err := core.Model("biz_hrm_attendance_group").Ctx(ctx).Where("status", 1).All()
	if err != nil {
		return result, err
	}
	holidays, err := core.Model("biz_hrm_attendance_holiday").Ctx(ctx).WhereGTE("holiday_date", start.Format("2006-01-02")).WhereLT("holiday_date", end.Format("2006-01-02")).All()
	if err != nil {
		return result, err
	}
	holidayMap := map[string]bool{}
	for _, h := range holidays {
		holidayMap[h["holiday_date"].String()] = h["is_workday"].Int() == 1
	}
	for _, employee := range employees {
		item := MonthEmployee{EmployeeID: employee["id"].String(), Name: employee["name"].String(), Days: []Day{}}
		direct, fallback := []gdb.Record{}, []gdb.Record{}
		for _, group := range groups {
			var ids []string
			var deps []int64
			if err := json.Unmarshal([]byte(group["employee_ids"].String()), &ids); err != nil {
				return result, err
			}
			if err := json.Unmarshal([]byte(group["dept_ids"].String()), &deps); err != nil {
				return result, err
			}
			for _, id := range ids {
				if id == item.EmployeeID {
					direct = append(direct, group)
					break
				}
			}
			for _, id := range deps {
				if id == employee["dept_id"].Int64() && id > 0 {
					fallback = append(fallback, group)
					break
				}
			}
		}
		chosen := direct
		if len(chosen) == 0 {
			chosen = fallback
		}
		if len(chosen) != 1 {
			item.Warning = "员工未配置考勤组或存在多个匹配组"
			result.Items = append(result.Items, item)
			continue
		}
		var shifts []Shift
		var rule Rule
		if err := json.Unmarshal([]byte(chosen[0]["shifts"].String()), &shifts); err != nil {
			return result, err
		}
		if err := json.Unmarshal([]byte(chosen[0]["rule_json"].String()), &rule); err != nil {
			return result, err
		}
		rows, err := core.Model("biz_hrm_attendance_clock").Ctx(ctx).Where("employee_id", item.EmployeeID).WhereGTE("clock_time", start.Add(-4*time.Hour).Format("2006-01-02 15:04:05")).WhereLT("clock_time", end.Add(28*time.Hour).Format("2006-01-02 15:04:05")).All()
		if err != nil {
			return result, err
		}
		clocks := []Clock{}
		for _, r := range rows {
			clocks = append(clocks, Clock{ClockTime: r["clock_time"].String(), ClockType: r["clock_type"].String(), Status: r["status"].String()})
		}
		absences := map[string][]span{}
		for _, kind := range []string{"leave", "out", "trip"} {
			rows, err := core.Model("biz_hrm_attendance_"+kind).Ctx(ctx).Where("employee_id", item.EmployeeID).Where("approval_status", "approved").WhereLT("start_time", end.AddDate(0, 0, 1).Format("2006-01-02 15:04:05")).WhereGT("end_time", start.Format("2006-01-02 15:04:05")).All()
			if err != nil {
				return result, err
			}
			for _, row := range rows {
				a, e := core.ParseTime(row["start_time"].String())
				if e != nil {
					return result, e
				}
				b, e := core.ParseTime(row["end_time"].String())
				if e != nil {
					return result, e
				}
				absences[kind] = append(absences[kind], span{a, b})
			}
		}
		for day := start; day.Before(end); day = day.AddDate(0, 0, 1) {
			date := day.Format("2006-01-02")
			entry, quit := employee["entry_date"].String(), employee["quit_date"].String()
			if entry != "" && date < entry || quit != "" && date > quit {
				continue
			}
			var h *bool
			if value, ok := holidayMap[date]; ok {
				v := value
				h = &v
			}
			d := calculateDayWithTrips(day, shifts, rule, clocks, absences["leave"], absences["out"], absences["trip"], h, time.Now().In(core.Location))
			if d.State == "ambiguous_schedule" {
				item.Warning = "调休工作日存在互斥班次重叠，该日未计入统计；请明确该日班次后重新计算"
			}
			item.Days = append(item.Days, d)
			if d.ExpectedHours > 0 {
				item.ExpectedDays++
			}
			if d.Worked {
				item.WorkedDays++
			}
			if d.Absent {
				item.AbsentDays++
			}
			item.LateCount += d.LateCount
			item.EarlyCount += d.EarlyCount
			item.MissingCount += d.MissingCount
			item.LeaveHours += d.LeaveHours
			item.OutHours += d.OutHours
			item.TripHours += d.TripHours
		}
		item.LeaveHours = round(item.LeaveHours)
		item.OutHours = round(item.OutHours)
		item.TripHours = round(item.TripHours)
		result.Items = append(result.Items, item)
	}
	return result, nil
}
func manualEditAllowed(r *ghttp.Request, id string) bool {
	if id == "" {
		return true
	}
	table := "biz_hrm_attendance_clock"
	if strings.Contains(r.URL.Path, "leaves") {
		table = "biz_hrm_attendance_leave"
	}
	if strings.Contains(r.URL.Path, "outs") {
		table = "biz_hrm_attendance_out"
	}
	if strings.Contains(r.URL.Path, "trips") {
		table = "biz_hrm_attendance_trip"
	}
	row, err := core.Model(table).Ctx(r.Context()).Where("id", id).One()
	if err != nil {
		r.Response.WriteJson(gf.Failed().SetMsg("记录读取失败"))
		return false
	}
	if len(row) > 0 && row["source"].String() != "manual" {
		r.Response.WriteJson(gf.Failed().SetMsg("同步记录须通过源系统修正"))
		return false
	}
	return true
}
