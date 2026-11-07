package hrm

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// ImportPreview is evidence, not an instruction to create employees or activate
// attendance rules. Persistence and explicit identity/rule approval belong to
// the caller. Source cells remain immutable even when a known typo is flagged.
type ImportPreview struct {
	Kind      string            `json:"kind"`
	Filename  string            `json:"filename"`
	SHA256    string            `json:"sha256"`
	Month     string            `json:"month"`
	Sheets    []ImportSheet     `json:"sheets"`
	Employees []ImportEmployee  `json:"employees"`
	Rules     []ImportRuleDraft `json:"rules"`
	Issues    []ImportIssue     `json:"issues"`
}

type ImportIssue struct {
	Code     string   `json:"code"`
	Severity string   `json:"severity"`
	Sheet    string   `json:"sheet"`
	Cell     string   `json:"cell"`
	Message  string   `json:"message"`
	Related  []string `json:"related,omitempty"`
}

// Value is Excel's formatted, cached display value; RawValue is its stored value.
// Formula is retained without execution. MergedFrom identifies a resolved
// source anchor, while Cell always remains the requested coordinate.
type ImportCell struct {
	Sheet      string `json:"sheet"`
	Cell       string `json:"cell"`
	Value      string `json:"value"`
	RawValue   string `json:"raw_value"`
	Formula    string `json:"formula,omitempty"`
	Comment    string `json:"comment,omitempty"`
	MergedFrom string `json:"merged_from,omitempty"`
}

type ImportSheet struct {
	Name       string       `json:"name"`
	LastRow    int          `json:"last_row"`
	LastColumn int          `json:"last_column"`
	Merges     []string     `json:"merges"`
	Cells      []ImportCell `json:"cells"`
}

type ImportDay struct {
	Date  string       `json:"date"`
	Cells []ImportCell `json:"cells"`
}

type ImportEmployee struct {
	SourceKey  string                `json:"source_key"`
	Sheet      string                `json:"sheet"`
	Row        int                   `json:"row"`
	Name       string                `json:"name"`
	EmployeeNo string                `json:"employee_no"`
	Department string                `json:"department"`
	Month      string                `json:"month"`
	Days       []ImportDay           `json:"days"`
	Summary    map[string]ImportCell `json:"summary"`
	Issues     []ImportIssue         `json:"issues"`
}

type ImportRuleDraft struct {
	SourceKey           string                `json:"source_key"`
	Sheet               string                `json:"sheet"`
	Row                 int                   `json:"row"`
	Department          string                `json:"department"`
	DirectoryDepartment string                `json:"directory_department,omitempty"`
	Category            string                `json:"category,omitempty"`
	NamesText           string                `json:"names_text"`
	Group               string                `json:"group"`
	ScheduleText        string                `json:"schedule_text"`
	OvertimeSchedule    string                `json:"overtime_schedule,omitempty"`
	SaturdaySchedule    string                `json:"saturday_schedule,omitempty"`
	RestdayText         string                `json:"restday_text,omitempty"`
	CheckinText         string                `json:"checkin_text"`
	Remarks             string                `json:"remarks"`
	Note                string                `json:"note"`
	Status              string                `json:"status"`
	OvertimeUnitMinutes int                   `json:"overtime_unit_minutes"`
	Fields              map[string]ImportCell `json:"fields"`
	Issues              []ImportIssue         `json:"issues"`
}

type attendanceSheetSource struct {
	sheet  ImportSheet
	cells  map[string]ImportCell
	merged map[string]string
}

func (s attendanceSheetSource) cell(column, row int, resolve bool) ImportCell {
	address, _ := excelize.CoordinatesToCellName(column, row)
	anchor := address
	if resolve && s.merged[address] != "" {
		anchor = s.merged[address]
	}
	c := s.cells[anchor]
	c.Sheet, c.Cell = s.sheet.Name, address
	if anchor != address {
		c.MergedFrom = anchor
	}
	return c
}

