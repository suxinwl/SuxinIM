package im

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func enterpriseHTTP(t *testing.T, ctx context.Context) func(string, string, int64, string) (int, map[string]any, string) {
	t.Helper()
	s := g.Server(guid.S())
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(func(r *ghttp.Request) {
			r.SetCtx(context.WithValue(r.Context(), "uid", r.Header.Get("X-Test-User")))
			r.Middleware.Next()
		}, core.Middleware)
		Bind(ctx, group)
	})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Shutdown() })
	return func(method, path string, uid int64, body string) (int, map[string]any, string) {
		t.Helper()
		req, err := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d/api/admin/im%s", s.GetListenedPort(), path), bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Test-User", fmt.Sprint(uid))
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]any{}
		_ = json.Unmarshal(raw, &out)
		return res.StatusCode, out, string(raw)
	}
}
func execFixture(t *testing.T, ctx context.Context, sql string, args ...any) {
	t.Helper()
	if _, err := core.DB().Exec(ctx, sql, args...); err != nil {
		t.Fatal(err)
	}
}

func TestEnterpriseDirectoryFiltersAndInactiveAccounts(t *testing.T) {
	ctx := setupDB(t)
	execFixture(t, ctx, `INSERT INTO gf_auth_dept VALUES(10,0,'集团',0,NULL),(20,10,'研发',0,NULL),(30,20,'设计',0,NULL),(40,10,'其他',0,NULL)`)
	execFixture(t, ctx, `UPDATE gf_admin SET name='同名员工' WHERE id IN(2,3)`)
	execFixture(t, ctx, `UPDATE gf_admin SET status=1,name='待启用员工' WHERE id=4`)
	execFixture(t, ctx, `INSERT INTO gf_admin(id,status,name,username) VALUES(7319,0,'王芳','3240')`)
	execFixture(t, ctx, `INSERT INTO gf_auth_role_access VALUES(7319,1)`)
	execFixture(t, ctx, `INSERT INTO gf_auth_rule VALUES(10,0,2,'biz-suite.hrm.self',''),(11,0,2,'biz-suite.hrm.department','')`)
	execFixture(t, ctx, `INSERT INTO gf_auth_role VALUES(2,0,'','[1,2,3,10]'),(3,0,'','[1,2,3,11]')`)
	execFixture(t, ctx, `UPDATE gf_auth_role_access SET role_id=2 WHERE uid=2`)
	execFixture(t, ctx, `UPDATE gf_auth_role_access SET role_id=3 WHERE uid=3`)
	execFixture(t, ctx, `INSERT INTO biz_hrm_employee VALUES('e2',2,'wx2','001','同名员工',20,'开发','active'),('e3',3,'wx3','002','同名员工',30,'设计','active'),('e4',4,'wx4','004','待启用员工',20,'','active'),('unbound-7319',NULL,'wx5','003','未映射员工',20,'','active'),('e7319',7319,'wx7319','3240','王芳',40,'','active')`)
	request := enterpriseHTTP(t, ctx)
	if _, err := SendMessage(ctx, 2, MessageInput{ConversationType: "private", ReceiverID: 7319, ClientMessageID: "cross-department", Content: "跨部门沟通"}); err != nil {
		t.Fatal("HRM self scope blocked enterprise IM", err)
	}
	status, out, raw := request("GET", "/directory/users?q=同名员工&department_id=20&include_subdepartments=true&page_size=1&page=2", 1, "")
	if status != 200 {
		t.Fatal(raw)
	}
	data := out["data"].(map[string]any)
	items := data["items"].([]any)
	if data["total"] != float64(2) || len(items) != 1 || items[0].(map[string]any)["employee_no"] != "002" {
		t.Fatalf("combination/page failed %s", raw)
	}
	_, out, raw = request("GET", "/directory/users?q=同名员工&department_id=20&include_subdepartments=false", 1, "")
	data = out["data"].(map[string]any)
	if data["total"] != float64(1) {
		t.Fatal(raw)
	}
	_, out, raw = request("GET", "/directory/users?q=7319", 1, "")
	if out["data"].(map[string]any)["total"] != float64(0) {
		t.Fatal("internal account/employee ID matched", raw)
	}
	_, out, raw = request("GET", "/directory/users?q=003", 1, "")
	p := out["data"].(map[string]any)["items"].([]any)[0].(map[string]any)
	if p["user_id"] != nil || p["can_chat"] != false || p["entry_id"] == "" {
		t.Fatal(raw)
	}
	_, out, raw = request("GET", "/directory/users?q=待启用员工", 1, "")
	p = out["data"].(map[string]any)["items"].([]any)[0].(map[string]any)
	if p["can_chat"] != false || p["unavailable_reason"] == "" {
		t.Fatal(raw)
	}
	_, out, raw = request("GET", "/directory/users?unassigned=true", 1, "")
	if out["data"].(map[string]any)["total"] != float64(1) {
		t.Fatal("manual account not in unassigned", raw)
	}
	execFixture(t, ctx, `UPDATE biz_hrm_employee SET status='inactive' WHERE id='e3'`)
	_, out, raw = request("GET", "/directory/users?q=002", 1, "")
	p = out["data"].(map[string]any)["items"].([]any)[0].(map[string]any)
	if p["can_chat"] != false {
		t.Fatal("inactive employee hidden or active", raw)
	}
	execFixture(t, ctx, `INSERT INTO wecom_user VALUES('notactivated','','未激活同事','[]','',4,'{}')`)
	_, out, raw = request("GET", "/directory/users?q=未激活同事", 1, "")
	if out["data"].(map[string]any)["total"] != float64(1) {
		t.Fatal("unmapped inactive colleague hidden", raw)
	}
	if status, _, _ := request("GET", "/private/peer?receiver_id=2&user_id=3", 1, ""); status != 400 {
		t.Fatal("conflicting peer aliases accepted")
	}
	if status, _, _ := request("GET", "/private/peer?receiver_id=2", 1, ""); status != 200 {
		t.Fatal("preferred peer alias unavailable")
	}
	for _, bad := range []string{"page=0", "page_size=101", "department_id=404", "include_subdepartments=invalid", "unassigned=true&department_id=20", "unassigned=invalid"} {
		if status, _, _ := request("GET", "/directory/users?"+bad, 1, ""); status != 400 {
			t.Fatal("invalid filter accepted", bad, status)
		}
	}
	_, _, raw = request("GET", "/directory/users", 1, "")
	for _, secret := range []string{"private@example.com", "13900000000", "raw_json", "wx7319"} {
		if strings.Contains(raw, secret) {
			t.Fatal("directory leaked private metadata", secret)
		}
	}
}

