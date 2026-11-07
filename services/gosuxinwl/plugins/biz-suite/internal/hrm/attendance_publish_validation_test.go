package hrm

import (
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func TestAttendancePublishHTTPExplainsDraftFieldsAndRejectsOverrides(t *testing.T) {
	ctx := setup(t)
	c := newAttWorkflowClient(t, ctx)
	for _, tc := range []struct {
		name string
		edit func(*AttRule)
		want string
	}{
		{"missing_effective_from", func(a *AttRule) { a.EffectiveFrom = "" }, "制度尚未填写生效日期"},
		{"invalid_effective_from", func(a *AttRule) { a.EffectiveFrom = "2026-02-30" }, "制度生效日期无效"},
		{"invalid_effective_to", func(a *AttRule) { a.EffectiveTo = "不是日期" }, "制度结束日期无效"},
		{"effective_to_before_start", func(a *AttRule) { a.EffectiveTo = "2026-07-31" }, "制度结束日期不能早于生效日期"},
		{"missing_cycle_anchor", func(a *AttRule) { a.Config.Cycle.Anchor = "" }, "排班周期尚未填写周期第一天"},
		{"invalid_cycle_anchor", func(a *AttRule) { a.Config.Cycle.Anchor = "2026-02-30" }, "排班周期的周期第一天无效"},
		{"missing_cycle_days", func(a *AttRule) { a.Config.Cycle.Days = nil }, "排班周期须配置1至366天"},
		{"missing_scope", func(a *AttRule) { a.Scope = AttScope{} }, "制度尚未配置适用范围"},
		{"unresolved_conflicts", func(a *AttRule) { a.Conflicts = []string{"请确认白天实际7h45是否固定核定8h"} }, "制度仍有待确认事项"},
		{"invalid_calendar_date", func(a *AttRule) {
			a.Config.Calendar = map[string]AttCalendarDay{"2026-02-30": {ShiftID: "day", Category: "workday"}}
		}, "节假日 / 单日覆盖中有无效日期"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			person := attDirtyEmployee(t, ctx, tc.name)
			draft := attWorkflowRule()
			draft.Scope = AttScope{EmployeeIDs: []string{person}}
			tc.edit(&draft)
			id := c.ok("POST", "rules", draft)["id"].(string)
			// The confirmation endpoint must validate the stored draft. Supplying
			// valid dates, scopes or cleared conflicts in its body cannot repair it.
			payload := attWorkflowRule()
			status, body := c.json(1, "POST", "rules/"+id+"/publish", payload)
			message, _ := body["message"].(string)
			if status != 200 || body["code"] == float64(0) || !strings.Contains(message, tc.want) {
				t.Fatalf("expected field-specific rejection %q: status=%d body=%v", tc.want, status, body)
			}
			stored, e := attLoadRule(ctx, id)
			if e != nil || stored.Status != "draft" {
				t.Fatalf("invalid draft was published: %+v %v", stored, e)
			}
			n, e := core.Model("biz_att_audit").Ctx(ctx).Where("entity", "rule").Where("entity_id", id).Where("action", "publish").Count()
			if e != nil || n != 0 {
				t.Fatalf("rejected publication wrote an audit success: %d %v", n, e)
			}
			// Correcting and saving the actual draft makes publication possible.
			fixed := attWorkflowRule()
			fixed.Scope = AttScope{EmployeeIDs: []string{person}}
			fixed.ID = id
			c.ok("POST", "rules", fixed)
			c.ok("POST", "rules/"+id+"/publish", g.Map{})
			stored, e = attLoadRule(ctx, id)
			if e != nil || stored.Status != "published" {
				t.Fatalf("corrected draft did not publish: %+v %v", stored, e)
			}
		})
	}
}
