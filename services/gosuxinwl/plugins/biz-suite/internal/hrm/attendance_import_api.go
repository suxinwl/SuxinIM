package hrm

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/xuri/excelize/v2"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

func attImports(r *ghttp.Request) {
	if !queryScope(r.Context()).All {
		r.Response.WriteStatus(403)
		return
	}
	rows, e := core.Model("biz_att_import").Ctx(r.Context()).Fields("id,kind,filename,month,status,created_at").OrderDesc("created_at").All()
	if e != nil {
		attError(r, e)
		return
	}
	attOK(r, g.Map{"items": rows})
}
func attReadImport(ctx context.Context, id string) (gdb.Record, ImportPreview, map[string]string, error) {
	row, e := core.Model("biz_att_import").Ctx(ctx).Where("id", id).One()
	var p ImportPreview
	m := map[string]string{}
	if e != nil {
		return row, p, m, e
	}
	if len(row) == 0 {
		return row, p, m, fmt.Errorf("导入记录不存在")
	}
	if e = json.Unmarshal([]byte(row["preview_json"].String()), &p); e != nil {
		return row, p, m, e
	}
	e = json.Unmarshal([]byte(row["mappings_json"].String()), &m)
	return row, p, m, e
}
func attImportDetail(r *ghttp.Request) {
	if !queryScope(r.Context()).All {
		r.Response.WriteStatus(403)
		return
	}
	row, p, m, e := attReadImport(r.Context(), r.GetRouter("id").String())
	if e != nil {
		attError(r, e)
		return
	}
	attOK(r, g.Map{"id": row["id"], "status": row["status"], "preview": p, "mappings": m})
}