func TestEnterprisePrivateHistoryAndBilateralBlocks(t *testing.T) {
	ctx := setupDB(t)
	msg, err := SendMessage(ctx, 1, MessageInput{ConversationType: "private", ReceiverID: 2, ClientMessageID: "first", Content: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := model(ctx, "biz_im_friend").Count(); n != 0 {
		t.Fatal("direct chat created friendship")
	}
	if err = ChangeFriend(ctx, 2, 1, "block", ""); err != nil {
		t.Fatal(err)
	}
	if err = ChangeFriend(ctx, 1, 2, "unblock", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = SendMessage(ctx, 1, MessageInput{ConversationType: "private", ReceiverID: 2, ClientMessageID: "blocked", Content: "no"}); !errors.Is(err, ErrForbidden) {
		t.Fatal("other person's block was cleared", err)
	}
	if rows, err := Messages(ctx, 2, "private", "1", 0); err != nil || len(rows) != 1 {
		t.Fatal("blocked history unavailable", err)
	}
	observer := liveHub.subscribe(1)
	defer liveHub.unsubscribe(1, observer)
	if err = MarkRead(ctx, 2, "private", "1", number(msg["seq"])); err != nil {
		t.Fatal(err)
	}
	select {
	case <-observer.events:
		t.Fatal("blocked peer received a read notification")
	default:
	}
	if err = RecallMessage(ctx, 1, fmtString(msg["id"])); err != nil {
		t.Fatal("sender could not recall after block", err)
	}
	execFixture(t, ctx, `UPDATE gf_admin SET status=1 WHERE id=2`)
	if rows, err := Messages(ctx, 1, "private", "2", 0); err != nil || len(rows) != 1 || rows[0]["status"].String() != "recalled" {
		t.Fatal("disabled peer history unavailable", err)
	}
	if _, err = Messages(ctx, 2, "private", "1", 0); !errors.Is(err, ErrForbidden) {
		t.Fatal("disabled viewer revived", err)
	}
	request := enterpriseHTTP(t, ctx)
	if status, _, _ := request("GET", "/messages/"+fmtString(msg["id"])+"/receipts", 3, ""); status != 403 {
		t.Fatal("third party read receipt", status)
	}
	execFixture(t, ctx, `UPDATE gf_admin SET deletetime='2026-01-01' WHERE id=2`)
	if status, out, raw := request("GET", "/private/peer?user_id=2", 1, ""); status != 200 || out["data"].(map[string]any)["can_read"] != true {
		t.Fatal("owner history peer failed", raw)
	}
	if status, _, _ := request("GET", "/private/peer?user_id=2", 3, ""); status != 403 {
		t.Fatal("unrelated deleted profile exposed", status)
	}
}

func TestEnterpriseLegacyActionsGoneAndRoleRevocation(t *testing.T) {
	ctx := setupDB(t)
	request := enterpriseHTTP(t, ctx)
	for _, path := range []string{"/friends", "/friend-requests", "/friend-requests/old/review", "/friends/2/action"} {
		body := `{"receiver_id":2,"approve":true,"action":"delete"}`
		if status, _, raw := request("POST", path, 1, body); status != 410 {
			t.Fatalf("legacy action %s %d %s", path, status, raw)
		}
	}
	if status, _, raw := request("POST", "/contacts/2/action", 1, `{"action":"update","display_name":"同事备注"}`); status != 200 {
		t.Fatal(raw)
	}
	if status, out, raw := request("GET", "/private/peer?user_id=2", 1, ""); status != 200 || out["data"].(map[string]any)["display_name"] != "同事备注" || out["data"].(map[string]any)["name"] != "User 2" {
		t.Fatal(raw)
	}
	execFixture(t, ctx, `INSERT INTO gf_auth_role VALUES(2,0,'','[1]')`)
	execFixture(t, ctx, `UPDATE gf_auth_role_access SET role_id=2 WHERE uid=2`)
	if _, err := SendMessage(ctx, 1, MessageInput{ConversationType: "private", ReceiverID: 2, ClientMessageID: "nogate", Content: "no"}); !errors.Is(err, ErrForbidden) {
		t.Fatal("recipient without gate accepted", err)
	}
	if socketAuthorized(context.WithValue(ctx, "uid", int64(2)), 2) {
		t.Fatal("socket retained removed gate")
	}
	execFixture(t, ctx, `UPDATE gf_auth_role SET btns='[1,3]' WHERE id=2`)
	if _, err := SendMessage(ctx, 1, MessageInput{ConversationType: "private", ReceiverID: 2, ClientMessageID: "readonly-receiver", Content: "yes"}); err != nil {
		t.Fatal("readonly recipient could not receive", err)
	}
	if _, err := SendMessage(ctx, 2, MessageInput{ConversationType: "private", ReceiverID: 1, ClientMessageID: "readonly-sender", Content: "no"}); !errors.Is(err, ErrForbidden) {
		t.Fatal("readonly sender bypassed HTTP boundary", err)
	}
}

func TestSelectedGroupAtomicMembershipAndEmptyConversations(t *testing.T) {
	ctx := setupDB(t)
	subscriber := liveHub.subscribe(2)
	defer liveHub.unsubscribe(2, subscriber)
	group, err := CreateGroupWithMembers(ctx, 1, "企业群", "", 1, []int64{1, 2, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	id := fmtString(group["id"])
	rows, err := model(ctx, "biz_im_conversation").Where("stream_id", "group:"+id).All()
	if err != nil || len(rows) != 3 {
		t.Fatal("empty conversations missing", err)
	}
	for _, r := range rows {
		if r["change_seq"].Int64() <= 0 || r["last_seq"].Int64() != 0 || r["unread_count"].Int() != 0 {
			t.Fatal("invalid initial cursor")
		}
	}
	select {
	case raw := <-subscriber.events:
		if !strings.Contains(string(raw), `"membership"`) {
			t.Fatal(string(raw))
		}
	default:
		t.Fatal("created group event missing")
	}
	execFixture(t, ctx, `UPDATE gf_admin SET status=1 WHERE id=4`)
	if _, err = CreateGroupWithMembers(ctx, 1, "bad", "", 1, []int64{2, 4}); !errors.Is(err, ErrForbidden) {
		t.Fatal("inactive selection accepted", err)
	}
	execFixture(t, ctx, `CREATE TRIGGER reject_selected_member BEFORE INSERT ON biz_im_group_member WHEN NEW.user_id=3 BEGIN SELECT RAISE(ABORT,'reject member'); END`)
	if _, err = CreateGroupWithMembers(ctx, 1, "rollback", "", 1, []int64{2, 3}); err == nil {
		t.Fatal("expected rollback")
	}
	if n, _ := model(ctx, "biz_im_group").Count(); n != 1 {
		t.Fatal("failed group partially committed")
	}
	if n, _ := model(ctx, "biz_im_conversation").Count(); n != 3 {
		t.Fatal("failed conversations partially committed")
	}
	select {
	case <-subscriber.events:
		t.Fatal("failed group emitted event")
	default:
	}
}

func TestHistoricalPrivateImportDoesNotNeedCurrentPermissionsOrFriendship(t *testing.T) {
	ctx := setupDB(t)
	execFixture(t, ctx, `UPDATE gf_admin SET status=1 WHERE id IN(1,2)`)
	execFixture(t, ctx, `DELETE FROM gf_auth_role_access`)
	execFixture(t, ctx, `INSERT INTO biz_im_friend(id,user_id,friend_user_id,status,created_at,updated_at) VALUES('blocked',1,2,'blocked','2026-01-01','2026-01-01')`)
	in := MessageInput{ConversationType: "private", ReceiverID: 2, ClientMessageID: "ruoyi:historical", Content: "history"}
	if err := ValidateImport(ctx, 1, in, "2024-01-01 08:00:00"); err != nil {
		t.Fatal(err)
	}
	listener := liveHub.subscribe(2)
	defer liveHub.unsubscribe(2, listener)
	first, err := ImportHistoricalMessage(ctx, 1, in, "2024-01-01 08:00:00")
	if err != nil {
		t.Fatal(err)
	}
	again, err := ImportHistoricalMessage(ctx, 1, in, "2024-01-01 08:00:00")
	if err != nil || first["id"] != again["id"] {
		t.Fatal("history duplicate", err)
	}
	if n, _ := model(ctx, "biz_im_friend").Count(); n != 1 {
		t.Fatal("history synthesized friends")
	}
	if status, _ := model(ctx, "biz_im_friend").Where("id", "blocked").Value("status"); status.String() != "blocked" {
		t.Fatal("history cleared block")
	}
	select {
	case <-listener.events:
		t.Fatal("history broadcast")
	default:
	}
	execFixture(t, ctx, `UPDATE gf_admin SET deletetime='2026-01-01' WHERE id=2`)
	if err := ValidateImport(ctx, 1, in, "2024-01-01 08:00:00"); !errors.Is(err, ErrForbidden) {
		t.Fatal("soft-deleted history identity accepted", err)
	}
}

func TestEnterpriseGroupReinviteDoesNotRestoreAdminAndLeftEmployeeCannotReceive(t *testing.T) {
	ctx := setupDB(t)
	group, err := CreateGroupWithMembers(ctx, 1, "项目组", "", 1, []int64{2, 3})
	if err != nil {
		t.Fatal(err)
	}
	id := fmtString(group["id"])
	for _, uid := range []int64{2, 3} {
		if err = ChangeGroup(ctx, 1, id, GroupAction{Action: "set-admin", UserID: uid, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err = ChangeGroup(ctx, 1, id, GroupAction{Action: "kick", UserID: 3}); err != nil {
		t.Fatal(err)
	}
	if err = ChangeGroup(ctx, 2, id, GroupAction{Action: "invite", UserID: 3}); err != nil {
		t.Fatal(err)
	}
	role, err := model(ctx, "biz_im_group_member").Where(g.Map{"group_id": id, "user_id": 3}).Value("role")
	if err != nil || role.String() != "member" {
		t.Fatal("non-owner restored administrator", err)
	}
	execFixture(t, ctx, `INSERT INTO biz_hrm_employee VALUES('resigned',3,'wx3','003','离职同事',0,'','resigned')`)
	watch := liveHub.subscribe(3)
	defer liveHub.unsubscribe(3, watch)
	if _, err = SendMessage(ctx, 1, MessageInput{ConversationType: "group", ConversationID: id, ClientMessageID: "after-resignation", Content: "new content"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-watch.events:
		t.Fatal("resigned member received new group event")
	default:
	}
	if _, err = Messages(ctx, 3, "group", id, 0); !errors.Is(err, ErrForbidden) {
		t.Fatal("resigned member fetched new group history", err)
	}
	if socketAuthorized(context.WithValue(ctx, "uid", int64(3)), 3) {
		t.Fatal("resigned member retained socket")
	}
	request := enterpriseHTTP(t, ctx)
	status, _, raw := request("GET", "/conversations", 3, "")
	if status != 200 || strings.Contains(raw, "new content") || strings.Contains(raw, id) {
		t.Fatal("resigned member read group preview", raw)
	}
	preview, _ := model(ctx, "biz_im_conversation").Where(g.Map{"user_id": 3, "stream_id": "group:" + id}).Value("last_content")
	if preview.String() == "new content" {
		t.Fatal("resigned member preview persisted new message")
	}
}

func TestSelectedGroupHTTPDefaultApprovalAndReadableProjections(t *testing.T) {
	ctx := setupDB(t)
	request := enterpriseHTTP(t, ctx)
	status, out, raw := request("POST", "/groups", 1, `{"name":"同事讨论组","member_ids":[2]}`)
	if status != 200 {
		t.Fatal(raw)
	}
	group := out["data"].(map[string]any)
	id := group["id"].(string)
	if group["join_approval"] != float64(1) {
		t.Fatal("quick group disabled later join approval", raw)
	}
	status, _, raw = request("GET", "/conversations", 2, "")
	if status != 200 || !strings.Contains(raw, "同事讨论组") {
		t.Fatal("invited group not immediately visible", raw)
	}
	status, _, raw = request("GET", "/groups/"+id+"/members", 2, "")
	if status != 200 || !strings.Contains(raw, "User 1") || !strings.Contains(raw, "User 2") {
		t.Fatal("member names not projected", raw)
	}
	msg, err := SendMessage(ctx, 1, MessageInput{ConversationType: "private", ReceiverID: 2, ClientMessageID: "receipt-name", Content: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if err = MarkRead(ctx, 2, "private", "1", number(msg["seq"])); err != nil {
		t.Fatal(err)
	}
	status, _, raw = request("GET", "/messages/"+fmtString(msg["id"])+"/receipts", 1, "")
	if status != 200 || !strings.Contains(raw, `"name":"User 2"`) || !strings.Contains(raw, `"status":"read"`) || !strings.Contains(raw, `"read_at"`) {
		t.Fatal("receipt missing readable fields", raw)
	}
}
