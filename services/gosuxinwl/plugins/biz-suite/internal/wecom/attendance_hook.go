package wecom

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

var attendanceHook struct {
	sync.RWMutex
	fn func(context.Context, time.Time, time.Time, []string) error
}

// SetAttendanceChangeHook registers the attendance engine without a package
// dependency on it. A nil hook disables notifications. The callback should
// persist dirty work; it must not publish or overwrite a closed month.
func SetAttendanceChangeHook(fn func(context.Context, time.Time, time.Time, []string) error) {
	attendanceHook.Lock()
	attendanceHook.fn = fn
	attendanceHook.Unlock()
}

type changeKey struct{}
type attendanceChanges struct {
	start, end time.Time
	employees  map[string]bool
}

type approvalImpact struct {
	employee   string
	start, end time.Time
}

func collectAttendanceChanges(ctx context.Context) (context.Context, *attendanceChanges) {
	c := &attendanceChanges{employees: map[string]bool{}}
	return context.WithValue(ctx, changeKey{}, c), c
}
func recordAttendanceChange(ctx context.Context, employee string, start, end time.Time) {
	c, ok := ctx.Value(changeKey{}).(*attendanceChanges)
	if !ok || employee == "" || start.IsZero() || end.IsZero() {
		return
	}
	if !end.After(start) {
		end = start.Add(time.Second)
	}
	if c.start.IsZero() || start.Before(c.start) {
		c.start = start
	}
	if c.end.IsZero() || end.After(c.end) {
		c.end = end
	}
	c.employees[employee] = true
}
func (c *attendanceChanges) notify(ctx context.Context, result *error) {
	// A canceled HTTP/job context must not lose changes already committed.
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	if err := RetryAttendanceChanges(finish); err != nil {
		*result = errors.Join(*result, errors.New("同步数据已保存，但考勤重算通知失败，请重试同步"))
	}
}

func enqueueAttendanceChanges(ctx context.Context, c *attendanceChanges) error {
	if len(c.employees) == 0 {
		return nil
	}
	ids := make([]string, 0, len(c.employees))
	for id := range c.employees {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	_, err := core.Model("wecom_attendance_outbox").Ctx(ctx).Data(g.Map{"id": guid.S(), "start_time": c.start.In(core.Location).Format("2006-01-02 15:04:05"), "end_time": c.end.In(core.Location).Format("2006-01-02 15:04:05"), "employee_ids": mustJSON(ids), "status": "pending", "error_message": "", "created_at": now()}).Insert()
	if err != nil {
		return dbError("attendance outbox")
	}
	return nil
}

var approvalKinds = []string{"leave", "out", "trip", "overtime"}

func validApprovalKind(kind string) bool {
	for _, candidate := range approvalKinds {
		if candidate == kind {
			return true
		}
	}
	return false
}

// Capture both old and new intervals: a revoked or edited approval can affect
// dates far outside the submission-time window being synchronized.
func approvalChanges(ctx context.Context, sp string) (string, []approvalImpact, error) {
	if _, ok := ctx.Value(changeKey{}).(*attendanceChanges); !ok {
		return "", nil, nil
	}
	var signature strings.Builder
	var impacts []approvalImpact
	for _, kind := range approvalKinds {
		rows, err := core.Model("biz_hrm_attendance_"+kind).Ctx(ctx).Where("source", "wecom").Where("external_ref", sp).Fields("employee_id,start_time,end_time,approval_status,duration,reason,raw_json").All()
		if err != nil {
			return "", nil, dbError("approval changed intervals")
		}
		signature.WriteString(kind + ":" + mustJSON(rows))
		for _, row := range rows {
			start, a := core.ParseTime(row["start_time"].String())
			end, b := core.ParseTime(row["end_time"].String())
			if a == nil && b == nil {
				impacts = append(impacts, approvalImpact{row["employee_id"].String(), start, end})
			}
		}
	}
	return signature.String(), impacts, nil
}