var attendanceMonthPattern = regexp.MustCompile(`(20\d{2})年\s*(\d{1,2})月`)
var attendanceRuleTimePattern = regexp.MustCompile(`(\d{1,2})\s*:\s*(\d{2})\s*-\s*(\d{1,2})\s*:\s*(\d{2})`)
var attendanceRuleHoursPattern = regexp.MustCompile(`共\s*(\d+(?:\.\d+)?)\s*个?小时`)
var attendanceOvertimeTypoPattern = regexp.MustCompile(`不满\s*(\d+)\s*分钟`)

// ParseAttendanceWorkbook reads either the approved monthly result workbook or
// the department schedule workbook. It never writes the source, accesses the
// database, invokes external links, or evaluates workbook formulas.
func ParseAttendanceWorkbook(reader io.Reader, filename string) (ImportPreview, error) {
	out := ImportPreview{Filename: filepath.Base(strings.ReplaceAll(filename, "\\", "/")), Sheets: []ImportSheet{}, Employees: []ImportEmployee{}, Rules: []ImportRuleDraft{}, Issues: []ImportIssue{}}
	if reader == nil || !strings.EqualFold(filepath.Ext(out.Filename), ".xlsx") {
		return out, errors.New("请选择有效的 .xlsx 考勤工作簿")
	}
	const maxBytes = 20 << 20
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return out, errors.New("无法读取考勤工作簿")
	}
	if len(data) > maxBytes {
		return out, errors.New("考勤工作簿不能超过 20 MiB")
	}
	digest := sha256.Sum256(data)
	out.SHA256 = hex.EncodeToString(digest[:])
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: 64 << 20, UnzipXMLSizeLimit: 64 << 20})
	if err != nil {
		return out, errors.New("无法打开工作簿，请确认文件未损坏且未加密")
	}
	defer f.Close()
	names := f.GetSheetList()
	if len(names) == 0 || len(names) > 12 {
		return out, errors.New("工作簿工作表数量必须为 1 至 12 张")
	}
	for _, name := range names {
		s, err := readAttendanceSheet(f, name)
		if err != nil {
			return out, err
		}
		out.Sheets = append(out.Sheets, s.sheet)
		for _, cell := range s.sheet.Cells {
			if cell.Formula != "" && cell.RawValue == "" {
				out.Issues = append(out.Issues, ImportIssue{Code: "missing_formula_cache", Severity: "warning", Sheet: name, Cell: cell.Cell, Message: "公式没有已保存的计算结果，保留公式但不执行或猜测；需从原表补充核定值"})
			}
		}
		title := s.cell(1, 1, true).Value
		kind := ""
		switch {
		case strings.Contains(title, "考勤签收表"):
			kind = "baseline"
		case strings.Contains(title, "上班时间表"):
			kind = "rules"
		default:
			return out, fmt.Errorf("工作表 %s 缺少支持的考勤签收表或上班时间表表头", name)
		}
		if out.Kind != "" && out.Kind != kind {
			return out, errors.New("月度基准和上班规则须分别导入，不能混在同一个工作簿")
		}
		out.Kind = kind
		if kind == "baseline" {
			if err = parseAttendanceBaseline(&out, s); err != nil {
				return out, err
			}
		} else {
			parseAttendanceRuleDrafts(&out, s)
		}
	}
	if out.Kind == "baseline" {
		flagAttendanceIdentities(&out)
		if len(out.Employees) == 0 {
			return out, errors.New("月度工作簿未发现人员记录")
		}
	} else {
		flagAttendanceRuleConflicts(&out)
		if len(out.Rules) == 0 {
			return out, errors.New("上班时间工作簿未发现规则记录")
		}
	}
	return out, nil
}

