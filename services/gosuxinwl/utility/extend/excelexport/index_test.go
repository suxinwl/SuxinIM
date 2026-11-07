package excelexport

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/xuri/excelize/v2"
)

func TestExportToExcelRoundTrip(t *testing.T) {
	columns := make([]interface{}, 28)
	for i := range columns {
		columns[i] = map[string]interface{}{"field": fmt.Sprintf("field%d", i)}
	}
	columns[0] = map[string]interface{}{"field": "name", "title": "中文名称"}
	columns[1] = map[string]interface{}{"field": "amount", "title": "金额"}
	columns[2] = map[string]interface{}{"field": "enabled"}
	columns[3] = map[string]interface{}{"field": "empty"}
	columns[26] = map[string]interface{}{"field": "code"}
	columns[27] = map[string]interface{}{"field": "text"}
	rows := gdb.Result{{
		"name":    gvar.New("生产部 O'Reilly\n下一行"),
		"amount":  gvar.New(12.5),
		"enabled": gvar.New(true),
		"empty":   gvar.New(nil),
		"code":    gvar.New("00123"),
		"text":    gvar.New("=SUM(A1:A2)"),
	}}
	data, err := ExportToExcel(context.Background(), &rows, columns, "", "部门记录")
	if err != nil {
		t.Fatal(err)
	}
	file, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("export is not a readable XLSX workbook: %v", err)
	}
	defer file.Close()
	for cell, want := range map[string]string{
		"A1": "中文名称", "A2": "生产部 O'Reilly\n下一行", "B2": "12.5",
		"C1": "enabled", "C2": "TRUE", "D2": "", "E2": "",
		"AA1": "code", "AA2": "00123", "AB2": "=SUM(A1:A2)",
	} {
		got, err := file.GetCellValue("Sheet1", cell)
		if err != nil || got != want {
			t.Errorf("%s = %q, want %q; error: %v", cell, got, want, err)
		}
	}
	if cellType, err := file.GetCellType("Sheet1", "B2"); err != nil || cellType == excelize.CellTypeSharedString {
		t.Errorf("numeric database value exported as text: type=%v error=%v", cellType, err)
	}
	if cellType, err := file.GetCellType("Sheet1", "C2"); err != nil || cellType != excelize.CellTypeBool {
		t.Errorf("boolean database value lost its type: type=%v error=%v", cellType, err)
	}
	if formula, err := file.GetCellFormula("Sheet1", "AB2"); err != nil || formula != "" {
		t.Errorf("literal text became an Excel formula: %q error=%v", formula, err)
	}
}

func TestExportToExcelRejectsEmptyRowsAndInvalidColumns(t *testing.T) {
	rows := gdb.Result{{"name": gvar.New("test")}}
	empty := gdb.Result{}
	for _, tc := range []struct {
		name    string
		rows    *gdb.Result
		columns []interface{}
	}{
		{"nil rows", nil, nil},
		{"empty rows", &empty, nil},
		{"invalid column", &rows, []interface{}{"name"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ExportToExcel(context.Background(), tc.rows, tc.columns, "", ""); err == nil {
				t.Fatal("expected export error")
			}
		})
	}
}