// ImportAttendanceEvidence is also used by the local bootstrap tool. Parsing
// and matching never create employees or enable accounts.
func ImportAttendanceEvidence(ctx context.Context, reader io.Reader, name string) (string, ImportPreview, map[string]string, error) {
	p, e := ParseAttendanceWorkbook(reader, name)
	if e != nil {
		return "", p, nil, e
	}
	attMu.Lock()
	defer attMu.Unlock()
	old, e := core.Model("biz_att_import").Ctx(ctx).Where("content_hash", p.SHA256).One()
	if e != nil {
		return "", p, nil, e
	}
	if len(old) > 0 {
		m := map[string]string{}
		_ = json.Unmarshal([]byte(old["mappings_json"].String()), &m)
		if p.Kind == "rules" && old["preview_json"].String() != attJSON(p) {
			e = core.DB().Transaction(ctx, func(c context.Context, _ gdb.TX) error {
				if err := attAudit(c, "import", old["id"].String(), "reparse_rules", "新版上班时间列解析；保留原预览及已确认制度，不修改核定基准", old["preview_json"].String(), p); err != nil {
					return err
				}
				_, err := core.Model("biz_att_import").Ctx(c).Where("id", old["id"]).Update(g.Map{"preview_json": attJSON(p), "status": "preview"})
				return err
			})
			if e != nil {
				return "", p, nil, e
			}
		}
		return old["id"].String(), p, m, nil
	}
	people, e := core.Model("biz_hrm_employee").Ctx(ctx).Fields("id,name,employee_no").All()
	if e != nil {
		return "", p, nil, e
	}
	byNo := map[string][]gdb.Record{}
	for _, person := range people {
		no := person["employee_no"].String()
		if no != "" {
			byNo[no] = append(byNo[no], person)
		}
	}
	counts := map[string]int{}
	for _, person := range p.Employees {
		counts[person.EmployeeNo]++
	}
	m := map[string]string{}
	for _, person := range p.Employees {
		candidates := byNo[person.EmployeeNo]
		if person.EmployeeNo != "" && counts[person.EmployeeNo] == 1 && len(candidates) == 1 && candidates[0]["name"].String() == person.Name {
			m[person.SourceKey] = candidates[0]["id"].String()
		}
	}
	id := guid.S()
	_, e = core.Model("biz_att_import").Ctx(ctx).Data(g.Map{"id": id, "kind": p.Kind, "filename": p.Filename, "content_hash": p.SHA256, "month": p.Month, "preview_json": attJSON(p), "mappings_json": attJSON(m), "status": "preview", "created_by": core.UID(ctx), "created_at": core.Now()}).Insert()
	return id, p, m, e
}
func attImportUpload(r *ghttp.Request) {
	r.Body = http.MaxBytesReader(nil, r.Body, 21<<20)
	file, header, e := r.Request.FormFile("file")
	if e != nil {
		attError(r, fmt.Errorf("请选择20MiB以内的xlsx工作簿"))
		return
	}
	defer file.Close()
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	id, p, m, e := ImportAttendanceEvidence(r.Context(), file, header.Filename)
	if e != nil {
		attError(r, e)
		return
	}
	attOK(r, g.Map{"id": id, "preview": p, "mappings": m})
}
func attImportMappings(r *ghttp.Request) {
	var in struct {
		Mappings map[string]string `json:"mappings"`
		Reason   string            `json:"reason"`
	}
	if e := r.Parse(&in); e != nil {
		attError(r, e)
		return
	}
	attMu.Lock()
	defer attMu.Unlock()
	ctx := r.Context()
	id := r.GetRouter("id").String()
	_, p, old, e := attReadImport(ctx, id)
	if e != nil {
		attError(r, e)
		return
	}
	keys := map[string]bool{}
	for _, person := range p.Employees {
		keys[person.SourceKey] = true
	}
	seen := map[string]bool{}
	for key, employeeID := range in.Mappings {
		if !keys[key] {
			attError(r, fmt.Errorf("映射来源不存在"))
			return
		}
		if employeeID == "" {
			delete(in.Mappings, key)
			continue
		}
		exists, e := core.Model("biz_hrm_employee").Ctx(ctx).Where("id", employeeID).Exist()
		if e != nil || !exists || seen[employeeID] {
			attError(r, fmt.Errorf("员工不存在或被重复映射，请核对工号和部门"))
			return
		}
		seen[employeeID] = true
	}
	e = core.DB().Transaction(ctx, func(c context.Context, tx gdb.TX) error {
		_, e := core.Model("biz_att_import").Ctx(c).Where("id", id).Update(g.Map{"mappings_json": attJSON(in.Mappings)})
		if e != nil {
			return e
		}
		return attAudit(c, "import", id, "mapping", defaultString(in.Reason, "人事确认员工映射"), old, in.Mappings)
	})
	if e != nil {
		attError(r, e)
		return
	}
	attOK(r, g.Map{"id": id, "mappings": in.Mappings})
}
func attImportRules(r *ghttp.Request) {
	ids, e := CreateImportedRuleDrafts(r.Context(), r.GetRouter("id").String())
	if e != nil {
		attError(r, e)
		return
	}
	attOK(r, g.Map{"count": len(ids), "ids": ids})
}
func CreateImportedRuleDrafts(ctx context.Context, id string) ([]string, error) {
	return createLocalImportedDrafts(ctx, id, false)
}