func readAttendanceSheet(f *excelize.File, name string) (attendanceSheetSource, error) {
	out := attendanceSheetSource{sheet: ImportSheet{Name: name, Merges: []string{}, Cells: []ImportCell{}}, cells: map[string]ImportCell{}, merged: map[string]string{}}
	dimension, err := f.GetSheetDimension(name)
	if err != nil {
		return out, errors.New("无法读取工作表范围")
	}
	end := strings.Split(dimension, ":")
	cols, rows, err := excelize.CellNameToCoordinates(end[len(end)-1])
	if err != nil || rows > 10000 || cols > 128 || rows*cols > 200000 {
		return out, fmt.Errorf("工作表 %s 超出支持范围（最多 10000 行、128 列、20 万单元格）", name)
	}
	merges, err := f.GetMergeCells(name, true)
	if err != nil {
		return out, errors.New("无法读取合并单元格")
	}
	for _, merge := range merges {
		start, end := merge.GetStartAxis(), merge.GetEndAxis()
		x1, y1, e1 := excelize.CellNameToCoordinates(start)
		x2, y2, e2 := excelize.CellNameToCoordinates(end)
		if e1 != nil || e2 != nil || x2 > 128 || y2 > 10000 || (x2-x1+1)*(y2-y1+1) > 200000 {
			return out, errors.New("合并单元格范围无效")
		}
		out.sheet.Merges = append(out.sheet.Merges, start+":"+end)
		for r := y1; r <= y2; r++ {
			for c := x1; c <= x2; c++ {
				a, _ := excelize.CoordinatesToCellName(c, r)
				if a != start {
					out.merged[a] = start
				}
			}
		}
	}
	formatted, err := f.GetRows(name)
	if err != nil {
		return out, errors.New("无法读取单元格显示值")
	}
	raw, err := f.GetRows(name, excelize.Options{RawCellValue: true})
	if err != nil {
		return out, errors.New("无法读取单元格原始值")
	}
	comments, err := f.GetComments(name)
	if err != nil {
		return out, errors.New("无法读取工作表批注")
	}
	commentMap := map[string]string{}
	for _, c := range comments {
		text := c.Text
		for _, run := range c.Paragraph {
			text += run.Text
		}
		commentMap[c.Cell] = text
	}
	rows = max(rows, len(raw), len(formatted))
	for _, row := range raw {
		cols = max(cols, len(row))
	}
	if rows > 10000 || cols > 128 || rows*cols > 200000 {
		return out, errors.New("实际单元格数量超出支持范围")
	}
	valueAt := func(values [][]string, r, c int) string {
		if r <= len(values) && c <= len(values[r-1]) {
			return values[r-1][c-1]
		}
		return ""
	}
	for r := 1; r <= rows; r++ {
		for c := 1; c <= cols; c++ {
			address, _ := excelize.CoordinatesToCellName(c, r)
			cell := ImportCell{Sheet: name, Cell: address, Value: valueAt(formatted, r, c), RawValue: valueAt(raw, r, c), Comment: commentMap[address]}
			// Asking Excelize for a merged child's formula resolves the anchor.
			// Only retain actual anchor formulas in the source cell collection.
			if out.merged[address] == "" {
				cell.Formula, err = f.GetCellFormula(name, address)
				if err != nil {
					return out, fmt.Errorf("无法读取 %s!%s 的公式", name, address)
				}
			}
			if cell.Value == "" && cell.RawValue == "" && cell.Formula == "" && cell.Comment == "" {
				continue
			}
			out.cells[address] = cell
			out.sheet.Cells = append(out.sheet.Cells, cell)
			out.sheet.LastRow, out.sheet.LastColumn = max(out.sheet.LastRow, r), max(out.sheet.LastColumn, c)
		}
	}
	return out, nil
}

