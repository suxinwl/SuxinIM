package im

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	ws "github.com/gorilla/websocket"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func setupDB(t *testing.T) context.Context {
	t.Helper()
	ctx := context.Background()
	db, err := gdb.New(gdb.ConfigNode{Type: "sqlite", Link: fmt.Sprintf("sqlite::@file(%s)", filepath.ToSlash(filepath.Join(t.TempDir(), "im.db"))), Prefix: "gf_"})
	if err != nil {
		t.Fatal(err)
	}
	if err = core.Init(db); err != nil {
		t.Fatal(err)
	}
	business := core.DB()
	t.Cleanup(func() { _ = business.Close(ctx); _ = db.Close(ctx) })
	for _, sql := range Schema() {
		if _, err = core.DB().Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = core.DB().Exec(ctx, "CREATE TABLE gf_admin (id INTEGER PRIMARY KEY, status INTEGER NOT NULL, name TEXT NOT NULL DEFAULT '', nickname TEXT NOT NULL DEFAULT '', username TEXT NOT NULL DEFAULT '', avatar TEXT NOT NULL DEFAULT '', dept_id INTEGER DEFAULT 0,deletetime DATETIME,email TEXT NOT NULL DEFAULT 'private@example.com', mobile TEXT NOT NULL DEFAULT '13900000000')"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{1, 2, 3, 4} {
		if _, err = core.DB().Exec(ctx, "INSERT INTO gf_admin (id,status,name,username) VALUES (?,0,?,?)", id, fmt.Sprintf("User %d", id), fmt.Sprintf("user%d", id)); err != nil {
			t.Fatal(err)
		}
	}
	for _, sql := range []string{
		`CREATE TABLE gf_auth_dept(id INTEGER PRIMARY KEY,pid INTEGER,name TEXT,status INTEGER,deletetime DATETIME)`,
		`CREATE TABLE gf_auth_role_access(uid BIGINT,role_id BIGINT)`,
		`CREATE TABLE gf_auth_role(id BIGINT,status INTEGER,rules TEXT,btns TEXT)`,
		`CREATE TABLE gf_auth_rule(id BIGINT,status INTEGER,type INTEGER,permission TEXT,path TEXT DEFAULT '')`,
		`INSERT INTO gf_auth_role VALUES(1,0,'','[1,2,3]')`,
		`INSERT INTO gf_auth_rule VALUES(1,0,2,'biz-suite.im.read',''),(2,0,2,'biz-suite.im.write',''),(3,0,2,'runtime.plugin.biz-suite.admin','/plugins/biz-suite/admin')`,
		`INSERT INTO gf_auth_role_access VALUES(1,1),(2,1),(3,1),(4,1)`,
		`CREATE TABLE biz_hrm_employee(id TEXT PRIMARY KEY,user_id BIGINT,wecom_userid TEXT,employee_no TEXT,name TEXT,dept_id BIGINT,position TEXT,status TEXT)`,
		`CREATE TABLE wecom_department(wecom_id BIGINT,host_dept_id BIGINT,parent_id BIGINT DEFAULT 0,name TEXT DEFAULT '')`,
		`CREATE TABLE wecom_user(wecom_userid TEXT,employee_id TEXT,name TEXT,department_ids TEXT,position TEXT,status INTEGER,raw_json TEXT)`,
		`CREATE TABLE wecom_settings(id INTEGER PRIMARY KEY,settings_json TEXT,updated_at DATETIME)`,
	} {
		if _, err = core.DB().Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	return ctx
}
func befriend(t *testing.T, ctx context.Context, a, b int64) {
	t.Helper()
	request, err := RequestFriend(ctx, a, b, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if err = ReviewFriendRequest(ctx, b, request["id"].(string), true); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateIdempotencyMembershipReadAndFiltering(t *testing.T) {
	ctx := setupDB(t)
	in := MessageInput{ConversationType: "private", ReceiverID: 2, ClientMessageID: "mobile-1", Content: "secret hello"}
	// Enterprise colleagues can communicate without creating friend rows.
	_, err := model(ctx, "biz_im_sensitive_word").Data(g.Map{"id": "filter", "word": "secret", "replacement": "***", "status": "active", "created_at": core.Now(), "updated_at": core.Now()}).Insert()
	if err != nil {
		t.Fatal(err)
	}
	receiver := liveHub.subscribe(2)
	defer liveHub.unsubscribe(2, receiver)
	outsider := liveHub.subscribe(3)
	defer liveHub.unsubscribe(3, outsider)
	first, err := SendMessage(ctx, 1, in)
	if err != nil {
		t.Fatal(err)
	}
	if first["content"] != "*** hello" {
		t.Fatal(first)
	}
	select {
	case event := <-receiver.events:
		var body map[string]any
		if json.Unmarshal(event, &body) != nil || body["type"] != "message" {
			t.Fatal(string(event))
		}
	default:
		t.Fatal("committed message not delivered")
	}
	select {
	case <-outsider.events:
		t.Fatal("message leaked to nonparticipant")
	default:
	}
	again, err := SendMessage(ctx, 1, in)
	if err != nil || again["id"] != first["id"] {
		t.Fatalf("duplicate changed: %v %v", again, err)
	}
	select {
	case <-receiver.events:
		t.Fatal("duplicate emitted event")
	default:
	}
	in.Content = "other"
	if _, err = SendMessage(ctx, 1, in); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting id accepted: %v", err)
	}
	if foreign, e := Messages(ctx, 3, "private", "1", 0); e != nil || len(foreign) != 0 {
		t.Fatalf("history leaked: %v", err)
	}
	rows, err := Messages(ctx, 2, "private", "1", 0)
	if err != nil || len(rows) != 1 {
		t.Fatalf("history: %v %v", rows, err)
	}
	seq := rows[0]["seq"].Int64()
	if err = MarkRead(ctx, 2, "private", "1", seq+999); !errors.Is(err, ErrInvalid) {
		t.Fatal("read beyond stream accepted")
	}
	if err = MarkRead(ctx, 2, "private", "1", seq); err != nil {
		t.Fatal(err)
	}
	conversation, err := model(ctx, "biz_im_conversation").Where("user_id", 2).One()
	if err != nil || conversation["unread_count"].Int() != 0 || conversation["last_read_seq"].Int64() != seq {
		t.Fatalf("bad receipt: %v %v", conversation, err)
	}
	in.ReceiverID = 1
	in.Content = "other sender same client id"
	second, err := SendMessage(ctx, 2, in)
	if err != nil || second["id"] == first["id"] {
		t.Fatalf("dedup key not sender scoped: %v", err)
	}
}

func TestRepeatedReadDoesNotRebroadcastOrClearNewUnread(t *testing.T) {
	ctx := setupDB(t)
	befriend(t, ctx, 1, 2)
	send := func(client string) int64 {
		t.Helper()
		row, err := SendMessage(ctx, 1, MessageInput{ConversationType: "private", ReceiverID: 2, ClientMessageID: client, Content: client})
		if err != nil {
			t.Fatal(err)
		}
		return row["seq"].(int64)
	}
	first, second := send("first"), send("second")
	if err := MarkRead(ctx, 2, "private", "1", second); err != nil {
		t.Fatal(err)
	}
	third := send("unread")
	watcher := liveHub.subscribe(1)
	defer liveHub.unsubscribe(1, watcher)
	before, err := model(ctx, "biz_im_conversation").Where("user_id", 2).One()
	if err != nil {
		t.Fatal(err)
	}
	for _, seq := range []int64{second, first, second} {
		if err := MarkRead(ctx, 2, "private", "1", seq); err != nil {
			t.Fatal(err)
		}
	}
	after, err := model(ctx, "biz_im_conversation").Where("user_id", 2).One()
	if err != nil || after["last_read_seq"].Int64() != second || after["unread_count"].Int() != 1 || after["change_seq"].Int64() != before["change_seq"].Int64() {
		t.Fatalf("repeated receipt changed conversation: %v %v", after, err)
	}
	select {
	case event := <-watcher.events:
		t.Fatalf("duplicate receipt was broadcast: %s", event)
	default:
	}
	if err := MarkRead(ctx, 2, "private", "1", third); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-watcher.events:
		if !strings.Contains(string(event), `"type":"receipt"`) {
			t.Fatal(string(event))
		}
	default:
		t.Fatal("advanced receipt was not broadcast")
	}
}

func TestGroupAndChannelApprovalBoundary(t *testing.T) {
	ctx := setupDB(t)
	group, err := CreateSpace(ctx, 1, "group", "Finance", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	id := group["id"].(string)
	request, err := JoinSpace(ctx, 2, "group", id)
	if err != nil {
		t.Fatal(err)
	}
	in := MessageInput{ConversationType: "group", ConversationID: id, ClientMessageID: "m1", Content: "approved only"}
	if _, err = SendMessage(ctx, 2, in); !errors.Is(err, ErrForbidden) {
		t.Fatalf("pending member send: %v", err)
	}
	if err = ReviewGroupRequest(ctx, 3, id, request["id"].(string), true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("outsider approved join: %v", err)
	}
	if err = ReviewGroupRequest(ctx, 1, id, request["id"].(string), true); err != nil {
		t.Fatal(err)
	}
	if _, err = SendMessage(ctx, 2, in); err != nil {
		t.Fatal(err)
	}
	if _, err = Messages(ctx, 3, "group", id, 0); !errors.Is(err, ErrForbidden) {
		t.Fatalf("outsider reads group: %v", err)
	}
	channel, err := CreateSpace(ctx, 1, "channel", "News", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	channelID := channel["id"].(string)
	if _, err = Messages(ctx, 2, "channel", channelID, 0); !errors.Is(err, ErrForbidden) {
		t.Fatalf("nonmember reads channel: %v", err)
	}
	if _, err = JoinSpace(ctx, 2, "channel", channelID); err != nil {
		t.Fatal(err)
	}
	in = MessageInput{ConversationType: "channel", ConversationID: channelID, ClientMessageID: "c1", Content: "member message"}
	if _, err = SendMessage(ctx, 2, in); !errors.Is(err, ErrForbidden) {
		t.Fatal("ordinary subscriber can publish channel broadcasts")
	}
	if _, err = SendMessage(ctx, 1, in); err != nil {
		t.Fatal(err)
	}
	rows, err := Messages(ctx, 1, "channel", channelID, 0)
	if err != nil || len(rows) != 1 {
		t.Fatalf("channel history %v %v", rows, err)
	}
}

func TestFriendReviewAuthorizationAndRollback(t *testing.T) {
	ctx := setupDB(t)
	request, err := RequestFriend(ctx, 1, 2, "hello")
	if err != nil {
		t.Fatal(err)
	}
	id := request["id"].(string)
	if err = ReviewFriendRequest(ctx, 3, id, true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("third party approved: %v", err)
	}
	n, _ := model(ctx, "biz_im_friend").Count()
	if n != 0 {
		t.Fatal("unauthorized approval changed relationships")
	}
	if err = ReviewFriendRequest(ctx, 2, id, false); err != nil {
		t.Fatal(err)
	}
	if err = ReviewFriendRequest(ctx, 2, id, true); !errors.Is(err, ErrConflict) {
		t.Fatal("rejected request rewritten without reapply")
	}
	_, err = RequestFriend(ctx, 1, 2, "try again")
	if err != nil {
		t.Fatal(err)
	}
	if err = ReviewFriendRequest(ctx, 2, id, true); err != nil {
		t.Fatal(err)
	}
	n, _ = model(ctx, "biz_im_friend").Count()
	if n != 2 {
		t.Fatalf("expected symmetric friendship, got %d", n)
	}
}

func TestHTTPRejectsAnonymousAndWebSocketPush(t *testing.T) {
	ctx := setupDB(t)
	befriend(t, ctx, 1, 2)
	s := g.Server(guid.S())
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.Group("/", func(group *ghttp.RouterGroup) {
		// Test identity source only. Production identity is injected by the signed host proxy.
		group.Middleware(func(r *ghttp.Request) {
			if r.Header.Get("X-Test-User") == "2" {
				r.SetCtx(context.WithValue(r.Context(), "uid", int64(2)))
			}
			r.Middleware.Next()
		})
		Bind(ctx, group)
	})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	base := fmt.Sprintf("127.0.0.1:%d", s.GetListenedPort())
	response, err := http.Get("http://" + base + "/api/admin/im/conversations")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatalf("anonymous status %d", response.StatusCode)
	}
	if _, err = core.DB().Exec(ctx, "UPDATE gf_admin SET status=1 WHERE id=4"); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest("GET", "http://"+base+"/api/admin/im/users?q=user", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Test-User", "2")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || strings.Contains(string(body), "private@example.com") || strings.Contains(string(body), "13900000000") || strings.Contains(string(body), "user4") {
		t.Fatalf("directory leaked inactive or sensitive data: %s", body)
	}
	t.Setenv("LIVEKIT_URL", "")
	t.Setenv("LIVEKIT_API_KEY", "")
	t.Setenv("LIVEKIT_API_SECRET", "")
	postRTC := func() (*http.Response, []byte) {
		request, e := http.NewRequest("POST", "http://"+base+"/api/admin/im/rtc/token", bytes.NewBufferString(`{"conversation_type":"private","conversation_id":"1"}`))
		if e != nil {
			t.Fatal(e)
		}
		request.Header.Set("X-Test-User", "2")
		request.Header.Set("Content-Type", "application/json")
		response, e := http.DefaultClient.Do(request)
		if e != nil {
			t.Fatal(e)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		return response, body
	}
	response, body = postRTC()
	if response.StatusCode != 503 {
		t.Fatalf("unconfigured RTC not unavailable: %d %s", response.StatusCode, body)
	}
	secret := strings.Repeat("x", 40)
	t.Setenv("LIVEKIT_URL", "wss://rtc.example.test")
	t.Setenv("LIVEKIT_API_KEY", "api-key")
	t.Setenv("LIVEKIT_API_SECRET", secret)
	response, body = postRTC()
	if response.StatusCode != 200 {
		t.Fatalf("RTC token: %d %s", response.StatusCode, body)
	}
	var envelope struct {
		Data struct {
			Token string `json:"token"`
			Room  string `json:"room"`
		} `json:"data"`
	}
	if err = json.Unmarshal(body, &envelope); err != nil {
		t.Fatal(err)
	}
	claims := jwt.MapClaims{}
	if _, err = jwt.ParseWithClaims(envelope.Data.Token, claims, func(token *jwt.Token) (any, error) { return []byte(secret), nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("api-key")); err != nil {
		t.Fatal(err)
	}
	video, ok := claims["video"].(map[string]any)
	if !ok || video["room"] != envelope.Data.Room || video["roomJoin"] != true || video["roomAdmin"] == true || claims["sub"] != "2" {
		t.Fatalf("overbroad RTC grant: %v", claims)
	}
	conn, _, err := ws.DefaultDialer.Dial("ws://"+base+"/api/admin/im/ws", http.Header{"X-Test-User": []string{"2"}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Wait until subscribe has run after the upgrade handshake.
	deadline := time.Now().Add(time.Second)
	for {
		liveHub.mu.RLock()
		n := len(liveHub.users[2])
		liveHub.mu.RUnlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("subscription absent")
		}
		time.Sleep(time.Millisecond)
	}
	_, err = SendMessage(ctx, 1, MessageInput{ConversationType: "private", ReceiverID: 2, ClientMessageID: "socket-message", Content: "delivered over socket"})
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	var event map[string]any
	if err = conn.ReadJSON(&event); err != nil {
		t.Fatal(err)
	}
	if event["type"] != "message" {
		t.Fatal(event)
	}
}

func TestGroupControlsEnforceRolesAndImmediateRevocation(t *testing.T) {
	ctx := setupDB(t)
	group, err := CreateSpace(ctx, 1, "group", "Operations", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	id := group["id"].(string)
	if _, err = JoinSpace(ctx, 2, "group", id); err != nil {
		t.Fatal(err)
	}
	if err = ChangeGroup(ctx, 2, id, GroupAction{Action: "set-admin", UserID: 2, Enabled: true}); !errors.Is(err, ErrForbidden) {
		t.Fatal("member self-escalated")
	}
	if err = ChangeGroup(ctx, 1, id, GroupAction{Action: "mute-member", UserID: 2, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	in := MessageInput{ConversationType: "group", ConversationID: id, ClientMessageID: "muted", Content: "blocked"}
	if _, err = SendMessage(ctx, 2, in); !errors.Is(err, ErrForbidden) {
		t.Fatal("muted member sent message")
	}
	if err = ChangeGroup(ctx, 1, id, GroupAction{Action: "mute-member", UserID: 2, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if _, err = SendMessage(ctx, 2, in); err != nil {
		t.Fatal(err)
	}
	if err = ChangeGroup(ctx, 1, id, GroupAction{Action: "kick", UserID: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err = Messages(ctx, 2, "group", id, 0); !errors.Is(err, ErrForbidden) {
		t.Fatal("kicked member reads messages")
	}
	if _, err = SendMessage(ctx, 2, MessageInput{ConversationType: "group", ConversationID: id, ClientMessageID: "after-kick", Content: "leak"}); !errors.Is(err, ErrForbidden) {
		t.Fatal("kicked member sends")
	}
	n, _ := model(ctx, "biz_im_conversation").Where(g.Map{"user_id": 2, "stream_id": "group:" + id}).Count()
	if n != 0 {
		t.Fatal("kicked member retained conversation access")
	}
	if err = ChangeGroup(ctx, 1, id, GroupAction{Action: "invite", UserID: 2}); err != nil {
		t.Fatal(err)
	}
	if err = ChangeGroup(ctx, 1, id, GroupAction{Action: "transfer-owner", UserID: 2}); err != nil {
		t.Fatal(err)
	}
	if err = ChangeGroup(ctx, 1, id, GroupAction{Action: "dissolve"}); !errors.Is(err, ErrForbidden) {
		t.Fatal("former owner retained owner privilege")
	}
	if err = ChangeGroup(ctx, 2, id, GroupAction{Action: "dissolve"}); err != nil {
		t.Fatal(err)
	}
}

func TestRecallCatchUpAndBlockedPeer(t *testing.T) {
	ctx := setupDB(t)
	befriend(t, ctx, 1, 2)
	in := MessageInput{ConversationType: "private", ReceiverID: 2, ClientMessageID: "to-recall", Content: "a secret"}
	message, err := SendMessage(ctx, 1, in)
	if err != nil {
		t.Fatal(err)
	}
	seq := message["seq"].(int64)
	id := message["id"].(string)
	if err = RecallMessage(ctx, 2, id); !errors.Is(err, ErrForbidden) {
		t.Fatal("recipient recalled sender message")
	}
	if err = RecallMessage(ctx, 1, id); err != nil {
		t.Fatal(err)
	}
	rows, err := Messages(ctx, 2, "private", "1", seq)
	if err != nil || len(rows) != 1 || rows[0]["status"].String() != "recalled" || rows[0]["content"].String() != "" {
		t.Fatalf("offline recall not catchable: %v %v", rows, err)
	}
	duplicate, err := SendMessage(ctx, 1, in)
	if err != nil || duplicate["id"] != id || duplicate["status"] != "recalled" {
		t.Fatalf("recall resend recreated content %v %v", duplicate, err)
	}
	if err = ChangeFriend(ctx, 2, 1, "block", ""); err != nil {
		t.Fatal(err)
	}
	in.ClientMessageID = "blocked"
	if _, err = SendMessage(ctx, 1, in); !errors.Is(err, ErrForbidden) {
		t.Fatal("blocked peer bypassed relationship check")
	}
}

func TestConcurrentDuplicateCommitsOnce(t *testing.T) {
	ctx := setupDB(t)
	befriend(t, ctx, 1, 2)
	const workers = 12
	var wg sync.WaitGroup
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := SendMessage(ctx, 1, MessageInput{ConversationType: "private", ReceiverID: 2, ClientMessageID: "same-network-retry", Content: "one durable message"})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	n, err := model(ctx, "biz_im_message").Count()
	if err != nil || n != 1 {
		t.Fatalf("duplicate rows %d %v", n, err)
	}
	row, err := model(ctx, "biz_im_conversation").Where("user_id", 2).One()
	if err != nil || row["unread_count"].Int() != 1 {
		t.Fatalf("duplicate unread increment: %v %v", row, err)
	}
}

func TestImportedHistoryPreservesTimeAndDeduplicates(t *testing.T) {
	ctx := setupDB(t)
	befriend(t, ctx, 1, 2)
	in := MessageInput{ConversationType: "private", ReceiverID: 2, ClientMessageID: "ruoyi:private-99", Content: "historical"}
	sub := liveHub.subscribe(2)
	defer liveHub.unsubscribe(2, sub)
	out, err := ImportHistoricalMessage(ctx, 1, in, "2024-01-02 11:12:13")
	if err != nil {
		t.Fatal(err)
	}
	if out["send_time"] != "2024-01-02 11:12:13" {
		t.Fatal(out)
	}
	_, err = ImportHistoricalMessage(ctx, 1, in, "2024-01-02 11:12:13")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-sub.events:
		t.Fatal("history import emitted new-message notification")
	default:
	}
	n, err := model(ctx, "biz_im_message").Count()
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
}

func TestWebSocketClosesAfterRoleRevocation(t *testing.T) {
	ctx := setupDB(t)
	statements := []string{
		"UPDATE gf_auth_role SET btns='[1,3]' WHERE id=1",
	}
	for _, sql := range statements {
		if _, err := core.DB().Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	userContext := context.WithValue(ctx, "uid", int64(2))
	if !socketAuthorized(userContext, 2) {
		t.Fatal("valid read role refused")
	}
	s := g.Server(guid.S())
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.BindHandler("/ws", func(r *ghttp.Request) {
		r.SetCtx(context.WithValue(r.Context(), "uid", int64(2)))
		serveWebSocket(r, 20*time.Millisecond)
	})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	conn, _, err := ws.DefaultDialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d/ws", s.GetListenedPort()), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err = core.DB().Exec(ctx, "UPDATE gf_auth_role SET status=1 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, _, err = conn.ReadMessage()
	if !ws.IsCloseError(err, ws.ClosePolicyViolation) {
		t.Fatalf("socket remained authorized after role removed: %v", err)
	}
	if _, err = core.DB().Exec(ctx, "UPDATE gf_auth_role SET status=0 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if _, err = core.DB().Exec(ctx, "UPDATE gf_admin SET status=1 WHERE id=2"); err != nil {
		t.Fatal(err)
	}
	if socketAuthorized(userContext, 2) {
		t.Fatal("disabled host identity retained websocket permission")
	}
}