func attDraftShifts(text string) []eng.Shift {
	out := []eng.Shift{}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		pairs := attendanceRuleTimePattern.FindAllStringSubmatch(line, -1)
		if len(pairs) == 0 {
			continue
		}
		s := eng.Shift{ID: fmt.Sprintf("shift%d", len(out)+1), Name: fmt.Sprintf("班次%d（待确认）", len(out)+1)}
		day := 0
		previous := -1
		for i, p := range pairs {
			h, _ := strconv.Atoi(p[1])
			m, _ := strconv.Atoi(p[2])
			a := h*60 + m
			h, _ = strconv.Atoi(p[3])
			m, _ = strconv.Atoi(p[4])
			b := h*60 + m
			if a < previous {
				day += 1440
			}
			a += day
			b += day
			if b <= a {
				b += 1440
			}
			s.Segments = append(s.Segments, eng.Segment{ID: fmt.Sprintf("segment%d", i+1), Name: fmt.Sprintf("工作段%d", i+1), Kind: "work", StartMinute: a, EndMinute: b, PunchMode: "both"})
			previous = b % 1440
			day = (b / 1440) * 1440
		}
		out = append(out, s)
	}
	return out
}
func attImportCompare(r *ghttp.Request) {
	if !queryScope(r.Context()).All {
		r.Response.WriteStatus(403)
		return
	}
	_, p, m, e := attReadImport(r.Context(), r.GetRouter("id").String())
	if e != nil {
		attError(r, e)
		return
	}
	batchID := r.GetQuery("batch_id").String()
	rows, e := core.Model("biz_att_result").Ctx(r.Context()).Where("batch_id", batchID).All()
	if e != nil {
		attError(r, e)
		return
	}
	byPerson := map[string][]gdb.Record{}
	for _, row := range rows {
		byPerson[row["employee_id"].String()] = append(byPerson[row["employee_id"].String()], row)
	}
	out := []g.Map{}
	actualColumns := map[string]string{}
	overtimeColumns := map[string]map[string]string{}
	for _, sheet := range p.Sheets {
		overtimeColumns[sheet.Name] = map[string]string{}
		for _, cell := range sheet.Cells {
			_, row, err := excelize.CellNameToCoordinates(cell.Cell)
			header := strings.Join(strings.Fields(cell.Value), "")
			if err == nil && row <= 3 && (header == "平时加班" || header == "周末加班") {
				col, _, _ := excelize.SplitCellName(cell.Cell)
				overtimeColumns[sheet.Name][header] = col
			}
			if err == nil && row <= 3 && strings.TrimSpace(cell.Value) == "实出勤" {
				col, _, _ := excelize.SplitCellName(cell.Cell)
				actualColumns[sheet.Name] = col
			}
		}
	}
	for _, person := range p.Employees {
		employeeID := m[person.SourceKey]
		column := actualColumns[person.Sheet]
		expected, parseErr := strconv.ParseFloat(person.Summary[column].RawValue, 64)
		if parseErr != nil {
			expected, parseErr = strconv.ParseFloat(person.Summary[column].Value, 64)
		}
		actual := 0.0
		overtime := map[string]float64{"平时加班": 0, "周末加班": 0}
		status := "matched"
		issues := []string{}
		if column == "" {
			status = "unverifiable"
			issues = append(issues, "无法识别实出勤表头")
		}
		if parseErr != nil {
			status = "unverifiable"
			issues = append(issues, "核定实出勤没有可读取的数值，不能按零处理")
		}
		if employeeID == "" {
			status = "unmapped"
			issues = append(issues, "工号缺失、重复或姓名不一致，需人工映射")
		}
		days := byPerson[employeeID]
		if employeeID != "" && len(days) != len(person.Days) {
			status = "unverifiable"
			issues = append(issues, "没有完整月份的计算结果")
		}
		for _, row := range days {
			var result eng.Result
			_ = json.Unmarshal([]byte(row["result_json"].String()), &result)
			if !strings.HasPrefix(row["work_date"].String(), p.Month+"-") {
				status = "unverifiable"
				issues = append(issues, "所选批次不属于该核定月份")
				continue
			}
			actual += float64(result.CreditedSeconds) / 3600
			for _, bucket := range result.OvertimeBuckets {
				if bucket.Category == eng.Workday {
					overtime["平时加班"] += float64(bucket.Seconds) / 3600
				} else if bucket.Category == eng.Restday {
					overtime["周末加班"] += float64(bucket.Seconds) / 3600
				}
			}
			if result.State != "calculated" && result.State != "rest" {
				status = "unverifiable"
			}
		}
		if status == "matched" && math.Abs(actual-expected) > 0.0001 {
			status = "difference"
		}
		metrics := []g.Map{{"name": "核定出勤", "expected_hours": expected, "actual_hours": actual, "difference_hours": actual - expected, "status": status, "source_cell": person.Summary[column]}}
		for _, name := range []string{"平时加班", "周末加班"} {
			cell := person.Summary[overtimeColumns[person.Sheet][name]]
			value, err := strconv.ParseFloat(cell.RawValue, 64)
			if err != nil {
				value, err = strconv.ParseFloat(cell.Value, 64)
			}
			metricStatus := "matched"
			if status == "unmapped" || status == "unverifiable" || err != nil {
				metricStatus = "unverifiable"
			} else if math.Abs(value-overtime[name]) > 0.0001 {
				metricStatus = "difference"
			}
			metrics = append(metrics, g.Map{"name": name, "expected_hours": value, "actual_hours": overtime[name], "difference_hours": overtime[name] - value, "status": metricStatus, "source_cell": cell})
		}
		out = append(out, g.Map{"source_key": person.SourceKey, "name": person.Name, "employee_no": person.EmployeeNo, "employee_id": employeeID, "expected_hours": expected, "actual_hours": actual, "difference_hours": actual - expected, "status": status, "issues": issues, "source_cell": person.Summary[column], "metrics": metrics})
	}
	attOK(r, g.Map{"items": out, "month": p.Month, "batch_id": batchID})
}