func parseAttendanceBaseline(out *ImportPreview, s attendanceSheetSource) error {
	match := attendanceMonthPattern.FindStringSubmatch(s.cell(1, 1, true).Value)
	if len(match) != 3 {
		return fmt.Errorf("%s!A1 必须明确记录年份和月份，不能按文件名猜测", s.sheet.Name)
	}
	year, _ := strconv.Atoi(match[1])
	month, _ := strconv.Atoi(match[2])
	if month < 1 || month > 12 {
		return errors.New("考勤月份无效")
	}
	period := fmt.Sprintf("%04d-%02d", year, month)
	if out.Month != "" && out.Month != period {
		return errors.New("同一月度基准工作簿存在不同月份")
	}
	out.Month = period
	dayStart := 0
	for col := 1; col <= s.sheet.LastColumn; col++ {
		if s.cell(col, 2, false).RawValue == "1" {
			dayStart = col
			break
		}
	}
	if dayStart < 5 || s.cell(2, 2, true).Value != "姓名" || s.cell(3, 2, true).Value != "工号" {
		return fmt.Errorf("%s 的姓名、工号或日期表头不完整", s.sheet.Name)
	}
	daysInMonth := time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
	dayColumns := 0
	for day := 1; day <= 31; day++ {
		if s.cell(dayStart+day-1, 2, false).RawValue != strconv.Itoa(day) {
			break
		}
		dayColumns++
	}
	if dayColumns < daysInMonth {
		return fmt.Errorf("%s 的日期列不足以覆盖 %s", s.sheet.Name, period)
	}
	for row := 4; row <= s.sheet.LastRow; row++ {
		name := strings.TrimSpace(s.cell(2, row, false).Value)
		if name == "" || s.merged[fmt.Sprintf("B%d", row)] != "" {
			continue
		}
		// Real roster rows have a sequence cell; notes and footer calculations do not.
		if s.cell(1, row, false).Value == "" && s.cell(1, row, false).Formula == "" {
			continue
		}
		twoRows := s.merged[fmt.Sprintf("B%d", row+1)] == fmt.Sprintf("B%d", row)
		e := ImportEmployee{SourceKey: fmt.Sprintf("%s!B%d", s.sheet.Name, row), Sheet: s.sheet.Name, Row: row, Name: name,
			EmployeeNo: strings.TrimSpace(s.cell(3, row, true).Value), Department: strings.TrimSpace(s.cell(4, row, true).Value), Month: period,
			Days: []ImportDay{}, Summary: map[string]ImportCell{}, Issues: []ImportIssue{}}
		// These monthly templates put a schedule under the department name.
		// The complete original text stays in Sheets.Cells for review.
		e.Department = strings.TrimSpace(strings.Split(e.Department, "\n")[0])
		for day := 1; day <= daysInMonth; day++ {
			d := ImportDay{Date: fmt.Sprintf("%s-%02d", period, day), Cells: []ImportCell{s.cell(dayStart+day-1, row, true)}}
			if twoRows {
				d.Cells = append(d.Cells, s.cell(dayStart+day-1, row+1, true))
			}
			e.Days = append(e.Days, d)
		}
		for col := dayStart + dayColumns; col <= s.sheet.LastColumn; col++ {
			letter, _ := excelize.ColumnNumberToName(col)
			e.Summary[letter] = s.cell(col, row, true)
		}
		if e.EmployeeNo == "" {
			e.Issues = append(e.Issues, ImportIssue{Code: "missing_employee_no", Severity: "error", Sheet: e.Sheet, Cell: fmt.Sprintf("C%d", row), Message: "工号缺失，需人工关联员工；不会按姓名自动创建或合并"})
		}
		if e.Department == "" {
			e.Issues = append(e.Issues, ImportIssue{Code: "missing_department", Severity: "warning", Sheet: e.Sheet, Cell: fmt.Sprintf("D%d", row), Message: "源表部门为空，需人工核对，不继承非合并的上一行部门"})
		}
		out.Employees = append(out.Employees, e)
	}
	return nil
}

func flagAttendanceIdentities(out *ImportPreview) {
	numbers, names := map[string][]int{}, map[string][]int{}
	for i, e := range out.Employees {
		if e.EmployeeNo != "" {
			numbers[e.EmployeeNo] = append(numbers[e.EmployeeNo], i)
		}
		names[e.Name] = append(names[e.Name], i)
	}
	for i := range out.Employees {
		e := &out.Employees[i]
		for _, check := range []struct {
			indices                         []int
			code, severity, column, message string
		}{
			{numbers[e.EmployeeNo], "duplicate_employee_no", "error", "C", "工号在基准中重复，需逐条人工确认，禁止自动覆盖或合并"},
			{names[e.Name], "duplicate_employee_name", "warning", "B", "存在同名员工，必须结合工号和部门精确关联"},
		} {
			if len(check.indices) < 2 {
				continue
			}
			issue := ImportIssue{Code: check.code, Severity: check.severity, Sheet: e.Sheet, Cell: fmt.Sprintf("%s%d", check.column, e.Row), Message: check.message, Related: []string{}}
			for _, index := range check.indices {
				other := out.Employees[index]
				issue.Related = append(issue.Related, fmt.Sprintf("%s!%s%d", other.Sheet, check.column, other.Row))
			}
			e.Issues = append(e.Issues, issue)
		}
		out.Issues = append(out.Issues, e.Issues...)
	}
}

