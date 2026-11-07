package wecom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/hrm"
)

type Checkin struct {
	UserID         string `json:"userid"`
	Type           string `json:"checkin_type"`
	Exception      string `json:"exception_type"`
	Time           int64  `json:"checkin_time"`
	ScheduledTime  int64  `json:"sch_checkin_time"`
	ScheduleID     int64  `json:"schedule_id"`
	GroupID        int64  `json:"groupid"`
	LocationTitle  string `json:"location_title"`
	LocationDetail string `json:"location_detail"`
	Latitude       int64  `json:"lat"`
	Longitude      int64  `json:"lng"`
}

func (c *Client) SyncCheckin(ctx context.Context, start, end time.Time) (count int, result error) {
	ctx, changes := collectAttendanceChanges(ctx)
	defer changes.notify(ctx, &result)
	if end.IsZero() {
		end = time.Now()
	}
	if start.IsZero() {
		start = end.Add(-24 * time.Hour)
	}
	var floorErr error
	start, floorErr = applySyncFloor("checkin", start, end)
	if floorErr != nil {
		return 0, floorErr
	}
	periods, err := windows(start, end, 30*24*time.Hour)
	if err != nil {
		return 0, err
	}
	users, err := core.Model("wecom_user").Ctx(ctx).Fields("wecom_userid,department_ids").OrderAsc("wecom_userid").All()
	if err != nil {
		return 0, dbError("checkin users")
	}
	if len(users) == 0 {
		return 0, errors.New("synchronize enterprise WeChat contacts before checkins")
	}
	filter, err := c.filter(ctx)
	if err != nil {
		return 0, err
	}
	eligible := users[:0]
	for _, row := range users {
		var deps []int64
		_ = json.Unmarshal([]byte(row["department_ids"].String()), &deps)
		if filter.allows(row["wecom_userid"].String(), deps) {
			eligible = append(eligible, row)
		}
	}
	users = eligible
	failed := 0
	for _, period := range periods {
		for i := 0; i < len(users); i += 100 {
			stop := i + 100
			if stop > len(users) {
				stop = len(users)
			}
			ids := []string{}
			allowed := map[string]bool{}
			for _, row := range users[i:stop] {
				uid := row["wecom_userid"].String()
				if uid != "" {
					ids = append(ids, uid)
					allowed[uid] = true
				}
			}
			if len(ids) == 0 {
				continue
			}
			var out struct {
				apiError
				CheckinData []jsonRecord `json:"checkindata"`
			}
			body := g.Map{"opencheckindatatype": 3, "useridlist": ids, "starttime": period[0].Unix(), "endtime": period[1].Unix()}
			if err = c.api(ctx, "checkin", http.MethodPost, "/checkin/getcheckindata", nil, body, &out); err != nil {
				return count, err
			}
			for _, raw := range out.CheckinData {
				var item Checkin
				if err = raw.decode(&item); err != nil {
					return count, errors.New("invalid checkin response record")
				}
				if !allowed[item.UserID] {
					return count, errors.New("wecom returned checkin for a user outside the requested batch")
				}
				var partialError *PartialError
				if err = saveCheckin(ctx, item, string(raw)); errors.As(err, &partialError) {
					failed += partialError.Failed
					continue
				} else if err != nil {
					return count, err
				}
				count++
			}
		}
	}
	return count, partial(failed)
}

func saveCheckin(ctx context.Context, item Checkin, raw string) (result error) {
	ctx, changes := collectAttendanceChanges(ctx)
	err := core.DB().Transaction(ctx, func(txctx context.Context, _ gdb.TX) error {
		ref := checkinRef(item)
		stamp := item.Time
		if stamp <= 0 {
			stamp = item.ScheduledTime
		}
		previous, e := core.Model("wecom_checkin_raw").Ctx(txctx).Where("external_ref", ref).Fields("raw_json,processing_error").One()
		if e != nil {
			return dbError("checkin change lookup")
		}
		previousClock, e := checkinBusinessRecord(txctx, ref)
		if e != nil {
			return e
		}
		data := g.Map{"id": stableID(ref), "wecom_userid": item.UserID, "checkin_time": unixTime(stamp), "checkin_type": item.Type, "exception_type": item.Exception, "location": item.LocationDetail, "raw_json": raw, "external_ref": ref, "processing_error": "", "created_at": now()}
		if e = upsert(txctx, "wecom_checkin_raw", "external_ref", ref, data); e != nil {
			return e
		}
		clock, normalErr := normalizeCheckin(txctx, item, ref, raw)
		if normalErr == nil {
			if _, normalErr = hrm.UpsertClock(txctx, clock); normalErr != nil {
				normalErr = errors.New("normalized checkin failed HRM validation or persistence")
			}
		}
		if normalErr != nil {
			result = &PartialError{1}
			if _, e = core.Model("wecom_checkin_raw").Ctx(txctx).Where("external_ref", ref).Update(g.Map{"processing_error": normalErr.Error()}); e != nil {
				return dbError("checkin exception")
			}
		}
		if e = recordCheckinChanges(txctx, item, ref, raw, previous, previousClock, normalErr); e != nil {
			return e
		}
		return enqueueAttendanceChanges(txctx, changes)
	})
	if err != nil {
		return err
	}
	return result
}

