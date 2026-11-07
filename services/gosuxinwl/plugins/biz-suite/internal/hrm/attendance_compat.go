package hrm

import (
	"context"
	"encoding/json"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"sort"
)

func attProjectClockStates(ctx context.Context, batchID, basis string) error {
	rows, e := core.Model("biz_att_result").Ctx(ctx).Where("batch_id", batchID).All()
	if e != nil {
		return e
	}
	return core.DB().Transaction(ctx, func(c context.Context, tx gdb.TX) error {
		for _, row := range rows {
			date := row["work_date"].String()
			month, e := core.Model("biz_att_month").Ctx(c).Where("month", date[:7]).One()
			if e != nil {
				return e
			}
			if basis == "trial" && (month["status"].String() == "published" || month["status"].String() == "locked") {
				continue
			}
			// A recalculation can select different punches. Remove projections of
			// the previous result before writing the current selection.
			oldResults := core.Model("biz_att_result").Ctx(c).Fields("id").Where("employee_id", row["employee_id"]).Where("work_date", date)
			if _, e = core.Model("biz_att_clock_state").Ctx(c).Where("result_id IN (?)", oldResults).Delete(); e != nil {
				return e
			}
			var result eng.Result
			if e = json.Unmarshal([]byte(row["result_json"].String()), &result); e != nil {
				return e
			}
			for _, s := range result.Segments {
				for _, p := range []struct {
					id          string
					late, early bool
				}{{s.InPunchID, s.LateSeconds > 0, false}, {s.OutPunchID, false, s.EarlySeconds > 0}} {
					if p.id == "" {
						continue
					}
					exists, e := core.Model("biz_hrm_attendance_clock").Ctx(c).Where("id", p.id).Exist()
					if e != nil {
						return e
					}
					if !exists {
						continue
					}
					status := "normal"
					if p.late {
						status = "late"
					}
					if p.early {
						status = "early"
					}
					if s.MissingCount > 0 {
						status = "missing"
					}
					if s.State == "pending" || result.State == "pending" {
						status = "in_progress"
					}
					if s.State == "review" || result.State == "review" {
						status = "pending_review"
					}
					if result.State == "configuration_pending" {
						status = "configuration_pending"
					}
					data := g.Map{"clock_id": p.id, "result_id": row["id"], "batch_id": batchID, "work_date": date, "local_status": status, "basis": basis, "updated_at": core.Now()}
					exists, e = core.Model("biz_att_clock_state").Ctx(c).Where("clock_id", p.id).Exist()
					if e != nil {
						return e
					}
					if exists {
						_, e = core.Model("biz_att_clock_state").Ctx(c).Where("clock_id", p.id).Update(data)
					} else {
						_, e = core.Model("biz_att_clock_state").Ctx(c).Data(data).Insert()
					}
					if e != nil {
						return e
					}
				}
			}
		}
		return nil
	})
}

// Unassigned raw cards may inherit only a pending day state. This never treats
// a raw card as normal/late or attributes it to a shift without selected proof.
// Published months keep their frozen batch even if newer trial data exists.
func attClockDayResultSQL(column string) string {
	return `(SELECT ar.` + column + ` FROM biz_att_result ar JOIN biz_att_batch ab ON ab.id=ar.batch_id AND ab.status='completed'
WHERE ar.employee_id=biz_hrm_attendance_clock.employee_id AND ar.work_date=SUBSTR(biz_hrm_attendance_clock.clock_time,1,10)
AND (NOT EXISTS (SELECT 1 FROM biz_att_month am WHERE am.month=SUBSTR(ar.work_date,1,7) AND am.status IN ('published','locked'))
OR ar.batch_id IN (SELECT am.batch_id FROM biz_att_month am WHERE am.month=SUBSTR(ar.work_date,1,7) AND am.status IN ('published','locked')))
ORDER BY COALESCE(CASE WHEN JSON_VALID(ab.request_json) THEN JSON_EXTRACT(ab.request_json,'$.queued_at_unix_nano') END,0) DESC, ab.created_at DESC, ar.created_at DESC, ar.id DESC LIMIT 1)`
}

func attClockLocalStatusSQL() string {
	return `COALESCE((SELECT ac.local_status FROM biz_att_clock_state ac WHERE ac.clock_id=biz_hrm_attendance_clock.id), CASE ` + attClockDayResultSQL("status") + ` WHEN 'configuration_pending' THEN 'configuration_pending' WHEN 'pending' THEN 'in_progress' ELSE 'uncomputed' END)`
}

func attPublishedMonth(ctx context.Context, month, employeeID string) (g.Map, bool, error) {
	publication, e := core.Model("biz_att_month").Ctx(ctx).Where("month", month).WhereIn("status", []string{"published", "locked"}).One()
	if e != nil || len(publication) == 0 {
		return nil, false, e
	}
	q := queryScope(ctx).apply(core.Model("biz_att_result").Ctx(ctx), "employee_id").Where("batch_id", publication["batch_id"])
	if employeeID != "" {
		q = q.Where("employee_id", employeeID)
	}
	rows, e := q.Order("employee_id,work_date").All()
	if e != nil {
		return nil, true, e
	}
	return g.Map{"month": month, "items": attMonthItems(rows), "basis": "published", "result_version": publication["revision"], "batch_id": publication["batch_id"], "status": publication["status"], "change_pending": publication["change_pending"]}, true, nil
}

