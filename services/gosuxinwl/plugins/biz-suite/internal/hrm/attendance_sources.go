package hrm

import (
	"context"
	"fmt"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"sort"
	"time"
)

func attCandidates(r *ghttp.Request) {
	var in struct {
		EmployeeID string `json:"employee_id"`
		Date       string `json:"date"`
	}
	if e := r.Parse(&in); e != nil {
		attError(r, e)
		return
	}
	date, e := attDate(in.Date)
	if e != nil {
		attError(r, e)
		return
	}
	people, e := attEmployees(r.Context())
	if e != nil {
		attError(r, e)
		return
	}
	person, ok := people[in.EmployeeID]
	if !ok {
		r.Response.WriteStatus(403)
		return
	}
	rows, e := core.Model("biz_att_rule").Ctx(r.Context()).Where("status", "published").All()
	if e != nil {
		attError(r, e)
		return
	}
	rules := []AttRule{}
	for _, row := range rows {
		rules = append(rules, attDecodeRule(row))
	}
	parents := map[int64]int64{}
	depts, e := core.HostModel("auth_dept").Ctx(r.Context()).Fields("id,pid").All()
	if e != nil {
		attError(r, e)
		return
	}
	for _, d := range depts {
		parents[d["id"].Int64()] = d["pid"].Int64()
	}
	rule, e := resolveAttRule(rules, person, in.Date, parents)
	if e != nil {
		attError(r, e)
		return
	}
	punches, e := attLoadPunches(r.Context(), person.ID, date.AddDate(0, 0, -1).Format("2006-01-02 15:04:05"), date.AddDate(0, 0, 2).Format("2006-01-02 15:04:05"))
	if e != nil {
		attError(r, e)
		return
	}
	absences, e := attLoadAbsences(r.Context(), person.ID, in.Date, date.AddDate(0, 0, 2).Format("2006-01-02 15:04:05"))
	if e != nil {
		attError(r, e)
		return
	}
	items := []g.Map{}
	for _, shift := range rule.Config.Shifts {
		result := eng.Calculate(eng.Input{EmployeeID: person.ID, Date: in.Date, Category: eng.Workday, Rule: rule.Config.Rule, Shift: shift, Punches: punches, Absences: absences, AsOf: time.Now().Unix()})
		score := len(result.Issues)*10000 + result.MissingCount*1000 + int((result.LateSeconds+result.EarlySeconds)/60)
		items = append(items, g.Map{"rule_id": rule.ID, "rule_name": rule.Name, "shift_id": shift.ID, "shift_name": shift.Name, "score": score, "preview": result})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i]["score"].(int) < items[j]["score"].(int) })
	attOK(r, g.Map{"items": items, "requires_confirmation": true})
}

type sourceTime struct {
	ID          int  `json:"time_id"`
	Start       int  `json:"work_sec"`
	End         int  `json:"off_work_sec"`
	EarliestIn  int  `json:"earliest_work_sec"`
	LatestIn    int  `json:"latest_work_sec"`
	EarliestOut int  `json:"earliest_off_work_sec"`
	LatestOut   int  `json:"latest_off_work_sec"`
	NoIn        bool `json:"no_need_checkon"`
	NoOut       bool `json:"no_need_checkoff"`
	AllowRest   bool `json:"allow_rest"`
	RestStart   int  `json:"rest_begin_time"`
	RestEnd     int  `json:"rest_end_time"`
}
type sourceSchedule struct {
	ID    int          `json:"schedule_id"`
	Name  string       `json:"schedule_name"`
	Times []sourceTime `json:"time_section"`
}
type sourceGroup struct {
	ID    int    `json:"groupid"`
	Name  string `json:"groupname"`
	Type  int    `json:"grouptype"`
	Dates []struct {
		Times    []sourceTime `json:"checkintime"`
		Workdays []int        `json:"workdays"`
	} `json:"checkindate"`
	Schedules []sourceSchedule `json:"schedulelist"`
}

func attImportWeCom(r *ghttp.Request) {
	var in struct {
		SnapshotID string `json:"snapshot_id"`
	}
	if e := r.Parse(&in); e != nil {
		attError(r, e)
		return
	}
	if !core.Has(r.Context(), "biz-suite.wecom.manage") {
		r.Response.WriteStatus(403)
		return
	}
	ids, e := CreateWeComRuleDrafts(r.Context(), in.SnapshotID)
	if e != nil {
		attError(r, e)
		return
	}
	attOK(r, g.Map{"ids": ids, "count": len(ids)})
}
func CreateWeComRuleDrafts(ctx context.Context, snapshotID string) ([]string, error) {
	return nil, fmt.Errorf("企业微信考勤规则导入已停用，请使用本地考勤设置")
}

