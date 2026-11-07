package wecom

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

// SchemaV5 is supplied to the central migration as additive DDL. Nothing in
// this package runs a schema migration, and capturing a snapshot never creates
// or publishes a local attendance rule.
func SchemaV5() []string {
	return []string{`CREATE TABLE IF NOT EXISTS wecom_checkin_rule_snapshot (id VARCHAR(64) PRIMARY KEY, snapshot_type VARCHAR(32) NOT NULL, scope_key VARCHAR(192) NOT NULL, rule_date VARCHAR(10) NOT NULL DEFAULT '', start_time DATETIME NULL, end_time DATETIME NULL, raw_json LONGTEXT NOT NULL, fetched_at DATETIME NOT NULL)`}
}

func checkinUserIDs(ids []string) ([]string, error) {
	if len(ids) == 0 || len(ids) > 100 {
		return nil, errors.New("打卡规则查询需选择1至100名员工")
	}
	set := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || len(id) > 128 {
			return nil, errors.New("打卡规则查询包含无效员工标识")
		}
		if !set[id] {
			out = append(out, id)
			set[id] = true
		}
	}
	sort.Strings(out)
	return out, nil
}

var ErrLocalAttendanceRules = errors.New("企业微信考勤规则已停用，请在智慧HRM考勤设置中配置本地制度")

func (c *Client) readCheckinRules(ctx context.Context, path string, body any, listKey string) (json.RawMessage, error) {
	return nil, ErrLocalAttendanceRules
}

// Retained method signatures reject old callers without making upstream requests.
func (c *Client) GetCorpCheckinOption(ctx context.Context) (json.RawMessage, error) {
	return c.readCheckinRules(ctx, "/checkin/getcorpcheckinoption", g.Map{}, "group")
}
func (c *Client) GetEmployeeCheckinOption(ctx context.Context, date time.Time, userIDs []string) (json.RawMessage, error) {
	ids, err := checkinUserIDs(userIDs)
	if err != nil {
		return nil, err
	}
	if date.IsZero() {
		return nil, errors.New("请选择员工打卡规则日期")
	}
	day := date.In(core.Location)
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, core.Location)
	return c.readCheckinRules(ctx, "/checkin/getcheckinoption", g.Map{"datetime": day.Unix(), "useridlist": ids}, "info")
}
func (c *Client) GetCheckinScheduleList(ctx context.Context, start, end time.Time, userIDs []string) (json.RawMessage, error) {
	ids, err := checkinUserIDs(userIDs)
	if err != nil {
		return nil, err
	}
	if start.IsZero() || end.IsZero() || !end.After(start) || end.After(start.AddDate(0, 1, 0)) {
		return nil, errors.New("排班查询起止时间应有效且单次跨度不超过一个月")
	}
	return c.readCheckinRules(ctx, "/checkin/getcheckinschedulist", g.Map{"starttime": start.Unix(), "endtime": end.Unix(), "useridlist": ids}, "schedule_list")
}

type CheckinSnapshotRequest struct {
	Scope       string   `json:"scope"`
	Date        string   `json:"date"`
	StartTime   string   `json:"start_time"`
	EndTime     string   `json:"end_time"`
	EmployeeIDs []string `json:"employee_ids"`
}
type CheckinSnapshot struct {
	ID           string `json:"id"`
	SnapshotType string `json:"snapshot_type"`
	ScopeKey     string `json:"scope_key"`
	RuleDate     string `json:"rule_date"`
	FetchedAt    string `json:"fetched_at"`
}

// Deprecated: historic snapshots stay available for audit, but new capture is disabled.
func (c *Client) CaptureCheckinSnapshot(ctx context.Context, in CheckinSnapshotRequest) (CheckinSnapshot, error) {
	return CheckinSnapshot{}, ErrLocalAttendanceRules
}

func snapshotAuth(r *ghttp.Request) bool {
	if !auth(r) {
		return false
	}
	if !core.Has(r.Context(), "biz-suite.wecom.manage") {
		r.Response.WriteStatus(http.StatusForbidden)
		return false
	}
	r.Response.Header().Set("Cache-Control", "no-store")
	return true
}
func bindCheckinSnapshots(api *ghttp.RouterGroup, c *Client) {
	api.GET("/checkin-rules/snapshots", func(r *ghttp.Request) {
		if !snapshotAuth(r) {
			return
		}
		query := core.Model("wecom_checkin_rule_snapshot").Ctx(r.Context()).Fields("id,snapshot_type,scope_key,rule_date,start_time,end_time,fetched_at").OrderDesc("fetched_at").Limit(100)
		if scope := r.Get("scope").String(); scope != "" {
			query = query.Where("snapshot_type", scope)
		}
		rows, err := query.All()
		if err != nil {
			fail(r, dbError("snapshot list"))
			return
		}
		r.Response.WriteJson(gf.Success().SetData(rows))
	})
	api.GET("/checkin-rules/snapshots/:id", func(r *ghttp.Request) {
		if !snapshotAuth(r) {
			return
		}
		row, err := core.Model("wecom_checkin_rule_snapshot").Ctx(r.Context()).Where("id", r.Get("id").String()).One()
		if err != nil {
			fail(r, dbError("snapshot detail"))
			return
		}
		if len(row) == 0 {
			fail(r, errors.New("规则快照不存在"))
			return
		}
		r.Response.WriteJson(gf.Success().SetData(row))
	})
	api.POST("/checkin-rules/snapshots", func(r *ghttp.Request) {
		if !snapshotAuth(r) {
			return
		}
		var in CheckinSnapshotRequest
		if r.Parse(&in) != nil {
			fail(r, errors.New("规则快照请求格式无效"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		result, err := c.CaptureCheckinSnapshot(ctx, in)
		if err != nil {
			fail(r, err)
			return
		}
		r.Response.WriteJson(gf.Success().SetData(result))
	})
}