func attMonthItems(rows gdb.Result) []MonthEmployee {
	items := []MonthEmployee{}
	idx := map[string]int{}
	for _, row := range rows {
		var p AttEmployee
		var v eng.Result
		_ = json.Unmarshal([]byte(row["employee_json"].String()), &p)
		_ = json.Unmarshal([]byte(row["result_json"].String()), &v)
		i, ok := idx[p.ID]
		if !ok {
			i = len(items)
			idx[p.ID] = i
			items = append(items, MonthEmployee{EmployeeID: p.ID, Name: p.Name, EmployeeNo: p.EmployeeNo, Department: p.Department, Days: []Day{}})
		}
		item := &items[i]
		day := Day{Date: v.Date, ExpectedHours: float64(v.ExpectedSeconds) / 3600, LeaveHours: float64(v.LeaveSeconds) / 3600, OutHours: float64(v.OutSeconds) / 3600, TripHours: float64(v.TripSeconds) / 3600, LateCount: v.LateCount, EarlyCount: v.EarlyCount, MissingCount: v.MissingCount, Worked: v.CreditedSeconds > 0, Absent: v.ConfirmedAbsentSeconds > 0, State: "work"}
		if v.State == "rest" {
			day.State = "rest"
		}
		if v.State == "review" || v.State == "pending" || v.State == "configuration_pending" {
			day.State = "pending"
		}
		item.Days = append(item.Days, day)
		if v.ExpectedSeconds > 0 {
			item.ExpectedDays++
		}
		item.ExpectedHours += float64(v.ExpectedSeconds) / 3600
		item.ActualHours += float64(v.ActualSeconds) / 3600
		item.CreditedHours += float64(v.CreditedSeconds) / 3600
		item.OvertimeHours += float64(v.OvertimeSeconds) / 3600
		for _, bucket := range v.OvertimeBuckets {
			switch bucket.Category {
			case eng.Workday:
				item.OvertimeWorkdayHours += float64(bucket.Seconds) / 3600
			case eng.Restday:
				item.OvertimeRestdayHours += float64(bucket.Seconds) / 3600
			case eng.Holiday:
				item.OvertimeHolidayHours += float64(bucket.Seconds) / 3600
			}
		}
		if day.Worked {
			item.WorkedDays++
		}
		if day.Absent {
			item.AbsentDays++
		}
		item.LateCount += v.LateCount
		item.EarlyCount += v.EarlyCount
		item.MissingCount += v.MissingCount
		item.LeaveHours += day.LeaveHours
		item.OutHours += day.OutHours
		item.TripHours += day.TripHours
	}
	return items
}

// The old month URL may display local trial data, but never invokes the former
// group/WeCom-status algorithm when no formal local publication exists.
func attLocalTrialMonth(ctx context.Context, month, employeeID string) (g.Map, error) {
	start, err := attDate(month + "-01")
	if err != nil {
		return nil, err
	}
	q := queryScope(ctx).apply(core.Model("biz_att_result").Ctx(ctx), "employee_id").WhereBetween("work_date", start.Format("2006-01-02"), start.AddDate(0, 1, -1).Format("2006-01-02")).Where("batch_id IN (?)", core.Model("biz_att_batch").Ctx(ctx).Fields("id").Where("status", "completed"))
	if employeeID != "" {
		q = q.Where("employee_id", employeeID)
	}
	rows, err := q.Order("created_at DESC,id DESC").Limit(100000).All()
	if err != nil {
		return nil, err
	}
	sequences, err := attCompletedBatchSequences(ctx)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return sequences[rows[i]["batch_id"].String()] > sequences[rows[j]["batch_id"].String()]
	})
	selected, seen, versions := gdb.Result{}, map[string]bool{}, map[string]bool{}
	for _, row := range rows {
		key := row["employee_id"].String() + ":" + row["work_date"].String()
		if seen[key] {
			continue
		}
		seen[key] = true
		selected = append(selected, row)
		versions[row["rule_id"].String()] = true
	}
	sort.SliceStable(selected, func(i, j int) bool {
		return selected[i]["employee_id"].String()+selected[i]["work_date"].String() < selected[j]["employee_id"].String()+selected[j]["work_date"].String()
	})
	items, status, warning := attMonthItems(selected), "trial", "本地试算，尚未完成月结；未计算日期不可视为正常出勤"
	if len(selected) == 0 {
		status, warning = "uncomputed", "待计算：请配置本地考勤制度并完成计算，不采用企业微信状态或旧版考勤算法"
	}
	people, err := attEmployees(ctx)
	if err != nil {
		return nil, err
	}
	index := map[string]int{}
	for i, item := range items {
		index[item.EmployeeID] = i
	}
	for id, person := range people {
		if employeeID != "" && id != employeeID {
			continue
		}
		i, ok := index[id]
		if !ok {
			i = len(items)
			items = append(items, MonthEmployee{EmployeeID: id, Name: person.Name, EmployeeNo: person.EmployeeNo, Department: person.Department, Days: []Day{}})
		}
		for date := start; date.Before(start.AddDate(0, 1, 0)); date = date.AddDate(0, 0, 1) {
			day := date.Format("2006-01-02")
			if !seen[id+":"+day] {
				items[i].Days = append(items[i].Days, Day{Date: day, State: "uncomputed"})
			}
		}
		sort.Slice(items[i].Days, func(a, b int) bool { return items[i].Days[a].Date < items[i].Days[b].Date })
	}
	sort.Slice(items, func(i, j int) bool { return items[i].EmployeeID < items[j].EmployeeID })
	ruleIDs := []string{}
	for id := range versions {
		if id != "" {
			ruleIDs = append(ruleIDs, id)
		}
	}
	sort.Strings(ruleIDs)
	return g.Map{"month": month, "items": items, "basis": "local_trial", "status": status, "result_version": 0, "rule_ids": ruleIDs, "warning": warning}, nil
}
