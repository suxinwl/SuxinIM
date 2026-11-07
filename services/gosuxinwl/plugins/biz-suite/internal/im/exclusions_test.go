package im

import (
	"errors"
	"testing"
)

func TestExplicitExclusionsHideDirectoryAndBlockNewCommunicationPreservingHistory(t *testing.T) {
	ctx := setupDB(t)
	execFixture(t, ctx, `INSERT INTO gf_auth_dept VALUES(10,0,'总部',0,NULL),(20,10,'万佳里',0,NULL),(21,20,'万佳里子部门',0,NULL),(30,10,'已删除本地部门',0,'2026-10-29')`)
	execFixture(t, ctx, `INSERT INTO wecom_department(wecom_id,host_dept_id,parent_id,name) VALUES(1,10,0,'总部'),(2,20,1,'万佳里'),(3,21,2,'万佳里子部门')`)
	execFixture(t, ctx, `INSERT INTO biz_hrm_employee VALUES('e2',2,'wx2','0002','张峰',20,'','active'),('e3',3,'wx3','0003','过滤规则生效员工',21,'','active')`)
	execFixture(t, ctx, `INSERT INTO wecom_user VALUES('wx2','e2','张峰','[2]','',1,'{}'),('wx3','e3','过滤规则生效员工','[3]','',1,'{}'),('wx-unlinked','','未关联过滤员工','[3]','',1,'{}')`)
	if _, err := SendMessage(ctx, 1, MessageInput{ConversationType: "private", ReceiverID: 2, ClientMessageID: "before-exclusion", Content: "保留的历史"}); err != nil {
		t.Fatal(err)
	}
	execFixture(t, ctx, `UPDATE biz_hrm_employee SET status='filtered' WHERE id='e2'`)
	execFixture(t, ctx, `INSERT INTO wecom_settings VALUES(1,'{"exclude_department_names":["万佳里"],"include_subdepartments":true}','2026-10-29')`)
	directory, err := loadDirectory(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := directory.departments[20]; exists {
		t.Fatal("filtered department still present")
	}
	if _, exists := directory.departments[21]; exists {
		t.Fatal("filtered descendant department still present")
	}
	if _, exists := directory.departments[30]; exists {
		t.Fatal("deleted host department still present")
	}
	for _, id := range []int64{2, 3} {
		person := directory.byUser[id]
		if person == nil || person.listed || person.active || person.CanChat || person.AccountStatus != "filtered" {
			t.Fatalf("filtered person remains reachable: %+v", person)
		}
	}
	if directory.byUser[2].Name != "张峰" || !directory.byUser[4].listed || !directory.byUser[4].CanChat {
		t.Fatal("historical name or unrelated manual account changed")
	}
	for _, person := range directory.people {
		if person.Name == "未关联过滤员工" {
			t.Fatal("external-only fallback resurrected filtered employee")
		}
	}
	for _, pair := range [][2]int64{{1, 2}, {2, 1}, {1, 3}, {3, 1}} {
		if _, err := SendMessage(ctx, pair[0], MessageInput{ConversationType: "private", ReceiverID: pair[1], ClientMessageID: "after-exclusion", Content: "不允许发送"}); !errors.Is(err, ErrForbidden) {
			t.Fatalf("filtered employee communication allowed: %v %v", pair, err)
		}
	}
	if messages, err := Messages(ctx, 1, "private", "2", 0); err != nil || len(messages) != 1 || messages[0]["content"].String() != "保留的历史" {
		t.Fatalf("peer exclusion hid private history: %v %v", messages, err)
	}
	if _, err := CreateGroupWithMembers(ctx, 1, "不能创建", "", 1, []int64{4, 2}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("filtered group member accepted: %v", err)
	}
	if _, err := CreateGroupWithMembers(ctx, 2, "不能创建", "", 1, []int64{4}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("filtered group creator accepted: %v", err)
	}
	for _, kind := range []string{"group", "channel"} {
		if _, err := CreateSpace(ctx, 2, kind, "不能创建", "", 1); !errors.Is(err, ErrForbidden) {
			t.Fatalf("legacy space creation bypassed employee exclusion: %s %v", kind, err)
		}
	}
	if count, _ := model(ctx, "biz_im_group").Count(); count != 0 {
		t.Fatal("invalid member left a partial group")
	}
	request := enterpriseHTTP(t, ctx)
	_, result, raw := request("GET", "/directory/users", 1, "")
	if result["data"].(map[string]any)["total"] != float64(2) {
		t.Fatal("directory returned excluded people", raw)
	}
	status, result, raw := request("GET", "/private/peer?receiver_id=2", 1, "")
	if status != 200 || result["data"].(map[string]any)["name"] != "张峰" || result["data"].(map[string]any)["can_send"] != false {
		t.Fatal("historical peer is inaccessible or can send", raw)
	}
	if status, _, raw = request("GET", "/private/peer?receiver_id=3", 1, ""); status != 403 {
		t.Fatal("new excluded peer discoverable without history", raw)
	}
	t.Setenv("LIVEKIT_URL", "wss://rtc.test.invalid")
	t.Setenv("LIVEKIT_API_KEY", "test-key")
	t.Setenv("LIVEKIT_API_SECRET", "01234567890123456789012345678901")
	if status, _, raw = request("POST", "/rtc/token", 1, `{"conversation_type":"private","conversation_id":"2"}`); status != 403 {
		t.Fatal("RTC bypassed employee exclusion", raw)
	}
	// A host account can be soft-deleted independently of preserved message
	// rows. Surviving participants must retain the employee's historical name.
	execFixture(t, ctx, `UPDATE gf_admin SET deletetime='2026-10-29',status=1 WHERE id=2`)
	if messages, err := Messages(ctx, 1, "private", "2", 0); err != nil || len(messages) != 1 {
		t.Fatalf("soft deletion removed private history: %v %v", messages, err)
	}
}