func convertSourceShift(id, name string, times []sourceTime) eng.Shift {
	shift := eng.Shift{ID: id, Name: name, Version: "1"}
	for i, t := range times {
		end := t.End
		if end <= t.Start {
			end += 86400
		}
		mode := "both"
		if t.NoIn && t.NoOut {
			mode = "none"
		} else if t.NoIn {
			mode = "out"
		} else if t.NoOut {
			mode = "in"
		}
		window := eng.PunchWindow{InBeforeMinutes: max(0, (t.Start-t.EarliestIn)/60), InAfterMinutes: max(0, (t.LatestIn-t.Start)/60), OutBeforeMinutes: max(0, (end-t.EarliestOut)/60), OutAfterMinutes: max(0, (t.LatestOut-end)/60)}
		shift.Segments = append(shift.Segments, eng.Segment{ID: fmt.Sprint(i + 1), Name: fmt.Sprintf("时段%d", i+1), Kind: "work", StartMinute: t.Start / 60, EndMinute: end / 60, PunchMode: mode, Window: &window})
		if t.AllowRest && t.RestEnd > t.RestStart {
			shift.Segments = append(shift.Segments, eng.Segment{ID: guid.S(), Name: "休息", Kind: "break", StartMinute: t.RestStart / 60, EndMinute: t.RestEnd / 60, PunchMode: "none"})
		}
	}
	return shift
}

func attManualPeriodAllowed(ctx context.Context, table, id, start, end string) error {
	a, e := core.ParseTime(start)
	if e != nil {
		return e
	}
	b, e := core.ParseTime(end)
	if e != nil {
		return e
	}
	periods := [][2]string{{a.Format("2006-01-02"), b.Format("2006-01-02")}}
	if id != "" {
		old, e := core.Model(table).Ctx(ctx).Where("id", id).One()
		if e != nil {
			return e
		}
		if len(old) > 0 {
			if old["source"].String() != "manual" {
				return fmt.Errorf("不能通过人工入口修改同步原文")
			}
			s, t := old["start_time"], old["end_time"]
			if table == "biz_hrm_attendance_clock" {
				s, t = old["clock_time"], old["clock_time"]
			}
			if s != nil && t != nil {
				from, e := core.ParseTime(s.String())
				if e != nil {
					return e
				}
				to, e := core.ParseTime(t.String())
				if e != nil {
					return e
				}
				periods = append(periods, [2]string{from.Format("2006-01-02"), to.Format("2006-01-02")})
			}
		}
	}
	for _, p := range periods {
		// A punch after midnight can belong to the preceding work date;
		// overtime may also grant a next-day late-arrival entitlement.
		first, _ := attDate(p[0])
		last, _ := attDate(p[1])
		days, e := attRange(first.AddDate(0, 0, -1).Format("2006-01-02"), last.AddDate(0, 0, 1).Format("2006-01-02"))
		if e != nil {
			return e
		}
		for _, d := range days {
			if attLocked(ctx, d) {
				return fmt.Errorf("该证据可能影响跨夜或次日晚到；%s已锁定，需先重开并记录原因", d[:7])
			}
		}
	}
	return nil
}

func attSaveEvidence(ctx context.Context, table, id string, data g.Map, source string, start, end time.Time) error {
	if source != "manual" {
		return saveByID(ctx, table, id, data)
	}
	attMu.Lock()
	defer attMu.Unlock()
	if e := attManualPeriodAllowed(ctx, table, id, start.Format(time.RFC3339), end.Format(time.RFC3339)); e != nil {
		return e
	}
	return core.DB().Transaction(ctx, func(c context.Context, tx gdb.TX) error {
		old, e := core.Model(table).Ctx(c).Where("id", id).One()
		if e != nil {
			return e
		}
		if e = saveByID(c, table, id, data); e != nil {
			return e
		}
		if e = AttendanceChanged(c, start, end, []string{fmt.Sprint(data["employee_id"])}); e != nil {
			return e
		}
		if len(old) > 0 {
			s, t := old["start_time"], old["end_time"]
			if table == "biz_hrm_attendance_clock" {
				s, t = old["clock_time"], old["clock_time"]
			}
			if s != nil && t != nil {
				a, err := core.ParseTime(s.String())
				if err != nil {
					return err
				}
				b, err := core.ParseTime(t.String())
				if err != nil {
					return err
				}
				if e = AttendanceChanged(c, a, b, []string{old["employee_id"].String()}); e != nil {
					return e
				}
			}
		}
		return attAudit(c, "manual_evidence", id, "save", fmt.Sprint(data["reason"]), old, data)
	})
}
