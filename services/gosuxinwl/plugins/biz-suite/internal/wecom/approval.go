package wecom

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/approvalview"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/hrm"
)

type jsonRecord json.RawMessage

func (r *jsonRecord) UnmarshalJSON(b []byte) error { *r = append((*r)[:0], b...); return nil }
func (r jsonRecord) decode(v any) error            { return json.Unmarshal(r, v) }

// Mapping uses exact top-level control IDs from the WeCom template. A title or
// a recursively encountered "start"/"userid" is never an attendance field.
type Mapping struct {
	AttendanceControlID  string `json:"attendance_control_id"`
	ReasonControlID      string `json:"reason_control_id"`
	DestinationControlID string `json:"destination_control_id"`
}
type Template struct {
	TemplateID   string  `json:"template_id"`
	ApprovalType string  `json:"approval_type"`
	Enabled      int     `json:"enabled"`
	Mapping      Mapping `json:"mapping"`
}
type approvalContent struct {
	Control string          `json:"control"`
	ID      string          `json:"id"`
	Value   json.RawMessage `json:"value"`
}
type approvalDetail struct {
	SPNo       string `json:"sp_no"`
	TemplateID string `json:"template_id"`
	Status     int    `json:"sp_status"`
	ApplyTime  int64  `json:"apply_time"`
	Applyer    struct {
		UserID string `json:"userid"`
	} `json:"applyer"`
	BatchApplyer json.RawMessage `json:"batch_applyer"`
	ApplyData    struct {
		Contents []approvalContent `json:"contents"`
	} `json:"apply_data"`
}

func (c *Client) SyncApprovals(ctx context.Context, start, end time.Time) (int, error) {
	return c.SyncApprovalType(ctx, start, end, "")
}

