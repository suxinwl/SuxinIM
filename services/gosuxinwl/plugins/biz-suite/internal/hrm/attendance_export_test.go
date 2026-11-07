package hrm

import (
	"bytes"
	"net/url"
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/xuri/excelize/v2"
)

func attendanceExportRecord(data g.Map) gdb.Record {
	row := gdb.Record{}
	for key, value := range data {
		row[key] = gvar.New(value)
	}
	return row
}

func TestAttendanceExportTrialRetainsUnconfiguredStateAndSnapshotVersion(t *testing.T) {
	rows := gdb.Result{attendanceExportRecord(g.Map{
		"employee_json": attJSON(AttEmployee{ID: "staff", Name: "测试员工", EmployeeNo: "0032", Category: "员工"}),
		"rule_id":       "immutable-rule-v3",
		"result_json":   attJSON(eng.Result{Date: "2026-10-01", State: "configuration_pending", RuleVersion: "3", ShiftVersion: "3", EngineVersion: eng.Version}),
	})}
	f, err := ExportAttendanceResults(rows, "2026-10-01", "2026-10-01")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	title, _ := f.GetCellValue("员工", "A1")
	if !strings.Contains(title, "试算") || strings.Contains(title, "核定签收表") {
		t.Fatalf("unpublished result impersonates final report: %s", title)
	}
	day, _ := f.GetCellValue("员工", "D3")
	if day != "待配置" {
		t.Fatalf("unconfigured day exported as numeric/normal attendance: %q", day)
	}
	for cell, want := range map[string]string{"B9": "0032", "D9": "immutable-rule-v3", "E9": "3", "F9": "3", "G9": eng.Version, "H9": "待配置"} {
		got, err := f.GetCellValue("结果与版本依据", cell)
		if err != nil || got != want {
			t.Fatalf("snapshot %s: got %q want %q err=%v", cell, got, want, err)
		}
	}
	rows[0]["result_json"] = gvar.New("{malformed")
	if bad, err := ExportAttendanceResults(rows, "2026-10-01", "2026-10-01"); err == nil {
		bad.Close()
		t.Fatal("corrupt result silently exported as zero hours")
	}
}

func TestAttendanceExportHTTPPublicationAndReopenAreExplicit(t *testing.T) {
	ctx := setup(t)
	c := newAttWorkflowClient(t, ctx)
	batchID := "export-completed"
	if _, err := core.Model("biz_att_batch").Ctx(ctx).Data(g.Map{"id": batchID, "start_date": "2026-10-01", "end_date": "2026-10-31", "status": "completed", "request_json": "{}", "employee_count": 1, "total_count": 1, "completed_count": 1, "created_by": 1, "created_at": core.Now()}).Insert(); err != nil {
		t.Fatal(err)
	}
	if _, err := core.Model("biz_att_result").Ctx(ctx).Data(g.Map{"id": "export-result", "batch_id": batchID, "employee_id": "staff", "work_date": "2026-10-01", "rule_id": "frozen-rule", "status": "calculated", "employee_json": attJSON(AttEmployee{ID: "staff", Name: "版本核对", EmployeeNo: "0099", Category: "员工"}), "result_json": attJSON(eng.Result{Date: "2026-10-01", State: "calculated", RuleVersion: "2", ShiftVersion: "2", CreditedSeconds: 28800}), "input_json": "{}", "created_at": core.Now()}).Insert(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, status, linkedBatch string
		revision, changed         int
		formal                    bool
		stateText                 string
	}{
		{"completed-only", "", "", 0, 0, false, "非当前正式结果"},
		{"different-published-batch", "locked", "other-batch", 3, 0, false, "非当前正式结果"},
		{"reviewed-not-published", "reviewed", batchID, 4, 0, false, "已复核，未正式发布"},
		{"published", "published", batchID, 5, 0, true, "已发布，未锁定"},
		{"locked-with-new-evidence", "locked", batchID, 6, 1, true, "已月结锁定"},
		{"reopened", "draft", batchID, 7, 0, false, "月份已重开"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := core.Model("biz_att_month").Ctx(ctx).Where("month", "2026-10").Delete(); err != nil {
				t.Fatal(err)
			}
			if tc.status != "" {
				if _, err := core.Model("biz_att_month").Ctx(ctx).Data(g.Map{"month": "2026-10", "status": tc.status, "batch_id": tc.linkedBatch, "revision": tc.revision, "change_pending": tc.changed, "updated_by": 1, "updated_at": core.Now()}).Insert(); err != nil {
					t.Fatal(err)
				}
			}
			status, headers, raw := c.request(1, "GET", "export?batch_id="+batchID, nil)
			if status != 200 || !strings.Contains(headers.Get("Content-Type"), "spreadsheetml") {
				t.Fatalf("export failed: %d %s", status, raw)
			}
			f, err := excelize.OpenReader(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			title, _ := f.GetCellValue("员工", "A1")
			filename, _ := url.PathUnescape(headers.Get("Content-Disposition"))
			if !strings.Contains(title, tc.stateText) || strings.Contains(title, "正式核定签收表") != tc.formal || strings.Contains(filename, "正式核定签收表") != tc.formal {
				t.Fatalf("publication mislabeled: title=%s file=%s", title, filename)
			}
			if tc.linkedBatch == batchID {
				version, _ := f.GetCellValue("结果与版本依据", "B4")
				if !strings.Contains(version, "2026-10 / v") {
					t.Fatalf("missing month version: %s", version)
				}
			}
			if tc.changed != 0 {
				note, _ := f.GetCellValue("结果与版本依据", "B5")
				if !strings.Contains(note, "尚未反映后续变化") || !strings.Contains(title, "尚未更新") {
					t.Fatalf("closed result hides later changes: %s / %s", title, note)
				}
			}
		})
	}
}

func TestAttendanceExportPartialBatchCannotClaimFormalMonth(t *testing.T) {
	batch := attendanceExportRecord(g.Map{"id": "partial", "start_date": "2026-10-01", "end_date": "2026-10-02"})
	month := attendanceExportRecord(g.Map{"month": "2026-10", "batch_id": "partial", "status": "locked", "revision": 3})
	meta := attExportPublication(batch, month)
	if meta.Formal || meta.MonthRevision != 0 || !strings.Contains(meta.Status, "试算") {
		t.Fatalf("partial month claimed a finalized month: %+v", meta)
	}
}