func parseAttendanceRuleDrafts(out *ImportPreview, s attendanceSheetSource) {
	headers := map[string]int{}
	for col := 1; col <= s.sheet.LastColumn; col++ {
		headers[strings.Join(strings.Fields(s.cell(col, 2, true).Value), "")] = col
	}
	for row := 3; row <= s.sheet.LastRow; row++ {
		get := func(header string) ImportCell {
			if col := headers[strings.Join(strings.Fields(header), "")]; col > 0 {
				return s.cell(col, row, true)
			}
			return ImportCell{}
		}
		schedule, dept, names, group := get("工作起止时间段"), get("部门"), get("姓名"), get("组别")
		category := "职员"
		if strings.HasPrefix(s.sheet.Name, "员工") {
			category = "员工"
		}
		if strings.HasPrefix(s.sheet.Name, "生产职员") {
			category = "生产职员"
		}
		if headers["工作日周一到周五【正班】"] > 0 {
			schedule = get("工作日周一到周五【正班】")
			names = group
			group = ImportCell{Value: category}
		}
		if strings.TrimSpace(schedule.Value) == "" && strings.TrimSpace(dept.Value) == "" && strings.TrimSpace(names.Value) == "" && strings.TrimSpace(group.Value) == "" {
			continue
		}
		draft := ImportRuleDraft{SourceKey: fmt.Sprintf("%s!A%d", s.sheet.Name, row), Sheet: s.sheet.Name, Row: row,
			Department: strings.TrimSpace(dept.Value), NamesText: strings.TrimSpace(names.Value), Group: strings.TrimSpace(group.Value),
			DirectoryDepartment: strings.TrimSpace(get("企业微信部门").Value), Category: category,
			ScheduleText: schedule.Value, CheckinText: get("考勤打卡要求").Value, Remarks: get("备注").Value, Note: get("注").Value,
			OvertimeSchedule: get("工作日周一到周五【加班】").Value, SaturdaySchedule: get("周六【单休班】").Value,
			Status: "draft", OvertimeUnitMinutes: 30, Fields: map[string]ImportCell{}, Issues: []ImportIssue{}}
		if category == "员工" {
			draft.RestdayText = get("周六、周日").Value
		}
		if category == "生产职员" {
			draft.RestdayText = get("周日【休息】").Value
		}
		for col := 1; col <= s.sheet.LastColumn; col++ {
			letter, _ := excelize.ColumnNumberToName(col)
			draft.Fields[letter] = s.cell(col, row, true)
		}
		add := func(code, severity, cell, message string) {
			draft.Issues = append(draft.Issues, ImportIssue{Code: code, Severity: severity, Sheet: draft.Sheet, Cell: cell, Message: message})
		}
		// The user's confirmed correction is separate from the immutable cell
		// evidence retained in Fields. Re-importing an older sheet cannot undo it.
		if category == "员工" && draft.Department == "品质部" && strings.Contains(draft.NamesText, "邢玉琴") && strings.Contains(draft.NamesText, "杨玛玲") {
			draft.ScheduleText = "8:00-11:40 13:10-17:30"
			draft.OvertimeSchedule = "8:00-11:40 13:10-17:15 17:45-结束"
			add("confirmed_quality_hours", "info", schedule.Cell, "2026-11-01人事确认：邢玉琴、杨玛玲无晚间加班17:30下班；有晚间加班17:15结束下午班、17:45返岗，源单元格原文保留")
		}
		add("rule_approval_required", "warning", fmt.Sprintf("A%d", row), "草稿未生效：需确认本地部门、具体人员或组别、有效起止日期及排班日历，不能按相似名称自动套用")
		if draft.Department == "" {
			add("missing_department", "error", fmt.Sprintf("A%d", row), "部门缺失，不能继承非合并的上一行部门")
		}
		if strings.TrimSpace(schedule.Value) == "" {
			add("missing_schedule", "error", schedule.Cell, "工作时段缺失，需人工确认")
		}
		if strings.Contains(schedule.Value+draft.OvertimeSchedule, "结束") {
			add("open_overtime_end", "warning", schedule.Cell, "源表加班结束时间未固定，需明确打卡配对、跨日归属和最长时长")
		}
		if strings.Contains(draft.Remarks, "大小周") || strings.Contains(draft.Remarks, "AB班") || strings.Contains(draft.Remarks, "倒班") || strings.Contains(schedule.Value, "夜班") || strings.Contains(schedule.Value, "晚班") {
			add("roster_anchor_required", "warning", schedule.Cell, "大小周或轮班须指定排班起点及适用日期，不从当前日期猜测")
		}
		if match := attendanceOvertimeTypoPattern.FindStringSubmatch(draft.Remarks); len(match) == 2 && match[1] != "30" {
			add("confirmed_overtime_unit", "info", get("备注").Cell, "源表递增分钟数已由用户确认为误填，草稿统一满30分钟计0.5小时，不足部分不计；保留源单元格原文")
		}
		normalized := strings.NewReplacer("：", ":", "－", "-", "—", "-").Replace(schedule.Value)
		segments := attendanceRuleTimePattern.FindAllStringSubmatch(normalized, -1)
		if hours := attendanceRuleHoursPattern.FindStringSubmatch(draft.Note); len(hours) == 2 && len(segments) > 0 {
			expected, _ := strconv.ParseFloat(hours[1], 64)
			total := 0
			for _, segment := range segments {
				h1, _ := strconv.Atoi(segment[1])
				m1, _ := strconv.Atoi(segment[2])
				h2, _ := strconv.Atoi(segment[3])
				m2, _ := strconv.Atoi(segment[4])
				minutes := h2*60 + m2 - h1*60 - m1
				if minutes <= 0 {
					minutes += 24 * 60
				}
				total += minutes
			}
			if total != int(expected*60) {
				add("hours_definition_requires_review", "warning", schedule.Cell, "列出时段的合计与备注正班工时不同，需确认正班、加班、休息或多个班次的边界；不自动补足或扣减")
			}
		}
		out.Rules = append(out.Rules, draft)
	}
}