func checkinBusinessRecord(ctx context.Context, ref string) (gdb.Record, error) {
	row, err := core.Model("biz_hrm_attendance_clock").Ctx(ctx).Where("source", "wecom").Where("external_ref", ref).Fields("employee_id,wecom_userid,clock_time,clock_type,status,source,external_ref,latitude,longitude,address,raw_json").One()
	if err != nil {
		return nil, dbError("checkin normalized change lookup")
	}
	return row, nil
}
func recordCheckinChanges(ctx context.Context, item Checkin, ref, raw string, previous, previousClock gdb.Record, normalErr error) error {
	current, err := checkinBusinessRecord(ctx, ref)
	if err != nil {
		return err
	}
	message := ""
	if normalErr != nil {
		message = normalErr.Error()
	}
	if len(previous) > 0 && previous["raw_json"].String() == raw && previous["processing_error"].String() == message && mustJSON(previousClock) == mustJSON(current) {
		return nil
	}
	// Raw evidence changes matter even if it has no recognizable punch type.
	employee, e := employeeIDByWeCom(ctx, item.UserID)
	if e == nil {
		var old Checkin
		_ = json.Unmarshal([]byte(previous["raw_json"].String()), &old)
		for _, stamp := range []int64{old.Time, old.ScheduledTime, item.Time, item.ScheduledTime} {
			if stamp > 0 {
				recordAttendanceChange(ctx, employee, time.Unix(stamp, 0), time.Unix(stamp, 0))
			}
		}
	}
	for _, row := range []gdb.Record{previousClock, current} {
		if len(row) > 0 {
			if stamp, e := core.ParseTime(row["clock_time"].String()); e == nil {
				recordAttendanceChange(ctx, row["employee_id"].String(), stamp, stamp)
			}
		}
	}
	return nil
}

func checkinRef(v Checkin) string {
	// Fixed-shift corrections replace the corresponding missing/incorrect punch.
	// Free/outside punches lack a scheduled identity and use their actual instant.
	stamp := v.Time
	if v.ScheduledTime > 0 && (v.Type == "上班打卡" || v.Type == "下班打卡") {
		stamp = v.ScheduledTime
	}
	return "wecom:" + stableID(fmt.Sprintf("%s|%s|%d|%d|%d", v.UserID, v.Type, stamp, v.GroupID, v.ScheduleID))
}
func normalizeCheckin(ctx context.Context, v Checkin, ref, raw string) (hrm.Clock, error) {
	clock := hrm.Clock{WeComUserID: v.UserID, Source: "wecom", ExternalRef: ref, RawJSON: raw, Address: strings.TrimSpace(v.LocationTitle + " " + v.LocationDetail)}
	switch v.Type {
	case "上班打卡":
		clock.ClockType = "in"
	case "下班打卡":
		clock.ClockType = "out"
	case "外出打卡", "仅记录打卡时间和位置":
		clock.ClockType = "outside"
	default:
		return clock, errors.New("unsupported checkin_type; raw record retained")
	}
	employee, err := employeeIDByWeCom(ctx, v.UserID)
	if err != nil {
		return clock, err
	}
	clock.EmployeeID = employee
	stamp := v.Time
	clock.Status = "normal"
	if strings.Contains(v.Exception, "未打卡") {
		clock.Status = "missing"
		stamp = v.ScheduledTime
	} else if v.Exception != "" && v.Exception != "正常" {
		clock.Status = "abnormal"
		if v.Exception == "时间异常" && v.ScheduledTime > 0 {
			if clock.ClockType == "in" && v.Time > v.ScheduledTime {
				clock.Status = "late"
			}
			if clock.ClockType == "out" && v.Time < v.ScheduledTime {
				clock.Status = "early"
			}
		}
	}
	if stamp <= 0 {
		return clock, errors.New("checkin has no valid actual or scheduled timestamp")
	}
	clock.ClockTime = unixTime(stamp)
	if v.Latitude < -90000000 || v.Latitude > 90000000 || v.Longitude < -180000000 || v.Longitude > 180000000 {
		return clock, errors.New("checkin coordinates are out of range")
	}
	clock.Latitude = strconv.FormatFloat(float64(v.Latitude)/1e6, 'f', 6, 64)
	clock.Longitude = strconv.FormatFloat(float64(v.Longitude)/1e6, 'f', 6, 64)
	return clock, nil
}
func unixTime(stamp int64) string {
	return time.Unix(stamp, 0).In(core.Location).Format("2006-01-02 15:04:05")
}