// A typed job queries only its configured templates and maintains its own cursor.
// The empty kind is retained for legacy raw-approval discovery/imports.
func (c *Client) SyncApprovalType(ctx context.Context, start, end time.Time, kind string) (count int, result error) {
	ctx, changes := collectAttendanceChanges(ctx)
	defer changes.notify(ctx, &result)
	if kind != "" && kind != "unmapped" && !validApprovalKind(kind) {
		return 0, errors.New("invalid approval sync type")
	}
	templateIDs := []string{""}
	allTemplateIDs := []string{}
	if kind != "" && kind != "unmapped" {
		rows, err := core.Model("wecom_approval_template").Ctx(ctx).Where("approval_type", kind).OrderAsc("template_id").All()
		if err != nil {
			return 0, dbError("sync approval templates")
		}
		templateIDs = nil
		for _, row := range rows {
			allTemplateIDs = append(allTemplateIDs, row["template_id"].String())
			if row["enabled"].Int() == 1 {
				templateIDs = append(templateIDs, row["template_id"].String())
			}
		}
		if len(allTemplateIDs) == 0 {
			return 0, errors.New("请先配置对应业务类型的审批模板")
		}
	}
	filter, err := c.filter(ctx)
	if err != nil {
		return 0, err
	}
	if end.IsZero() {
		end = time.Now()
	}
	if start.IsZero() {
		start = end.Add(-31 * 24 * time.Hour)
	}
	var floorErr error
	start, floorErr = applySyncFloor("approval", start, end)
	if floorErr != nil {
		return 0, floorErr
	}
	periods, err := windows(start, end, 31*24*time.Hour)
	if err != nil {
		return 0, err
	}
	failed := 0
	seen := map[string]bool{}
	process := func(sp string) error {
		if sp == "" {
			return errors.New("empty approval number returned by wecom")
		}
		if seen[sp] {
			return nil
		}
		seen[sp] = true
		if kind == "unmapped" {
			known, e := core.Model("wecom_approval_raw").Ctx(ctx).Where("sp_no", sp).One()
			if e != nil {
				return dbError("unmapped approval lookup")
			}
			if len(known) > 0 {
				mapped, e := core.Model("wecom_approval_template").Ctx(ctx).Where("template_id", known["template_id"].String()).Exist()
				if e != nil {
					return dbError("unmapped template lookup")
				}
				if mapped {
					return nil
				}
			}
		}
		var out struct {
			apiError
			Info jsonRecord `json:"info"`
		}
		if err := c.api(ctx, "approval", http.MethodPost, "/oa/getapprovaldetail", nil, g.Map{"sp_no": sp}, &out); err != nil {
			return err
		}
		var detail approvalDetail
		if out.Info.decode(&detail) != nil || detail.SPNo != sp {
			return errors.New("invalid approval detail identity returned by wecom")
		}
		if kind != "" {
			template, e := core.Model("wecom_approval_template").Ctx(ctx).Where("template_id", detail.TemplateID).One()
			if e != nil {
				return dbError("approval type check")
			}
			if kind == "unmapped" {
				if len(template) > 0 {
					return nil
				}
			} else if len(template) == 0 || template["approval_type"].String() != kind {
				return errors.New("wecom returned an approval outside the requested template type")
			}
		}
		// Reconcile revoked/rejected history even if the applicant has since been
		// excluded (or a deleted record no longer carries applicant fields).
		allowed, e := filter.applicantAllowed(ctx, detail.Applyer.UserID)
		if e != nil {
			return e
		}
		if !allowed {
			known, e := core.Model("wecom_approval_raw").Ctx(ctx).Where("sp_no", sp).Exist()
			if e != nil {
				return dbError("approval scope history")
			}
			state := normalizeStatus(strconv.Itoa(detail.Status))
			if !known || state == "approved" {
				return nil
			}
		}
		err := saveApproval(ctx, detail, string(out.Info))
		var pe *PartialError
		if errors.As(err, &pe) {
			failed += pe.Failed
			return nil
		}
		if err != nil {
			return err
		}
		count++
		return nil
	}
	for _, templateID := range templateIDs {
		for _, period := range periods {
			cursor := ""
			visited := map[string]bool{}
			for {
				if visited[cursor] {
					return count, errors.New("wecom approval pagination repeated a cursor")
				}
				visited[cursor] = true
				var page struct {
					apiError
					Numbers []string `json:"sp_no_list"`
					Next    *string  `json:"new_next_cursor"`
				}
				// Official API 91816: cursor/next_cursor are deprecated. Missing
				// new_next_cursor, not a short/empty page, signals end of pagination.
				body := g.Map{"starttime": strconv.FormatInt(period[0].Unix(), 10), "endtime": strconv.FormatInt(period[1].Unix(), 10), "new_cursor": cursor, "size": 100}
				if templateID != "" {
					body["filters"] = []g.Map{{"key": "template_id", "value": templateID}}
				}
				if err := c.api(ctx, "approval", http.MethodPost, "/oa/getapprovalinfo", nil, body, &page); err != nil {
					return count, err
				}
				for _, sp := range page.Numbers {
					if err = process(sp); err != nil {
						return count, err
					}
				}
				if page.Next == nil {
					break
				}
				if *page.Next == "" {
					return count, errors.New("wecom returned an empty continuation cursor")
				}
				cursor = *page.Next
			}
		}
	}
	// getapprovalinfo filters submission time, not last-modified time. Revisit
	// every known non-canceled approval so an old approved leave can be revoked.
	// Batched keyset reads avoid an arbitrary 100/200-row reconciliation limit.
	last := ""
	for {
		query := core.Model("wecom_approval_raw").Ctx(ctx).Where("sp_no > ?", last).Where("status != ?", "canceled")
		if kind == "unmapped" {
			query = query.Where("approval_type", "unknown")
		} else if kind != "" {
			query = query.WhereIn("template_id", allTemplateIDs)
		}
		rows, e := query.Fields("sp_no").OrderAsc("sp_no").Limit(100).All()
		if e != nil {
			return count, dbError("approval reconciliation")
		}
		for _, row := range rows {
			sp := row["sp_no"].String()
			if err = process(sp); err != nil {
				return count, err
			}
			last = sp
		}
		if len(rows) < 100 {
			break
		}
	}
	return count, partial(failed)
}

func normalizeStatus(value string) string {
	switch value {
	case "1":
		return "pending"
	case "2", "10":
		return "approved"
	case "3":
		return "rejected"
	case "4", "6", "7":
		return "canceled"
	default:
		return "unknown"
	}
}

func saveApproval(ctx context.Context, d approvalDetail, raw string) (result error) {
	ctx, changes := collectAttendanceChanges(ctx)
	err := core.DB().Transaction(ctx, func(txctx context.Context, _ gdb.TX) error {
		before, previous, err := approvalChanges(txctx, d.SPNo)
		if err != nil {
			return err
		}
		result = persistApproval(txctx, d, raw)
		var partial *PartialError
		if result != nil && !errors.As(result, &partial) {
			return result
		}
		after, current, err := approvalChanges(txctx, d.SPNo)
		if err != nil {
			return err
		}
		if before != after {
			for _, v := range append(previous, current...) {
				recordAttendanceChange(txctx, v.employee, v.start, v.end)
			}
		}
		return enqueueAttendanceChanges(txctx, changes)
	})
	if err != nil {
		return err
	}
	return result
}

