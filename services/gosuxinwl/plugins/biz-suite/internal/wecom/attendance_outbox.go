package wecom

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

var outboxMu sync.Mutex

// RetryAttendanceChanges delivers durable notifications left by committed
// source changes. Delivery is at least once; a failed callback never consumes
// the outbox record. Monthly grouping bounds dirty jobs and excludes empty
// gaps between unrelated historical approvals.
func RetryAttendanceChanges(ctx context.Context) error {
	outboxMu.Lock()
	defer outboxMu.Unlock()
	attendanceHook.RLock()
	hook := attendanceHook.fn
	attendanceHook.RUnlock()
	if hook == nil {
		return nil
	}
	for {
		rows, err := core.Model("wecom_attendance_outbox").Ctx(ctx).Where("status", "pending").Order("created_at,id").Limit(200).All()
		if err != nil {
			return dbError("attendance outbox read")
		}
		if len(rows) == 0 {
			return nil
		}
		buckets := map[string]*attendanceChanges{}
		keys := []string{}
		rowIDs := []string{}
		for _, row := range rows {
			start, a := core.ParseTime(row["start_time"].String())
			end, b := core.ParseTime(row["end_time"].String())
			var employees []string
			if a != nil || b != nil || !end.After(start) || json.Unmarshal([]byte(row["employee_ids"].String()), &employees) != nil || len(employees) == 0 {
				return errors.New("考勤重算通知记录格式无效，请人工检查")
			}
			rowIDs = append(rowIDs, row["id"].String())
			for from := start; from.Before(end); {
				next := time.Date(from.Year(), from.Month()+1, 1, 0, 0, 0, 0, core.Location)
				if next.After(end) {
					next = end
				}
				key := from.Format("2006-01")
				bucket := buckets[key]
				if bucket == nil {
					bucket = &attendanceChanges{employees: map[string]bool{}}
					buckets[key] = bucket
					keys = append(keys, key)
				}
				if bucket.start.IsZero() || from.Before(bucket.start) {
					bucket.start = from
				}
				if bucket.end.IsZero() || next.After(bucket.end) {
					bucket.end = next
				}
				for _, id := range employees {
					bucket.employees[id] = true
				}
				from = next
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			bucket := buckets[key]
			ids := []string{}
			for id := range bucket.employees {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			if err = hook(ctx, bucket.start, bucket.end, ids); err != nil {
				_, _ = core.Model("wecom_attendance_outbox").Ctx(ctx).WhereIn("id", rowIDs).Update(g.Map{"error_message": "考勤引擎通知失败，将自动重试"})
				return errors.New("考勤引擎通知失败，待重试记录已保留")
			}
		}
		if _, err = core.Model("wecom_attendance_outbox").Ctx(ctx).WhereIn("id", rowIDs).Update(g.Map{"status": "processed", "error_message": ""}); err != nil {
			return dbError("attendance outbox completion")
		}
	}
}
