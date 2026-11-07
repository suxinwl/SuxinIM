package wecom

import (
	"encoding/json"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/approvalview"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func TestDecodeApprovalUsesSuppliedLeaveLabelInsteadOfOptionKey(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options []any
		want    string
	}{
		{"Chinese label", []any{g.Map{"key": "11", "value": []any{g.Map{"text": "Annual leave", "lang": "en"}, g.Map{"text": "年假", "lang": "zh_CN"}}}}, "年假"},
		{"other language label", []any{g.Map{"key": "11", "value": []any{g.Map{"text": "Special leave", "lang": "en"}}}}, "Special leave"},
		{"identifier only", []any{g.Map{"key": "11"}}, approvalview.UnknownLeaveType},
		{"missing selection", nil, approvalview.UnknownLeaveType},
		{"multiple selections", []any{g.Map{"key": "11", "value": []any{g.Map{"text": "事假", "lang": "zh_CN"}}}, g.Map{"key": "16", "value": []any{g.Map{"text": "年假", "lang": "zh_CN"}}}}, approvalview.UnknownLeaveType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := approvalFixture("LABEL-1", "leave-tpl", 2)
			contents := fixture["apply_data"].(g.Map)["contents"].([]any)
			vacation := contents[0].(g.Map)["value"].(g.Map)["vacation"].(g.Map)
			vacation["selector"] = g.Map{"options": tc.options}
			var detail approvalDetail
			if err := json.Unmarshal([]byte(mustJSON(fixture)), &detail); err != nil {
				t.Fatal(err)
			}
			got, err := decodeApproval(detail, Mapping{AttendanceControlID: "vacation-control", ReasonControlID: "reason-control"}, "leave")
			if err != nil || got.LeaveType != tc.want || got.Hours != 2 || got.Reason != "family" {
				t.Fatalf("unexpected normalized label or altered attendance: %#v %v", got, err)
			}
		})
	}
}

func TestApprovalLabelPersistsAndCancellationKeepsItsExistingBehavior(t *testing.T) {
	ctx := setupDB(t)
	seedEmployee(t, ctx, "alice")
	installTemplate(t, ctx)
	fixture := approvalFixture("LABEL-SAVE-1", "leave-tpl", 2)
	fixture["batch_applyer"] = []any{}
	var detail approvalDetail
	if err := json.Unmarshal([]byte(mustJSON(fixture)), &detail); err != nil {
		t.Fatal(err)
	}
	if err := saveApproval(ctx, detail, mustJSON(fixture)); err != nil {
		t.Fatal(err)
	}
	row, err := core.Model("biz_hrm_attendance_leave").Ctx(ctx).Where("external_ref", detail.SPNo).One()
	if err != nil || row["leave_type"].String() != "年假" || row["duration"].Float64() != 2 || row["approval_status"].String() != "approved" {
		t.Fatalf("label was not persisted with unchanged attendance: %v %v", row, err)
	}
	fixture["sp_status"] = 4
	detail.Status = 4
	if err := saveApproval(ctx, detail, mustJSON(fixture)); err != nil {
		t.Fatal(err)
	}
	row, err = core.Model("biz_hrm_attendance_leave").Ctx(ctx).Where("external_ref", detail.SPNo).One()
	if err != nil || row["leave_type"].String() != "年假" || row["approval_status"].String() != "canceled" {
		t.Fatalf("label update changed cancellation behavior: %v %v", row, err)
	}
}
