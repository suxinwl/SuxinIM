package hrm

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/approvalview"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

type approvalFormField struct {
	Label string `json:"label"`
	Value string `json:"value"`
}
type approvalFormStep struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	StatusName string `json:"status_name"`
	Time       string `json:"time"`
	Comment    string `json:"comment"`
}
type approvalForm struct {
	Title     string              `json:"title"`
	Applicant g.Map               `json:"applicant"`
	ApplyTime string              `json:"apply_time"`
	Fields    []approvalFormField `json:"fields"`
	Timeline  []approvalFormStep  `json:"timeline"`
	Notice    string              `json:"notice"`
}
type sourceApprovalForm struct {
	SPNo      string          `json:"sp_no"`
	Name      string          `json:"sp_name"`
	ApplyTime json.RawMessage `json:"apply_time"`
	Applyer   struct {
		UserID string `json:"userid"`
	} `json:"applyer"`
	ApplyData struct {
		Contents []sourceApprovalControl `json:"contents"`
	} `json:"apply_data"`
	Records []struct {
		Details []struct {
			Approver struct {
				UserID string `json:"userid"`
			} `json:"approver"`
			Speech string          `json:"speech"`
			Status int             `json:"sp_status"`
			Time   json.RawMessage `json:"sptime"`
		} `json:"details"`
	} `json:"sp_record"`
}
type sourceApprovalControl struct {
	Control string                       `json:"control"`
	Title   []approvalview.LocalizedText `json:"title"`
	Value   json.RawMessage              `json:"value"`
}

func buildApprovalForm(ctx context.Context, row gdb.Record, kind string, item g.Map) (approvalForm, error) {
	form := approvalForm{Title: absenceTitles[kind] + "审批详情", Applicant: g.Map{"name": item["employee_name"], "employee_no": item["employee_no"], "department_names": item["department_names"]}, Fields: []approvalFormField{}, Timeline: []approvalFormStep{}}
	add := func(label, value string) {
		if strings.TrimSpace(value) == "" {
			value = "未提供"
		}
		form.Fields = append(form.Fields, approvalFormField{label, limitedFormText(value)})
	}
	if row["source"].String() != "wecom" {
		form.Notice = "手工或历史导入记录，未提供企业微信原始审批表单与流程。"
		return form, nil
	}
	doc, ok, err := loadSourceApproval(ctx, row, kind)
	if err != nil {
		return form, err
	}
	if !ok {
		form.Notice = "未取得可核对的企业微信原始表单，当前展示HRM业务记录。"
		return form, nil
	}
	if strings.TrimSpace(doc.Name) != "" {
		form.Title = limitedFormText(doc.Name)
	}
	form.ApplyTime = approvalTimestamp(doc.ApplyTime)
	for i, control := range doc.ApplyData.Contents {
		if i >= 200 {
			break
		}
		label := approvalview.PreferredLabel(control.Title)
		if label == "" {
			label = sourceControlTitle(control.Control)
		}
		add(label, sourceControlValue(control))
	}
	ids := []string{}
	for _, record := range doc.Records {
		for _, detail := range record.Details {
			if detail.Approver.UserID != "" {
				ids = append(ids, detail.Approver.UserID)
			}
		}
	}
	names, err := approvalParticipantNames(ctx, ids)
	if err != nil {
		return form, fmt.Errorf("无法读取审批人姓名映射")
	}
	for _, record := range doc.Records {
		for _, detail := range record.Details {
			if len(form.Timeline) >= 200 {
				break
			}
			name := names[detail.Approver.UserID]
			if name == "" {
				name = "未匹配审批人"
			}
			status, label := "unknown", "未识别状态"
			switch detail.Status {
			case 1:
				status, label = "pending", "审批中"
			case 2:
				status, label = "approved", "已同意"
			case 3:
				status, label = "rejected", "已驳回"
			case 4:
				status, label = "transferred", "已转审"
			case 11:
				status, label = "returned", "已退回"
			case 12:
				status, label = "added_approver", "已加签"
			case 13:
				status, label = "approved_added_approver", "已同意并加签"
			}
			form.Timeline = append(form.Timeline, approvalFormStep{Name: name, Status: status, StatusName: label, Time: approvalTimestamp(detail.Time), Comment: limitedFormText(detail.Speech)})
		}
	}
	form.Notice = "上方业务状态、时间和事由以当前HRM记录为准；原表单及流程显示已同步的企业微信内容。"
	if len(form.Timeline) == 0 {
		form.Notice += " 暂无已同步的审批流程。"
	}
	return form, nil
}

// A document is displayed only after matching the selected business record's
// external reference AND its explicitly mapped applicant, never from a request-
// supplied approval number or by matching a person's name.
func loadSourceApproval(ctx context.Context, row gdb.Record, kind string) (sourceApprovalForm, bool, error) {
	var empty sourceApprovalForm
	ref := row["external_ref"].String()
	if ref == "" {
		return empty, false, nil
	}
	employee, err := core.Model("biz_hrm_employee").Ctx(ctx).Where("id", row["employee_id"].String()).Fields("wecom_userid").One()
	if err != nil {
		return empty, false, fmt.Errorf("无法核对审批申请人")
	}
	userid := employee["wecom_userid"].String()
	if userid == "" {
		members, err := core.Model("wecom_user").Ctx(ctx).Where("employee_id", row["employee_id"].String()).Fields("wecom_userid").Limit(2).All()
		if err != nil {
			return empty, false, fmt.Errorf("无法核对审批申请人")
		}
		if len(members) == 1 {
			userid = members[0]["wecom_userid"].String()
		}
	}
	if userid == "" {
		return empty, false, nil
	}
	parse := func(raw string) (sourceApprovalForm, bool) {
		var doc sourceApprovalForm
		if len(raw) == 0 || len(raw) > 4<<20 || json.Unmarshal([]byte(raw), &doc) != nil {
			return doc, false
		}
		if doc.SPNo == "" {
			var wrapper struct {
				Info sourceApprovalForm `json:"info"`
			}
			if json.Unmarshal([]byte(raw), &wrapper) == nil {
				doc = wrapper.Info
			}
		}
		return doc, doc.SPNo == ref && doc.Applyer.UserID == userid
	}
	latest, err := core.Model("wecom_approval_raw").Ctx(ctx).Where("sp_no", ref).Where("approval_type", kind).Where("wecom_userid", userid).Fields("detail_json,raw_json").One()
	if err != nil {
		return empty, false, fmt.Errorf("无法读取已同步的审批表单")
	}
	for _, raw := range []string{latest["detail_json"].String(), latest["raw_json"].String(), row["raw_json"].String()} {
		if doc, ok := parse(raw); ok {
			return doc, true, nil
		}
	}
	return empty, false, nil
}

