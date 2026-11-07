package hrm

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func attendanceWorkbookFixture(t *testing.T, name string) []byte {
	t.Helper()
	_, source, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(source), "..", "..", "..", "..", "docs", name)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skip("用户核定源工作簿未包含在当前检出中：" + name)
	}
	if err != nil {
		t.Fatal(err)
	}
	before := sha256.Sum256(data)
	t.Cleanup(func() {
		after, err := os.ReadFile(path)
		if err != nil || sha256.Sum256(after) != before {
			t.Fatal("解析修改了源工作簿", err)
		}
	})
	return data
}

func TestAttendanceImportApprovedAugustWorkbook(t *testing.T) {
	data := attendanceWorkbookFixture(t, "8月考勤数据.xlsx")
	p, err := ParseAttendanceWorkbook(bytes.NewReader(data), "8月考勤数据.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "baseline" || p.Month != "2026-08" || len(p.Employees) != 330 || len(p.Sheets) != 3 {
		t.Fatalf("unexpected roster: kind=%s month=%s employees=%d sheets=%d", p.Kind, p.Month, len(p.Employees), len(p.Sheets))
	}
	counts, issues := map[string]int{}, map[string]int{}
	var wang []string
	for _, e := range p.Employees {
		counts[e.Sheet]++
		if len(e.Days) != 31 || e.Days[0].Date != "2026-08-01" || e.Days[30].Date != "2026-08-31" {
			t.Fatalf("missing date cells: %+v", e)
		}
		if e.Name == "王芳" {
			wang = append(wang, e.EmployeeNo)
		}
		if e.SourceKey == "8月员工!B4" {
			if e.EmployeeNo != "0421" || len(e.Days[0].Cells) != 2 || e.Summary["AN"].Value != "80.5" || !strings.Contains(e.Summary["AN"].Formula, "SUM(H5:L5") {
				t.Fatalf("source evidence lost: %+v", e)
			}
		}
		if e.SourceKey == "8月员工!B6" && e.Department != "制造一部" {
			t.Fatal("merged department not resolved")
		}
		if e.SourceKey == "8月职员!B43" && (e.EmployeeNo != "7319" || e.Department != "市场部" || len(e.Days[0].Cells) != 1) {
			t.Fatal("staff identity or daily layout lost")
		}
	}
	if counts["8月员工"] != 166 || counts["8月生产职员"] != 15 || counts["8月职员"] != 149 || strings.Join(wang, ",") != "1778,3240" {
		t.Fatalf("wrong sheet/name mapping: %v %v", counts, wang)
	}
	for _, issue := range p.Issues {
		issues[issue.Code]++
	}
	if issues["missing_employee_no"] != 5 || issues["duplicate_employee_no"] != 4 || issues["duplicate_employee_name"] != 2 {
		t.Fatalf("identity conflicts not retained: %v", issues)
	}
	comments, formulas := 0, 0
	foundNote := false
	for _, sheet := range p.Sheets {
		for _, c := range sheet.Cells {
			if c.Comment != "" {
				comments++
			}
			if c.Formula != "" {
				formulas++
			}
			if sheet.Name == "8月职员" && c.Cell == "B119" {
				foundNote = strings.Contains(c.Comment, "一天打两次卡")
			}
		}
	}
	if comments != 476 || formulas != 666 || !foundNote {
		t.Fatalf("source evidence incomplete comments=%d formulas=%d note=%t", comments, formulas, foundNote)
	}
}

func TestAttendanceImportDepartmentWorkbookDrafts(t *testing.T) {
	p, err := ParseAttendanceWorkbook(bytes.NewReader(attendanceWorkbookFixture(t, "部门上班时间.xlsx")), "部门上班时间.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "rules" || len(p.Rules) != 49 || len(p.Employees) != 0 {
		t.Fatalf("wrong draft count: %s %d", p.Kind, len(p.Rules))
	}
	counts := map[string]int{}
	quality := false
	night := false
	for _, r := range p.Rules {
		counts[r.Category]++
		if r.Status != "draft" || r.OvertimeUnitMinutes != 30 {
			t.Fatal("imported a published rule or changed rounding")
		}
		if r.Category == "员工" && strings.Contains(r.NamesText, "邢玉琴") {
			quality = strings.Contains(r.ScheduleText, "17:30") && strings.Contains(r.OvertimeSchedule, "17:15") && strings.Contains(r.OvertimeSchedule, "17:45") && r.Fields["D"].Value != ""
		}
		if r.Category == "员工" && strings.Contains(r.ScheduleText, "夜班") && r.DirectoryDepartment == "分切事业部" {
			night = strings.Contains(r.OvertimeSchedule, "4:30") && strings.Contains(r.OvertimeSchedule, "8：00") && r.Fields["B"].MergedFrom == "B6"
		}
	}
	if counts["职员"] != 28 || counts["员工"] != 12 || counts["生产职员"] != 9 || !quality || !night {
		t.Fatal("new layout/provenance lost", counts, quality, night)
	}
}