func attExport(r *ghttp.Request) {
	// Keep the publication marker and the exported immutable batch consistent
	// with a concurrent month close/reopen.
	attMu.Lock()
	defer attMu.Unlock()
	batchID := r.GetQuery("batch_id").String()
	batch, e := core.Model("biz_att_batch").Ctx(r.Context()).Where("id", batchID).One()
	if e != nil || len(batch) == 0 || batch["status"].String() != "completed" {
		attError(r, fmt.Errorf("请选择计算完成的批次"))
		return
	}
	rows, e := queryScope(r.Context()).apply(core.Model("biz_att_result").Ctx(r.Context()), "employee_id").Where("batch_id", batchID).Order("employee_id,work_date").All()
	if e != nil {
		attError(r, e)
		return
	}
	publication, e := core.Model("biz_att_month").Ctx(r.Context()).Where("batch_id", batchID).One()
	if e != nil {
		attError(r, e)
		return
	}
	meta := attExportPublication(batch, publication)
	file, e := ExportAttendanceResults(rows, batch["start_date"].String(), batch["end_date"].String(), meta)
	if e != nil {
		attError(r, e)
		return
	}
	defer file.Close()
	buf, e := file.WriteToBuffer()
	if e != nil {
		attError(r, e)
		return
	}
	r.Response.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	name := "考勤试算明细-"
	if meta.Formal {
		name = "考勤正式核定签收表-"
	}
	r.Response.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(name+batch["start_date"].String()+".xlsx"))
	r.Response.Write(buf.Bytes())
}

type attExportMetadata struct {
	BatchID       string
	Formal        bool
	Status        string
	Month         string
	MonthRevision int
	ChangePending bool
}

func attExportPublication(batch, publication gdb.Record) attExportMetadata {
	m := attExportMetadata{BatchID: batch["id"].String(), Status: "试算（非当前正式结果）"}
	if len(publication) == 0 || publication["batch_id"].String() != m.BatchID {
		return m
	}
	month := publication["month"].String()
	start, err := attDate(month + "-01")
	if err != nil || batch["start_date"].String() != month+"-01" || batch["end_date"].String() != start.AddDate(0, 1, -1).Format("2006-01-02") {
		return m
	}
	m.Month, m.MonthRevision, m.ChangePending = month, publication["revision"].Int(), publication["change_pending"].Int() != 0
	switch publication["status"].String() {
	case "published":
		m.Formal, m.Status = true, "正式（已发布，未锁定）"
	case "locked":
		m.Formal, m.Status = true, "正式（已月结锁定）"
	case "reviewed":
		m.Status = "试算（已复核，未正式发布）"
	case "draft":
		m.Status = "试算（月份已重开或尚未发布）"
	}
	return m
}

func attExportResultState(state string) string {
	switch state {
	case "calculated":
		return "已计算"
	case "rest":
		return "休息"
	case "configuration_pending":
		return "待配置"
	case "pending":
		return "待计算"
	case "review":
		return "待复核"
	default:
		return "待复核（未知结果状态）"
	}
}

