package hrm

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func TestAttendanceAdjustmentHTTPPreservesOptionalMinutesAndSeconds(t *testing.T) {
	ctx := setup(t)
	c := newAttWorkflowClient(t, ctx)
	employee, e := UpsertEmployee(ctx, Employee{Name: "接口核对员工", EmployeeNo: "HTTP001"})
	if e != nil {
		t.Fatal(e)
	}
	day, _ := attDate("2026-08-03")
	for _, tc := range []struct {
		name string
		data g.Map
	}{
		{"omitted_minutes", g.Map{"kind": "override", "confirm_issues": true}},
		{"zero_credit_not_actual", g.Map{"kind": "override", "credited_minutes": 0, "confirm_issues": false}},
		{"null_actual", g.Map{"kind": "override", "credited_minutes": 480, "actual_minutes": nil}},
		{"null_credit", g.Map{"kind": "override", "credited_minutes": nil, "actual_minutes": 90}},
		{"punch_seconds", g.Map{"kind": "punch", "at": day.Unix() + 9*3600 + 17, "punch_type": "in"}},
		{"credit_interval_seconds", g.Map{"kind": "credit", "start_at": day.Unix() + 9*3600, "end_at": day.Unix() + 10*3600, "segment_id": "work"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.data["employee_id"], tc.data["work_date"] = employee, "2026-08-03"
			tc.data["reason"], tc.data["evidence"] = "接口复核原因", "签字单与原始打卡记录"
			expectedJSON, _ := json.Marshal(tc.data)
			var expected AttAdjustmentRequest
			if e := json.Unmarshal(expectedJSON, &expected); e != nil {
				t.Fatal(e)
			}
			response := c.ok("POST", "adjustments", tc.data)
			expected.ID = response["id"].(string)
			check := func(label, raw string) {
				t.Helper()
				var got AttAdjustmentRequest
				if e := json.Unmarshal([]byte(raw), &got); e != nil || !reflect.DeepEqual(got, expected) {
					t.Fatalf("%s changed request fields: got=%+v expected=%+v error=%v", label, got, expected, e)
				}
			}
			check("response", attJSON(response))
			stored, e := core.Model("biz_att_adjustment").Ctx(ctx).Where("id", expected.ID).Value("adjustment_json")
			if e != nil {
				t.Fatal(e)
			}
			check("stored", stored.String())
			items := c.ok("GET", "adjustments?employee_id="+employee+"&work_date=2026-08-03", nil)["items"].([]any)
			found := false
			for _, value := range items {
				item := value.(map[string]any)
				if item["id"] == expected.ID {
					check("read", item["adjustment_json"].(string))
					found = true
				}
			}
			if !found {
				t.Fatal("created adjustment not returned by HTTP read")
			}
		})
	}
}

func TestAttendanceMonthHTTPPreservesBatchAndReason(t *testing.T) {
	ctx := setup(t)
	c := newAttWorkflowClient(t, ctx)
	_, _, batch := attWorkflowMonth(t, ctx, c)
	for _, action := range []string{"review", "publish", "lock", "reopen"} {
		reason := "月报" + action + "：签字确认 8月"
		payload := g.Map{"action": action, "reason": reason}
		if action != "reopen" {
			payload["batch_id"] = batch
		}
		c.ok("POST", "months/2026-08/action", payload)
		publication, e := core.Model("biz_att_publication").Ctx(ctx).Where("month", "2026-08").Where("action", action).One()
		if e != nil || publication["batch_id"].String() != batch || publication["reason"].String() != reason {
			t.Fatalf("month request did not round-trip: %v %v", publication, e)
		}
		items := c.ok("GET", "months", nil)["items"].([]any)
		if len(items) != 1 || items[0].(map[string]any)["batch_id"] != batch {
			t.Fatalf("month read lost selected batch: %v", items)
		}
	}
}