func flagAttendanceRuleConflicts(out *ImportPreview) {
	groups := map[string][]int{}
	for i, rule := range out.Rules {
		key := strings.Join([]string{rule.Sheet, rule.Department, rule.NamesText, rule.Group}, "\x00")
		groups[key] = append(groups[key], i)
	}
	for i := range out.Rules {
		rule := &out.Rules[i]
		key := strings.Join([]string{rule.Sheet, rule.Department, rule.NamesText, rule.Group}, "\x00")
		for _, other := range groups[key] {
			if other == i || strings.TrimSpace(out.Rules[other].ScheduleText) == strings.TrimSpace(rule.ScheduleText) {
				continue
			}
			if (strings.Contains(rule.ScheduleText, "白班") && strings.Contains(out.Rules[other].ScheduleText, "夜班")) || (strings.Contains(rule.ScheduleText, "夜班") && strings.Contains(out.Rules[other].ScheduleText, "白班")) {
				continue // Separate labelled shifts still require a roster anchor.
			}
			rule.Issues = append(rule.Issues, ImportIssue{Code: "conflicting_rule_targets", Severity: "error", Sheet: rule.Sheet, Cell: fmt.Sprintf("A%d", rule.Row), Message: "同一源表部门及适用对象存在不同时间段，必须人工区分适用人员或生效时间", Related: []string{rule.SourceKey, out.Rules[other].SourceKey}})
		}
		out.Issues = append(out.Issues, rule.Issues...)
	}
	// Stable output makes a reviewed preview comparable to the same upload.
	sort.SliceStable(out.Issues, func(i, j int) bool {
		a, b := out.Issues[i], out.Issues[j]
		return a.Sheet+"\x00"+a.Cell+"\x00"+a.Code < b.Sheet+"\x00"+b.Cell+"\x00"+b.Code
	})
}
