package wecom

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/hrm"
)

func TestContactEmployeeNumberSyncAndPreservation(t *testing.T) {
	ctx := setupDB(t)
	id, err := hrm.UpsertEmployee(ctx, hrm.Employee{Name: "Number fixture", UserID: 1, WeComUserID: "number-user", EmployeeNo: "LOCAL-1", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	var extattr any
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/department/list" {
			respond(w, g.Map{"errcode": 0, "department": []Department{{ID: 1, Name: "Fixture department"}}})
			return
		}
		member := g.Map{"userid": "number-user", "name": "Number fixture", "department": []int64{1}, "status": 1}
		if extattr != nil {
			member["extattr"] = extattr
		}
		respond(w, g.Map{"errcode": 0, "userlist": []g.Map{member}})
	})
	for _, step := range []struct {
		name, attrs, want string
	}{
		{"populate", `{"attrs":[{"name":"工号","type":0,"text":{"value":"7319"}}]}`, "7319"},
		{"repeat", `{"attrs":[{"name":"工号","type":0,"text":{"value":"7319"}}]}`, "7319"},
		{"missing", `null`, "7319"},
		{"empty", `{"attrs":[{"name":"工号","type":0,"text":{"value":""}}]}`, "7319"},
		{"conflicting", `{"attrs":[{"name":"工号","type":0,"text":{"value":"7319"}},{"name":"工号","type":0,"text":{"value":"3240"}}]}`, "7319"},
		{"wrong type", `{"attrs":[{"name":"工号","type":1,"text":{"value":"3240"}}]}`, "7319"},
		{"changed", `{"attrs":[{"name":"工号","type":0,"text":{"value":"007319"}}]}`, "007319"},
	} {
		t.Run(step.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(step.attrs), &extattr); err != nil {
				t.Fatal(err)
			}
			count, err := c.SyncContact(ctx)
			if err != nil || count != 1 {
				t.Fatalf("contact sync count=%d: %v", count, err)
			}
			row, err := core.Model("biz_hrm_employee").Ctx(ctx).Where("id", id).One()
			if err != nil || row["employee_no"].String() != step.want || row["user_id"].Int64() != 1 || row["wecom_userid"].String() != "number-user" {
				t.Fatalf("employee number or existing host mapping changed incorrectly: %v", err)
			}
			if countRows(t, ctx, "biz_hrm_employee") != 1 || countRows(t, ctx, "wecom_user") != 1 {
				t.Fatal("repeated contact sync created duplicate profiles")
			}
			raw, err := core.Model("wecom_user").Ctx(ctx).Where("wecom_userid", "number-user").One()
			if err != nil || raw["employee_id"].String() != id || raw["processing_error"].String() != "" {
				t.Fatalf("raw directory mapping was not preserved: %v", err)
			}
			var retained struct {
				ExtAttr any `json:"extattr"`
			}
			if json.Unmarshal([]byte(raw["raw_json"].String()), &retained) != nil || mustJSON(retained.ExtAttr) != mustJSON(extattr) {
				t.Fatal("raw employee attributes must remain available even when ambiguous")
			}
		})
	}
}

func TestContactDuplicateEmployeeNumberDoesNotMergeUsers(t *testing.T) {
	ctx := setupDB(t)
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/department/list" {
			respond(w, g.Map{"errcode": 0, "department": []Department{{ID: 1, Name: "Fixture department"}}})
			return
		}
		members := []g.Map{}
		for _, userid := range []string{"first-number-user", "second-number-user"} {
			members = append(members, g.Map{"userid": userid, "name": userid, "department": []int64{1}, "status": 1,
				"extattr": g.Map{"attrs": []g.Map{{"name": "工号", "type": 0, "text": g.Map{"value": "7319"}}}}})
		}
		respond(w, g.Map{"errcode": 0, "userlist": members})
	})
	count, err := c.SyncContact(ctx)
	var partialErr *PartialError
	if count != 1 || !errors.As(err, &partialErr) || countRows(t, ctx, "biz_hrm_employee") != 1 || countRows(t, ctx, "wecom_user") != 2 {
		t.Fatalf("duplicate employee number must remain an explicit mapping failure: count=%d, err=%v", count, err)
	}
	employee, err := core.Model("biz_hrm_employee").Ctx(ctx).One()
	if err != nil || employee["employee_no"].String() != "7319" || employee["wecom_userid"].String() != "first-number-user" {
		t.Fatal("a number collision overwrote the first user's employee identity")
	}
	second, err := core.Model("wecom_user").Ctx(ctx).Where("wecom_userid", "second-number-user").One()
	if err != nil || second["processing_error"].String() == "" || second["employee_id"].String() != "" {
		t.Fatal("unmatched second user must retain raw data and a mapping exception")
	}
}