// Metadata is optional for existing callers, which must default to trial rather
// than implying a batch has been approved merely because calculation completed.
func ExportAttendanceResults(rows gdb.Result, start, end string, metadata ...attExportMetadata) (*excelize.File, error) {
	meta := attExportMetadata{Status: "试算（未确认正式发布）"}
	if len(metadata) > 0 {
		meta = metadata[0]
	}
	for _, row := range rows {
		var person AttEmployee
		if e := json.Unmarshal([]byte(row["employee_json"].String()), &person); e != nil {
			return nil, fmt.Errorf("结果人员资料无法读取，不能导出不完整签收表")
		}
		if person.Category != "" && person.Category != "员工" && person.Category != "生产职员" && person.Category != "职员" {
			return nil, fmt.Errorf("%s的考勤分类为%s，请核对后重新计算；不能在导出时遗漏人员", person.Name, person.Category)
		}
		var result eng.Result
		if e := json.Unmarshal([]byte(row["result_json"].String()), &result); e != nil {
			return nil, fmt.Errorf("%s的考勤结果无法读取，不能导出不完整报表", person.Name)
		}
	}
	dates, e := attRange(start, end)
	if e != nil {
		return nil, e
	}
	f := excelize.NewFile()
	categories := []string{"员工", "生产职员", "职员"}
	headerStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"245B78"}, Pattern: 1}, Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true}})
	bodyStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Size: 10}, Alignment: &excelize.Alignment{Vertical: "center", WrapText: true}})
	for i, category := range categories {
		sheet := category
		if i == 0 {
			_ = f.SetSheetName("Sheet1", sheet)
		} else {
			_, _ = f.NewSheet(sheet)
		}
		headers := []any{"姓名", "工号", "部门"}
		for _, d := range dates {
			headers = append(headers, d[5:])
		}
		creditHeader, signatureHeader, title := "试算核定出勤H", "备注（试算未签收）", "考勤试算明细"
		if meta.Formal {
			creditHeader, signatureHeader, title = "核定出勤H", "员工签名", "考勤正式核定签收表"
		}
		headers = append(headers, "应出勤H", "实际工作H", creditHeader, "平时加班H", "休息日加班H", "节假日加班H", "请假H", "外出H", "出差H", "迟到分", "早退分", "缺卡次数", "复核状态", "规则版本", signatureHeader)
		last, _ := excelize.ColumnNumberToName(len(headers))
		_ = f.MergeCell(sheet, "A1", last+"1")
		versionLabel := "未形成正式月结版本"
		if meta.MonthRevision > 0 {
			versionLabel = fmt.Sprintf("%s 月结版本 v%d", meta.Month, meta.MonthRevision)
		}
		changeLabel := ""
		if meta.ChangePending {
			changeLabel = "；有新增证据或制度差异，当前表保留原结果，尚未更新"
		}
		_ = f.SetCellStr(sheet, "A1", start+" 至 "+end+" "+category+title+"｜"+meta.Status+"｜"+versionLabel+changeLabel+"（不同工时指标不可直接相加）")
		_ = f.SetSheetRow(sheet, "A2", &headers)
		_ = f.SetCellStyle(sheet, "A1", last+"2", headerStyle)
		_ = f.SetRowHeight(sheet, 1, 48)
		_ = f.SetRowHeight(sheet, 2, 42)
		_ = f.SetColWidth(sheet, "A", "C", 15)
		_ = f.SetColWidth(sheet, "D", last, 10)
		_ = f.SetPanes(sheet, &excelize.Panes{Freeze: true, XSplit: 3, YSplit: 2, TopLeftCell: "D3", ActivePane: "bottomRight"})
		grouped := map[string][]gdb.Record{}
		order := []string{}
		for _, row := range rows {
			var p AttEmployee
			_ = json.Unmarshal([]byte(row["employee_json"].String()), &p)
			cat := p.Category
			if cat == "" {
				cat = "职员"
			}
			if cat != category {
				continue
			}
			if _, ok := grouped[p.ID]; !ok {
				order = append(order, p.ID)
			}
			grouped[p.ID] = append(grouped[p.ID], row)
		}
		for index, id := range order {
			records := grouped[id]
			var person AttEmployee
			_ = json.Unmarshal([]byte(records[0]["employee_json"].String()), &person)
			values := []any{person.Name, person.EmployeeNo, person.Department}
			days := map[string]eng.Result{}
			var expected, actual, credit, leave, out, trip, late, early float64
			var overtime [3]float64
			missing := 0
			state := "已计算"
			ruleVersions := map[string]bool{}
			for _, record := range records {
				var result eng.Result
				_ = json.Unmarshal([]byte(record["result_json"].String()), &result)
				days[result.Date] = result
				expected += float64(result.StandardSeconds) / 3600
				actual += float64(result.ActualSeconds) / 3600
				credit += float64(result.CreditedSeconds) / 3600
				leave += float64(result.LeaveSeconds) / 3600
				out += float64(result.OutSeconds) / 3600
				trip += float64(result.TripSeconds) / 3600
				late += float64(result.LateSeconds) / 60
				early += float64(result.EarlySeconds) / 60
				missing += result.MissingCount
				ruleVersions[defaultString(record["rule_id"].String(), "未确定制度")+" / v"+defaultString(result.RuleVersion, "未记录")] = true
				if result.State != "rest" && result.State != "calculated" {
					state = "待复核"
				}
				for _, bucket := range result.OvertimeBuckets {
					n := 0
					if bucket.Category == eng.Restday {
						n = 1
					}
					if bucket.Category == eng.Holiday {
						n = 2
					}
					overtime[n] += float64(bucket.Seconds) / 3600
				}
			}
			for _, d := range dates {
				if result, ok := days[d]; ok {
					if result.State == "rest" {
						values = append(values, "休")
					} else if result.State != "calculated" {
						values = append(values, attExportResultState(result.State))
					} else {
						values = append(values, float64(result.CreditedSeconds)/3600)
					}
				} else {
					values = append(values, "未计算")
				}
			}
			versions := make([]string, 0, len(ruleVersions))
			for version := range ruleVersions {
				versions = append(versions, version)
			}
			sort.Strings(versions)
			values = append(values, expected, actual, credit, overtime[0], overtime[1], overtime[2], leave, out, trip, late, early, missing, state, strings.Join(versions, "；"), "")
			cell := fmt.Sprintf("A%d", index+3)
			_ = f.SetSheetRow(sheet, cell, &values)
			_ = f.SetCellStyle(sheet, cell, fmt.Sprintf("%s%d", last, index+3), bodyStyle)
		}
		landscape := "landscape"
		_ = f.SetPageLayout(sheet, &excelize.PageLayoutOptions{Orientation: &landscape})
	}
	// Use only the already-scoped rows; this sheet must not reveal colleagues
	// omitted by the caller's HRM data permissions.
	const versionSheet = "结果与版本依据"
	_, _ = f.NewSheet(versionSheet)
	monthVersion := "未形成正式月结版本"
	if meta.MonthRevision > 0 {
		monthVersion = fmt.Sprintf("%s / v%d", meta.Month, meta.MonthRevision)
	}
	changeNote := "无待处理差异标记"
	if meta.ChangePending {
		changeNote = "有新增证据或制度差异；本表为该批次保留结果，尚未反映后续变化"
	}
	for i, data := range [][]any{{"结果状态", meta.Status}, {"计算批次", meta.BatchID}, {"日期范围", start + " 至 " + end}, {"月结版本", monthVersion}, {"变化提示", changeNote}, {"指标说明", "实际工作、核定出勤与加班分别记录，不可直接相加；规则版本取计算时快照"}} {
		_ = f.SetSheetRow(versionSheet, fmt.Sprintf("A%d", i+1), &data)
	}
	versionHeaders := []any{"姓名", "工号", "日期", "制度标识", "规则版本", "班次版本", "引擎版本", "计算状态"}
	_ = f.SetSheetRow(versionSheet, "A8", &versionHeaders)
	_ = f.SetCellStyle(versionSheet, "A8", "H8", headerStyle)
	for index, row := range rows {
		var person AttEmployee
		var result eng.Result
		_ = json.Unmarshal([]byte(row["employee_json"].String()), &person)
		_ = json.Unmarshal([]byte(row["result_json"].String()), &result)
		values := []any{person.Name, person.EmployeeNo, result.Date, defaultString(row["rule_id"].String(), "未确定制度"), defaultString(result.RuleVersion, "未记录"), defaultString(result.ShiftVersion, "未记录"), defaultString(result.EngineVersion, "未记录"), attExportResultState(result.State)}
		_ = f.SetSheetRow(versionSheet, fmt.Sprintf("A%d", index+9), &values)
	}
	_ = f.SetColWidth(versionSheet, "A", "H", 22)
	_ = f.SetColWidth(versionSheet, "B", "B", 36)
	_ = f.SetPanes(versionSheet, &excelize.Panes{Freeze: true, YSplit: 8, TopLeftCell: "A9", ActivePane: "bottomLeft"})
	return f, nil
}