func TestAttendanceImportApprovedSeptemberWorkbook(t *testing.T) {
	p, err := ParseAttendanceWorkbook(bytes.NewReader(attendanceWorkbookFixture(t, "9月考勤数据.xlsx")), "9月考勤数据.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if p.Month != "2026-09" || len(p.Employees) != 321 {
		t.Fatalf("unexpected September counts: %s %d", p.Month, len(p.Employees))
	}
	counts, issues := map[string]int{}, map[string]int{}
	for _, e := range p.Employees {
		counts[e.Sheet]++
		if len(e.Days) != 30 || e.Days[29].Date != "2026-09-30" {
			t.Fatalf("September day columns incorrect: %s", e.SourceKey)
		}
		if e.SourceKey == "9月员工!B4" && (e.EmployeeNo != "0421" || e.Summary["AK"].RawValue != "176" || !strings.HasPrefix(e.Summary["AM"].Formula, "SUM(")) {
			t.Fatalf("September source positions incorrect: %+v", e)
		}
	}
	for _, i := range p.Issues {
		issues[i.Code]++
	}
	if counts["9月员工"] != 159 || counts["9月生产职员 "] != 16 || counts["9月职员"] != 146 || issues["missing_employee_no"] != 5 || issues["duplicate_employee_no"] != 2 || issues["duplicate_employee_name"] != 2 {
		t.Fatalf("September identities not preserved: %v %v", counts, issues)
	}
	comments, formulas := 0, 0
	for _, sheet := range p.Sheets {
		for _, c := range sheet.Cells {
			if c.Comment != "" {
				comments++
			}
			if c.Formula != "" {
				formulas++
			}
		}
	}
	if comments != 450 || formulas != 1246 {
		t.Fatalf("September evidence lost: comments=%d formulas=%d", comments, formulas)
	}
}

func attendanceSyntheticWorkbook(t *testing.T, title string) *excelize.File {
	t.Helper()
	f := excelize.NewFile()
	for cell, value := range map[string]any{"A1": title, "A2": "序号", "B2": "姓名", "C2": "工号", "D2": "部门", "A4": 1, "B4": "同名", "C4": "0012", "D4": "研发部"} {
		if err := f.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	for d := 1; d <= 31; d++ {
		a, _ := excelize.CoordinatesToCellName(d+4, 2)
		if err := f.SetCellValue("Sheet1", a, d); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func TestAttendanceImportDoesNotInferIdentityDepartmentsOrFormulaResults(t *testing.T) {
	f := attendanceSyntheticWorkbook(t, "2026年8月考勤签收表")
	defer f.Close()
	for cell, value := range map[string]any{"A5": 2, "B5": "同名", "C5": "0013"} {
		if err := f.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.SetCellFormula("Sheet1", "AP4", "1+2"); err != nil {
		t.Fatal(err)
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	p, err := ParseAttendanceWorkbook(bytes.NewReader(buf.Bytes()), "test.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Employees) != 2 || p.Employees[0].EmployeeNo != "0012" || p.Employees[1].Department != "" {
		t.Fatal("guessed or merged identities")
	}
	cell := p.Employees[0].Summary["AP"]
	if cell.Formula != "1+2" || cell.RawValue != "" {
		t.Fatalf("formula was evaluated: %+v", cell)
	}
	codes := map[string]int{}
	for _, i := range p.Issues {
		codes[i.Code]++
	}
	if codes["duplicate_employee_name"] != 2 || codes["missing_department"] != 1 || codes["missing_formula_cache"] != 1 {
		t.Fatal(codes)
	}
}

func TestAttendanceImportRejectsUnsupportedAndAmbiguousPeriod(t *testing.T) {
	for _, name := range []string{"test.xls", "test.csv", "test.xlsx"} {
		if _, err := ParseAttendanceWorkbook(strings.NewReader("not a workbook"), name); err == nil {
			t.Fatal("accepted invalid workbook", name)
		}
	}
	f := attendanceSyntheticWorkbook(t, "8月考勤签收表")
	defer f.Close()
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ParseAttendanceWorkbook(bytes.NewReader(buf.Bytes()), "2026年8月.xlsx"); err == nil {
		t.Fatal("inferred year from filename")
	}
}