func persistApproval(ctx context.Context, d approvalDetail, raw string) error {
	status := normalizeStatus(strconv.Itoa(d.Status))
	template, err := core.Model("wecom_approval_template").Ctx(ctx).Where("template_id", d.TemplateID).One()
	if err != nil {
		return dbError("approval template")
	}
	kind := "unknown"
	if len(template) > 0 {
		kind = template["approval_type"].String()
	}
	data := g.Map{"id": stableID("approval:" + d.SPNo), "sp_no": d.SPNo, "template_id": d.TemplateID, "wecom_userid": d.Applyer.UserID, "approval_type": kind, "status": status, "raw_json": raw, "detail_json": raw, "processing_error": "", "synced_at": now()}
	if d.ApplyTime > 0 {
		data["apply_time"] = unixTime(d.ApplyTime)
	}
	if err = upsert(ctx, "wecom_approval_raw", "sp_no", d.SPNo, data); err != nil {
		return err
	}
	var mapping Mapping
	var problem error
	switch {
	case status == "unknown":
		problem = errors.New("unknown approval state; requires review")
	case len(template) == 0 || template["enabled"].Int() != 1:
		problem = errors.New("approval template is not enabled or mapped")
	case !validApprovalKind(kind):
		problem = errors.New("approval template type must be leave, out, trip or overtime")
	case json.Unmarshal([]byte(template["mapping_json"].String()), &mapping) != nil || mapping.AttendanceControlID == "":
		problem = errors.New("approval template requires an exact attendance_control_id")
	}
	// Cancel prior attendance even if a template was subsequently disabled or
	// the deleted approval no longer returns its controls/applicant.
	if status != "approved" || problem != nil {
		err = core.DB().Transaction(ctx, func(txctx context.Context, _ gdb.TX) error {
			next := status
			if next == "unknown" || (problem != nil && next == "approved") {
				next = "pending"
			}
			for _, target := range approvalKinds {
				if _, e := core.Model("biz_hrm_attendance_"+target).Ctx(txctx).Where("source", "wecom").Where("external_ref", d.SPNo).Update(g.Map{"approval_status": next, "updated_at": now()}); e != nil {
					return dbError("approval cancellation")
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if problem == nil && (status == "canceled" || status == "rejected") {
		return nil
	}
	var employee string
	if problem == nil {
		problem = validateSingleApplicantBatch(d.BatchApplyer)
		if problem == nil {
			employee, problem = employeeIDByWeCom(ctx, d.Applyer.UserID)
		}
	}
	var normalized normalizedApproval
	if problem == nil {
		normalized, problem = decodeApproval(d, mapping, kind)
	}
	if problem == nil {
		err = core.DB().Transaction(ctx, func(txctx context.Context, _ gdb.TX) error {
			for _, other := range approvalKinds {
				if other == kind {
					continue
				}
				if _, e := core.Model("biz_hrm_attendance_"+other).Ctx(txctx).Where("source", "wecom").Where("external_ref", d.SPNo).Update(g.Map{"approval_status": "canceled", "updated_at": now()}); e != nil {
					return dbError("approval remapping")
				}
			}
			if kind == "leave" {
				_, e := hrm.UpsertLeave(txctx, hrm.Leave{EmployeeID: employee, Source: "wecom", ExternalRef: d.SPNo, ApprovalStatus: status, LeaveType: normalized.LeaveType, StartTime: normalized.Start, EndTime: normalized.End, Duration: normalized.Hours, Reason: normalized.Reason, RawJSON: raw})
				return e
			}
			if kind == "trip" {
				_, e := hrm.UpsertTrip(txctx, hrm.Out{EmployeeID: employee, Source: "wecom", ExternalRef: d.SPNo, ApprovalStatus: status, StartTime: normalized.Start, EndTime: normalized.End, Duration: normalized.Hours, Destination: normalized.Destination, Reason: normalized.Reason, RawJSON: raw})
				return e
			}
			if kind == "overtime" {
				_, e := hrm.UpsertOvertime(txctx, hrm.Out{EmployeeID: employee, Source: "wecom", ExternalRef: d.SPNo, ApprovalStatus: status, StartTime: normalized.Start, EndTime: normalized.End, Duration: normalized.Hours, Reason: normalized.Reason, RawJSON: raw})
				return e
			}
			_, e := hrm.UpsertOut(txctx, hrm.Out{EmployeeID: employee, Source: "wecom", ExternalRef: d.SPNo, ApprovalStatus: status, StartTime: normalized.Start, EndTime: normalized.End, Duration: normalized.Hours, Destination: normalized.Destination, Reason: normalized.Reason, RawJSON: raw})
			return e
		})
		if err != nil {
			problem = errors.New("normalized approval failed HRM validation or persistence")
		}
	}
	if problem != nil {
		// Do not retain a previously approved HRM result when its new payload
		// cannot be interpreted after a template/mapping change.
		for _, target := range approvalKinds {
			next := status
			if next == "approved" || next == "unknown" {
				next = "pending"
			}
			if _, e := core.Model("biz_hrm_attendance_"+target).Ctx(ctx).Where("source", "wecom").Where("external_ref", d.SPNo).Update(g.Map{"approval_status": next, "updated_at": now()}); e != nil {
				return dbError("approval exception reconciliation")
			}
		}
		if _, err = core.Model("wecom_approval_raw").Ctx(ctx).Where("sp_no", d.SPNo).Update(g.Map{"processing_error": problem.Error()}); err != nil {
			return dbError("approval exception")
		}
		return &PartialError{1}
	}
	return nil
}

func validateSingleApplicantBatch(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	value := strings.TrimSpace(string(raw))
	if value == "null" {
		return nil
	}
	// Single-person approvals also return an empty batch_applyer array.
	// Decode its shape rather than treating the JSON byte length as a count.
	var applicants []json.RawMessage
	if err := json.Unmarshal([]byte(value), &applicants); err != nil {
		return errors.New("invalid batch applicants; raw approval retained")
	}
	if len(applicants) > 0 {
		return errors.New("batch applicants require explicit per-employee mapping; raw approval retained")
	}
	return nil
}

type normalizedApproval struct {
	Start, End, Reason, Destination, LeaveType string
	Hours                                      float64
}
type dateRange struct {
	Begin    int64   `json:"new_begin"`
	End      int64   `json:"new_end"`
	Duration float64 `json:"new_duration"`
}
type attendanceValue struct {
	Type  int       `json:"type"`
	Range dateRange `json:"date_range"`
}

func decodeApproval(d approvalDetail, m Mapping, kind string) (normalizedApproval, error) {
	var out normalizedApproval
	if !validApprovalKind(kind) {
		return out, errors.New("unsupported approval template type")
	}
	controls := map[string]approvalContent{}
	for _, v := range d.ApplyData.Contents {
		if _, ok := controls[v.ID]; ok {
			return out, errors.New("duplicate control ID in approval")
		}
		controls[v.ID] = v
	}
	control, ok := controls[m.AttendanceControlID]
	if !ok {
		return out, errors.New("configured attendance control is absent")
	}
	var value struct {
		Attendance attendanceValue `json:"attendance"`
		Range      dateRange       `json:"date_range"`
		Vacation   struct {
			Attendance attendanceValue `json:"attendance"`
			Selector   struct {
				Options []struct {
					Value []approvalview.LocalizedText `json:"value"`
				} `json:"options"`
			} `json:"selector"`
		} `json:"vacation"`
	}
	if json.Unmarshal(control.Value, &value) != nil {
		return out, errors.New("invalid approval control value")
	}
	var dr dateRange
	switch control.Control {
	case "Vacation":
		if kind != "leave" || value.Vacation.Attendance.Type != 1 {
			return out, errors.New("Vacation control does not match leave template")
		}
		dr = value.Vacation.Attendance.Range
		if len(value.Vacation.Selector.Options) == 1 {
			out.LeaveType = approvalview.PreferredLabel(value.Vacation.Selector.Options[0].Value)
		}
	case "Attendance":
		// Official 91983: 2 is punch correction; overtime is 5.
		expected := map[string]int{"leave": 1, "trip": 3, "out": 4, "overtime": 5}[kind]
		if value.Attendance.Type != expected {
			return out, errors.New("Attendance control type does not match template")
		}
		dr = value.Attendance.Range
	case "DateRange":
		dr = value.Range
	default:
		return out, errors.New("attendance control must be Vacation, Attendance or DateRange")
	}
	if kind == "leave" && out.LeaveType == "" {
		out.LeaveType = approvalview.UnknownLeaveType
	}
	if dr.Begin <= 0 || dr.End <= dr.Begin || dr.Duration <= 0 {
		return out, errors.New("approval has invalid start/end/duration")
	}
	out.Start = unixTime(dr.Begin)
	out.End = unixTime(dr.End)
	out.Hours = dr.Duration / 3600
	readText := func(id string) (string, error) {
		if id == "" {
			return "", nil
		}
		v, ok := controls[id]
		if !ok {
			return "", errors.New("configured text control is absent")
		}
		if v.Control != "Text" && v.Control != "Textarea" {
			return "", errors.New("reason/destination control must be Text or Textarea")
		}
		var text struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(v.Value, &text) != nil {
			return "", errors.New("invalid text control")
		}
		return strings.TrimSpace(text.Text), nil
	}
	var err error
	if out.Reason, err = readText(m.ReasonControlID); err != nil {
		return out, err
	}
	out.Destination, err = readText(m.DestinationControlID)
	return out, err
}
