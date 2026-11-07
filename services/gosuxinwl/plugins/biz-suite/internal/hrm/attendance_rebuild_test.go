package hrm

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func TestLocalSourceAlternatingDutyKeepsBothEnds(t *testing.T) {
	c, conflicts := localSourceConfig(ImportRuleDraft{Category: "生产职员", ScheduleText: "8:30-12:00 13:30-18:00/20:00", SaturdaySchedule: "8:30-12:00 13:30-18:00/20:00", Remarks: "每天留守一人值班到20：00，其余人员18:00下班"})
	if len(c.Shifts) != 4 || len(conflicts) == 0 {
		t.Fatal("ambiguous duty roster must retain candidates and require confirmation")
	}
	if c.Shifts[0].Segments[1].EndMinute != 1080 || c.Shifts[1].Segments[1].EndMinute != 1200 {
		t.Fatal("18:00 / 20:00 alternatives collapsed")
	}
	if c.Cycle.Weekdays[5].CandidateShiftIDs == nil || len(*c.Cycle.Weekdays[5].CandidateShiftIDs) != 2 {
		t.Fatal("Saturday candidates lost")
	}
	for _, id := range *c.Cycle.Weekdays[5].CandidateShiftIDs {
		if id != "saturday" && id != "saturday2" {
			t.Fatal("Saturday references weekday shift")
		}
	}
}

func TestLocalRebuildPreservesBaselineVersionsExclusionsAndIdempotency(t *testing.T) {
	ctx := context.WithValue(setup(t), "uid", int64(1))
	old := attAutoFixtureRule()
	old.ID, old.FamilyID, old.Status = "production-one-conditional-draft-20261029", "production-family", "published"
	attAutoStoreRule(t, ctx, old)
	_, err := core.Model("biz_att_import").Ctx(ctx).Data(g.Map{"id": "baseline", "kind": "baseline", "filename": "approved.xlsx", "content_hash": "frozen", "month": "2026-09", "preview_json": "{}", "mappings_json": "{}", "status": "preview", "created_by": 1, "created_at": core.Now()}).Insert()
	if err != nil {
		t.Fatal(err)
	}
	person, err := UpsertEmployee(ctx, Employee{Name: "邢玉琴", EmployeeNo: "excluded-001", DeptID: 10})
	if err != nil {
		t.Fatal(err)
	}
	_, err = core.Model("biz_hrm_employee").Ctx(ctx).Where("id", person).Update(g.Map{"status": "filtered"})
	if err != nil {
		t.Fatal(err)
	}
	id, preview, _, err := ImportAttendanceEvidence(ctx, bytes.NewReader(attendanceWorkbookFixture(t, "部门上班时间.xlsx")), "部门上班时间.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	ids, err := RebuildLocalAttendanceSources(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != len(localRuleGroups(preview.Rules)) {
		t.Fatal("source groups lost")
	}
	previous, err := attLoadRule(ctx, old.ID)
	if err != nil || previous.Status != "superseded" {
		t.Fatalf("legacy version not retained as superseded: %+v %v", previous, err)
	}
	for _, ruleID := range ids {
		a, e := attLoadRule(ctx, ruleID)
		if e != nil || a.Status != "draft" || a.EffectiveFrom != "2026-10-01" {
			t.Fatalf("invalid imported rule %+v %v", a, e)
		}
		for _, mapped := range a.Scope.EmployeeIDs {
			if mapped == person {
				t.Fatal("filtered employee restored to active scope")
			}
		}
		if validateAttConfig(a) == nil {
			t.Fatal("missing windows/scope silently publishable")
		}
	}
	again, err := RebuildLocalAttendanceSources(ctx, id)
	if err != nil || len(again) != len(ids) {
		t.Fatal("repeat migration failed", err)
	}
	count, _ := core.Model("biz_att_rule").Ctx(ctx).Count()
	if count != len(ids)+1 {
		t.Fatalf("repeat migration duplicated rules: %d", count)
	}
	baseline, _ := core.Model("biz_att_import").Ctx(ctx).Where("id", "baseline").One()
	if baseline["preview_json"].String() != "{}" || baseline["mappings_json"].String() != "{}" {
		t.Fatal("approved baseline rewritten")
	}
	status, _ := core.Model("biz_hrm_employee").Ctx(ctx).Where("id", person).Value("status")
	if status.String() != "filtered" {
		t.Fatal("filtered employee reactivated")
	}
	// Retain source evidence and the manager identity on the replacement version.
	row, _ := core.Model("biz_att_rule").Ctx(ctx).Where("family_id", old.FamilyID).Where("status", "draft").One()
	var provenance map[string]any
	_ = json.Unmarshal([]byte(row["source_json"].String()), &provenance)
	if row["version"].Int() != 2 || row["created_by"].Int64() != 1 || provenance["local_rebuild"] == nil {
		t.Fatal("replacement version or provenance missing")
	}
}
