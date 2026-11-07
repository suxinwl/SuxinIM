package hrm

import (
	"encoding/json"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

func TestAttendanceRuleSourceRoundTripsThroughHTTP(t *testing.T) {
	ctx := setup(t)
	attDirtyEmployee(t, ctx, "来源核对员工")
	c := newAttWorkflowClient(t, ctx)
	in := attWorkflowRule()
	in.Source = json.RawMessage(`{"source":"用户确认的生产一部案例","note":"必须保留来源"}`)
	id := c.ok("POST", "rules", in)["id"].(string)
	stored, err := attLoadRule(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var source map[string]any
	if err = json.Unmarshal(stored.Source, &source); err != nil || source["source"] != "用户确认的生产一部案例" {
		t.Fatalf("source not preserved: %s (%v)", stored.Source, err)
	}
	_, body := c.json(1, "GET", "rules", nil)
	items := body["data"].(map[string]any)["items"].([]any)
	for _, raw := range items {
		row := raw.(map[string]any)
		if row["id"] == id {
			if row["source"].(map[string]any)["source"] != source["source"] {
				t.Fatalf("API changed source: %v", row["source"])
			}
			stored.Name = "保存编辑的来源"
			c.ok("POST", "rules", stored)
			var omitted map[string]any
			encoded, e := json.Marshal(stored)
			if e != nil {
				t.Fatal(e)
			}
			if e = json.Unmarshal(encoded, &omitted); e != nil {
				t.Fatal(e)
			}
			delete(omitted, "source")
			omitted["name"] = "旧客户端编辑未带来源"
			c.ok("POST", "rules", omitted)
			c.ok("POST", "rules/"+id+"/publish", g.Map{})
			after, e := attLoadRule(ctx, id)
			if e != nil || string(after.Source) != string(stored.Source) {
				t.Fatalf("save or publish changed source: %s %v", after.Source, e)
			}
			omitted["name"] = "新版本也保留来源"
			newID := c.ok("POST", "rules", omitted)["id"].(string)
			version, e := attLoadRule(ctx, newID)
			if e != nil || newID == id || string(version.Source) != string(stored.Source) {
				t.Fatalf("new draft lost published source: %s %v", version.Source, e)
			}
			return
		}
	}
	t.Fatal("Saved rule missing from API list")
}
