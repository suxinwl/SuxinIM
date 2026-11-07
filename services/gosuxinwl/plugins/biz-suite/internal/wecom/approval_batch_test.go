package wecom

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func TestValidateSingleApplicantBatch(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  json.RawMessage
		ok   bool
	}{
		{"omitted", nil, true},
		{"null", json.RawMessage(" \n null\t"), true},
		{"empty array", json.RawMessage("[]"), true},
		{"empty array with whitespace", json.RawMessage(" \n[ \t ] \r\n"), true},
		{"one batch applicant", json.RawMessage(`[{"userid":"alice"}]`), false},
		{"multiple batch applicants", json.RawMessage(`[{"userid":"alice"},{"userid":"bob"}]`), false},
		{"null member", json.RawMessage(`[null]`), false},
		{"object", json.RawMessage(`{}`), false},
		{"string", json.RawMessage(`"[]"`), false},
		{"boolean", json.RawMessage(`false`), false},
		{"number", json.RawMessage(`0`), false},
		{"truncated array", json.RawMessage(`[`), false},
		{"multiple values", json.RawMessage(`[] []`), false},
		{"only whitespace", json.RawMessage(" \n\t"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateSingleApplicantBatch(tc.raw); (err == nil) != tc.ok {
				t.Fatalf("expected valid=%t, got %v", tc.ok, err)
			}
		})
	}
}

func TestSingleApplicantEmptyBatchApprovalLifecycle(t *testing.T) {
	for _, kind := range []string{"leave", "trip", "out"} {
		t.Run(kind, func(t *testing.T) {
			ctx := setupDB(t)
			employee := seedEmployee(t, ctx, "alice")
			template := Template{TemplateID: "template-" + kind, ApprovalType: kind, Enabled: 1, Mapping: Mapping{AttendanceControlID: "vacation-control", ReasonControlID: "reason-control"}}
			if err := SaveTemplate(ctx, template); err != nil {
				t.Fatal(err)
			}
			fixture := approvalFixture("EMPTY-BATCH-"+kind, template.TemplateID, 2)
			if kind != "leave" {
				attendanceType := 3
				if kind == "out" {
					attendanceType = 4
				}
				fixture["apply_data"].(g.Map)["contents"].([]any)[0] = g.Map{"id": "vacation-control", "control": "Attendance", "value": g.Map{"attendance": g.Map{"type": attendanceType, "date_range": g.Map{"type": "hour", "new_begin": 1700000000, "new_end": 1700007200, "new_duration": 7200}}}}
			}
			fixture["batch_applyer"] = []any{}
			var detail approvalDetail
			if err := json.Unmarshal([]byte(mustJSON(fixture)), &detail); err != nil {
				t.Fatal(err)
			}
			table := "biz_hrm_attendance_" + kind
			assertState := func(want string) {
				t.Helper()
				row, err := core.Model(table).Ctx(ctx).One()
				if err != nil || len(row) == 0 || row["employee_id"].String() != employee || row["approval_status"].String() != want || row["duration"].Float64() != 2 || countRows(t, ctx, table) != 1 {
					t.Fatalf("expected one %s record with status %s: %v %v", kind, want, row, err)
				}
			}
			for _, raw := range []json.RawMessage{nil, json.RawMessage("null"), json.RawMessage("[]"), json.RawMessage(" \n[ \t ] \r\n")} {
				detail.BatchApplyer = raw
				if err := saveApproval(ctx, detail, mustJSON(detail)); err != nil {
					t.Fatalf("single applicant with empty batch must normalize: %v", err)
				}
				assertState("approved")
			}
			// A later unsupported batch must withdraw the earlier effective result.
			detail.BatchApplyer = json.RawMessage(`[{"userid":"alice"},{"userid":"bob"}]`)
			var partial *PartialError
			if err := saveApproval(ctx, detail, mustJSON(detail)); !errors.As(err, &partial) {
				t.Fatalf("unsupported batch should be retained as an exception: %v", err)
			}
			assertState("pending")
			row, err := core.Model("wecom_approval_raw").Ctx(ctx).One()
			if err != nil || row["processing_error"].String() == "" {
				t.Fatalf("batch exception was not retained: %v %v", row, err)
			}
			// A withdrawal still cancels prior attendance even without usable controls.
			detail.Status = 4
			detail.ApplyData.Contents = nil
			if err := saveApproval(ctx, detail, mustJSON(detail)); err != nil {
				t.Fatal(err)
			}
			assertState("canceled")
		})
	}
}

func TestInvalidBatchShapesRemainRawExceptions(t *testing.T) {
	ctx := setupDB(t)
	seedEmployee(t, ctx, "alice")
	installTemplate(t, ctx)
	for i, value := range []any{g.Map{}, "", "[]", true, 0, []any{nil}, []any{g.Map{"userid": "alice"}}} {
		fixture := approvalFixture(fmt.Sprintf("INVALID-BATCH-%d", i), "leave-tpl", 2)
		fixture["batch_applyer"] = value
		var detail approvalDetail
		if err := json.Unmarshal([]byte(mustJSON(fixture)), &detail); err != nil {
			t.Fatal(err)
		}
		var partial *PartialError
		if err := saveApproval(ctx, detail, mustJSON(fixture)); !errors.As(err, &partial) {
			t.Fatalf("batch shape %T should remain an exception: %v", value, err)
		}
		row, err := core.Model("wecom_approval_raw").Ctx(ctx).Where("sp_no", detail.SPNo).One()
		if err != nil || row["processing_error"].String() == "" || row["raw_json"].String() != mustJSON(fixture) {
			t.Fatalf("raw exception missing: %v %v", row, err)
		}
	}
	if countRows(t, ctx, "biz_hrm_attendance_leave") != 0 {
		t.Fatal("unsupported batch shape normalized to a guessed single applicant")
	}
}
