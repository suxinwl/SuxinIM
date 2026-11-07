package wecom

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

type HistoryProbeResult struct {
	Date      string `json:"date"`
	Count     int    `json:"count"`
	OK        bool   `json:"ok"`
	ErrorCode int    `json:"error_code,omitempty"`
	Error     string `json:"error,omitempty"`
}

// ProbeHistoricalCheckins reads a single currently mapped, active, allowed
// employee for up to three single-day windows. It does not persist source data,
// move cursors or expose a person's identity/raw response in its report.
func (c *Client) ProbeHistoricalCheckins(ctx context.Context, dates []string) ([]HistoryProbeResult, error) {
	if len(dates) == 0 || len(dates) > 3 {
		return nil, errors.New("历史探测限1至3个单日窗口")
	}
	parsed := []time.Time{}
	for _, date := range dates {
		day, e := time.ParseInLocation("2006-01-02", date, core.Location)
		if e != nil || day.Format("2006-01-02") != date || !day.AddDate(0, 0, 1).Before(time.Now()) {
			return nil, errors.New("历史探测日期须为已结束的有效自然日")
		}
		parsed = append(parsed, day)
	}
	if !c.claim() {
		return nil, ErrSyncRunning
	}
	defer c.release()
	filter, e := c.filter(ctx)
	if e != nil {
		return nil, e
	}
	rows, e := core.Model("biz_hrm_employee").Ctx(ctx).Fields("id,wecom_userid").Where("status", "active").WhereNot("wecom_userid", "").Order("employee_no,id").All()
	if e != nil {
		return nil, dbError("history probe employee")
	}
	uid := ""
	for _, row := range rows {
		candidate := row["wecom_userid"].String()
		if candidate == "" {
			continue
		}
		exists, e := core.Model("wecom_user").Ctx(ctx).Where("wecom_userid", candidate).Exist()
		if e != nil {
			return nil, dbError("history probe directory")
		}
		if !exists {
			continue
		}
		allowed, e := filter.applicantAllowed(ctx, candidate)
		if e != nil {
			return nil, e
		}
		if allowed {
			uid = candidate
			break
		}
	}
	if uid == "" {
		return nil, errors.New("没有当前有效且在同步范围内的企业微信映射员工")
	}
	report := []HistoryProbeResult{}
	for i, day := range parsed {
		result := HistoryProbeResult{Date: dates[i]}
		var out struct {
			apiError
			Records json.RawMessage `json:"checkindata"`
		}
		end := day.AddDate(0, 0, 1).Unix() - 1
		e = c.api(ctx, "checkin", http.MethodPost, "/checkin/getcheckindata", nil, g.Map{"opencheckindatatype": 3, "useridlist": []string{uid}, "starttime": day.Unix(), "endtime": end}, &out)
		if e != nil {
			var apiErr *APIError
			if errors.As(e, &apiErr) {
				result.ErrorCode = apiErr.Code
				result.Error = "企业微信拒绝此查询"
			} else {
				result.Error = "查询失败，请核对凭据、权限或网络"
			}
			if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
				result.Error = "探测已取消或超时"
			}
		} else {
			result.OK = true
			var records []json.RawMessage
			if len(out.Records) == 0 || string(out.Records) == "null" || json.Unmarshal(out.Records, &records) != nil {
				result.OK = false
				result.Error = "接口响应缺少有效打卡列表"
			}
			for _, raw := range records {
				var p Checkin
				if json.Unmarshal(raw, &p) != nil || p.UserID != uid {
					result.OK = false
					result.Error = "接口返回了非预期的员工或数据格式"
					break
				}
				stamp := p.Time
				if stamp <= 0 {
					stamp = p.ScheduledTime
				}
				if stamp < day.Unix() || stamp > end {
					result.OK = false
					result.Error = "接口返回了查询时间范围之外的记录"
					break
				}
				result.Count++
			}
			if !result.OK {
				result.Count = 0
			}
		}
		report = append(report, result)
		if ctx.Err() != nil {
			break
		}
	}
	return report, nil
}