func approvalParticipantNames(ctx context.Context, ids []string) (map[string]string, error) {
	names := map[string]string{}
	if len(ids) == 0 {
		return names, nil
	}
	set := map[string]bool{}
	for _, id := range ids {
		if id != "" {
			set[id] = true
		}
	}
	ids = ids[:0]
	for id := range set {
		ids = append(ids, id)
	}
	employees, err := core.Model("biz_hrm_employee").Ctx(ctx).WhereIn("wecom_userid", ids).Fields("id,wecom_userid,name").All()
	if err != nil {
		return nil, err
	}
	for _, employee := range employees {
		id, name := employee["wecom_userid"].String(), strings.TrimSpace(employee["name"].String())
		if name != "" && name != id && name != employee["id"].String() {
			names[id] = name
		}
	}
	members, err := core.Model("wecom_user").Ctx(ctx).WhereIn("wecom_userid", ids).Fields("wecom_userid,name,employee_id").All()
	if err != nil {
		return nil, err
	}
	for _, member := range members {
		id, name := member["wecom_userid"].String(), strings.TrimSpace(member["name"].String())
		if name != "" && name != id && name != member["employee_id"].String() {
			names[id] = name
		}
	}
	return names, nil
}

func limitedFormText(value string) string {
	text := []rune(strings.TrimSpace(value))
	if len(text) > 10000 {
		return string(text[:10000]) + "…"
	}
	return string(text)
}

func approvalTimestamp(raw json.RawMessage) string {
	value := strings.Trim(strings.TrimSpace(string(raw)), "\"")
	stamp, err := strconv.ParseInt(value, 10, 64)
	if err != nil || stamp <= 0 || stamp > 253402214399 {
		return ""
	}
	return time.Unix(stamp, 0).In(core.Location).Format("2006-01-02 15:04:05")
}

func sourceControlTitle(control string) string {
	if label := map[string]string{"Text": "文本", "Textarea": "多行文本", "Vacation": "请假信息", "Attendance": "考勤信息", "DateRange": "时间区间", "Selector": "选择项", "Date": "日期", "File": "附件", "Contact": "人员", "Location": "位置"}[control]; label != "" {
		return label
	}
	return "审批字段"
}

func sourceControlValue(control sourceApprovalControl) string {
	var value struct {
		Text     string `json:"text"`
		Selector struct {
			Options []struct {
				Value []approvalview.LocalizedText `json:"value"`
			} `json:"options"`
		} `json:"selector"`
		Date struct {
			Timestamp json.RawMessage `json:"s_timestamp"`
		} `json:"date"`
		Attendance struct {
			Range json.RawMessage `json:"date_range"`
		} `json:"attendance"`
		Range    json.RawMessage `json:"date_range"`
		Vacation struct {
			Attendance struct {
				Range json.RawMessage `json:"date_range"`
			} `json:"attendance"`
			Selector struct {
				Options []struct {
					Value []approvalview.LocalizedText `json:"value"`
				} `json:"options"`
			} `json:"selector"`
		} `json:"vacation"`
	}
	if json.Unmarshal(control.Value, &value) != nil {
		return "字段数据暂不可读取"
	}
	labels := func(options []struct {
		Value []approvalview.LocalizedText `json:"value"`
	}) string {
		out := []string{}
		for _, option := range options {
			if label := approvalview.PreferredLabel(option.Value); label != "" {
				out = append(out, label)
			}
		}
		if len(out) == 0 {
			return "未提供选项名称"
		}
		return strings.Join(out, "、")
	}
	dateRange := func(raw json.RawMessage) string {
		var period struct {
			Begin json.RawMessage `json:"new_begin"`
			End   json.RawMessage `json:"new_end"`
		}
		if json.Unmarshal(raw, &period) != nil {
			return "未提供时间区间"
		}
		from, to := approvalTimestamp(period.Begin), approvalTimestamp(period.End)
		if from == "" || to == "" {
			return "未提供时间区间"
		}
		if to < from {
			return "时间区间无效"
		}
		return from + " 至 " + to
	}
	switch control.Control {
	case "Text", "Textarea":
		return value.Text
	case "Selector":
		return labels(value.Selector.Options)
	case "Date":
		return approvalTimestamp(value.Date.Timestamp)
	case "Vacation":
		return labels(value.Vacation.Selector.Options) + "；" + dateRange(value.Vacation.Attendance.Range)
	case "Attendance":
		return dateRange(value.Attendance.Range)
	case "DateRange":
		return dateRange(value.Range)
	case "File":
		return "附件内容暂未在此页展示"
	case "Contact":
		return "人员控件内容暂未在此页展示"
	default:
		return "此类控件暂不支持展示"
	}
}
