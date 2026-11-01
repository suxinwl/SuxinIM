package enterprise

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type testAuth struct {
	mu      sync.Mutex
	revoked map[string]bool
}

func (a *testAuth) Authenticate(_ context.Context, r *http.Request, admin bool) (Actor, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	user := r.Header.Get("chatToken")
	if user == "" || a.revoked[user] {
		return Actor{}, unauthorized("revoked")
	}
	x := Actor{UserID: user, IMToken: r.Header.Get("token"), ChatToken: user, Platform: 5, Admin: user == "admin"}
	if admin && !x.Admin {
		return x, forbidden("admin required")
	}
	return x, nil
}

type testIM struct {
	mu       sync.Mutex
	members  map[string][]User
	messages map[string]Message
	sent     map[string]int
	fail     map[string]bool
}

func (i *testIM) Users(_ context.Context, ids []string) ([]User, error) {
	us := []User{}
	for _, id := range ids {
		if id == "missing" {
			continue
		}
		us = append(us, User{UserID: id, Nickname: "Name " + id})
	}
	return us, nil
}
func (i *testIM) Friends(context.Context, string) ([]string, error) {
	return []string{"alice", "bob", "carol"}, nil
}
func (i *testIM) GroupMembers(_ context.Context, g string) ([]User, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]User(nil), i.members[g]...), nil
}
func (i *testIM) Message(_ context.Context, c, id string, seq int64, sender string) (Message, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	m, ok := i.messages[id]
	if !ok || c != "sg_"+m.GroupID || seq > 0 && seq != m.Seq {
		return m, ErrNotFound
	}
	return m, nil
}
func (i *testIM) Send(_ context.Context, a Actor, to, group, content string, kind int) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.sent[to]++
	if i.fail[to] {
		return "", fmt.Errorf("injected failure")
	}
	return "msg-" + to, nil
}

type testMedia struct {
	LiveKit
	mu                            sync.Mutex
	deleted, removed, permissions int
}

func (m *testMedia) Create(context.Context, string) error { return nil }
func (m *testMedia) Delete(context.Context, string) error {
	m.mu.Lock()
	m.deleted++
	m.mu.Unlock()
	return nil
}
func (m *testMedia) Remove(context.Context, string, string) error {
	m.mu.Lock()
	m.removed++
	m.mu.Unlock()
	return nil
}
func (m *testMedia) Permissions(context.Context, string, string, []string) error {
	m.mu.Lock()
	m.permissions++
	m.mu.Unlock()
	return nil
}
func (m *testMedia) Mute(context.Context, string, string, string, bool) error { return nil }
func (m *testMedia) Proxy(w http.ResponseWriter, r *http.Request)             { w.WriteHeader(204) }

type testCatalog struct {
	mu     sync.Mutex
	latest map[string]Application
	fail   bool
}

func (c *testCatalog) Latest(_ context.Context, p string) (Application, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail {
		return Application{}, fmt.Errorf("offline")
	}
	v, ok := c.latest[p]
	if !ok {
		return v, ErrNotFound
	}
	return v, nil
}
func (c *testCatalog) Add(_ context.Context, _ Actor, v Application) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	v.ID = "official-id"
	c.latest[v.Platform] = v
	return nil
}
func fixture(t *testing.T) (*Server, *testIM, *testMedia, *time.Time) {
	t.Helper()
	now := time.Now().UTC()
	im := &testIM{members: map[string][]User{"g": {{UserID: "alice"}, {UserID: "bob"}, {UserID: "carol"}}}, messages: map[string]Message{}, sent: map[string]int{}, fail: map[string]bool{}}
	media := &testMedia{LiveKit: LiveKit{Key: "test-key", Secret: strings.Repeat("secret", 8), PublicURL: "wss://test.invalid/livekit"}}
	cipher, e := NewSessionCipher(strings.Repeat("ab", 32))
	if e != nil {
		t.Fatal(e)
	}
	s := &Server{Store: NewMemoryStore(), Auth: &testAuth{revoked: map[string]bool{}}, IM: im, Media: media, Cipher: cipher, Catalog: &testCatalog{latest: map[string]Application{}}, WebhookSecret: "private-test-secret", Now: func() time.Time { return now }}
	return s, im, media, &now
}
func req(v any) request { var q request; _ = json.Unmarshal(mustJSON(v), &q); return q }
func actor(id string) Actor {
	return Actor{UserID: id, Platform: 5, IMToken: "im-" + id, ChatToken: id, Admin: id == "admin"}
}
func post(s *Server, user, path string, body any) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", path, strings.NewReader(string(mustJSON(body))))
	if user != "" {
		r.Header.Set("token", "im-"+user)
		r.Header.Set("chatToken", user)
	}
	s.ServeHTTP(w, r)
	return w
}
func mustData(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("HTTP %d %s", w.Code, w.Body.String())
	}
	var e struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	return e.Data
}

func TestMomentVisibilityCommentOwnershipAndUnread(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := context.Background()
	v, e := s.office(ctx, actor("alice"), "/office/work_moment/add", req(map[string]any{"content": map[string]any{"text": "private audience", "type": 0}, "permission": 2, "permissionUserIDs": []string{"bob"}}))
	if e != nil {
		t.Fatal(e)
	}
	m := v.(map[string]any)["workMoment"].(Moment)
	q := req(map[string]any{"workMomentID": m.WorkMomentID})
	if _, e = s.office(ctx, actor("carol"), "/office/work_moment/get", q); e == nil {
		t.Fatal("non-audience read allowed")
	}
	if _, e = s.office(ctx, actor("carol"), "/office/work_moment/comment/add", req(map[string]any{"workMomentID": m.WorkMomentID, "content": "attack"})); e == nil {
		t.Fatal("hidden moment comment allowed")
	}
	comment, e := s.office(ctx, actor("bob"), "/office/work_moment/comment/add", req(map[string]any{"workMomentID": m.WorkMomentID, "content": "allowed"}))
	if e != nil {
		t.Fatal(e)
	}
	id := comment.(map[string]any)["commentID"].(string)
	if _, e = s.office(ctx, actor("carol"), "/office/work_moment/comment/del", req(map[string]any{"workMomentID": m.WorkMomentID, "commentID": id})); e == nil {
		t.Fatal("foreign comment delete allowed")
	}
	unread, e := s.office(ctx, actor("alice"), "/office/work_moment/unread/count", req(map[string]any{}))
	if e != nil || unread.(map[string]any)["total"].(int) != 1 {
		t.Fatalf("unread: %v %v", unread, e)
	}
	if _, e = s.office(ctx, actor("alice"), "/office/work_moment/comment/del", req(map[string]any{"workMomentID": m.WorkMomentID, "commentID": id})); e != nil {
		t.Fatal(e)
	}
	for j := 0; j < 2; j++ {
		if _, e = s.office(ctx, actor("bob"), "/office/work_moment/like", req(map[string]any{"workMomentID": m.WorkMomentID, "like": true})); e != nil {
			t.Fatal(e)
		}
	}
	stored, _, _ := load[Moment](ctx, s.Store, "moment:"+m.WorkMomentID)
	if len(stored.LikeUsers) != 1 {
		t.Fatal("duplicate likes")
	}
}
func TestTagOwnershipDeduplicatedTargetsAndPartialRetry(t *testing.T) {
	s, im, _, _ := fixture(t)
	ctx := context.Background()
	tag, e := s.tags(ctx, actor("alice"), "add", req(map[string]any{"tagName": "team", "userIDs": []string{"bob", "bob"}, "groupIDs": []string{"g"}}))
	if e != nil {
		t.Fatal(e)
	}
	tid := tag.(map[string]any)["tagID"].(string)
	if _, e = s.tags(ctx, actor("bob"), "get", req(map[string]any{"tagID": tid})); e == nil {
		t.Fatal("foreign tag read allowed")
	}
	im.fail["carol"] = true
	body := req(map[string]any{"requestID": "one", "tagIDs": []string{tid}, "userIDs": []string{"bob"}, "content": "{\"data\":\"{\\\"customType\\\":903}\",\"description\":\"mass\",\"extension\":\"\"}"})
	v, e := s.sendTag(ctx, actor("alice"), body)
	if e != nil {
		t.Fatal(e)
	}
	log := v.(map[string]any)["sendLog"].(TagSendLog)
	if len(log.Delivery) != 2 || im.sent["bob"] != 1 || im.sent["carol"] != 1 {
		t.Fatalf("dedup: %+v", log)
	}
	if _, e = s.sendTag(ctx, actor("alice"), body); e != nil || im.sent["bob"] != 1 {
		t.Fatal("same request resent")
	}
	im.fail["carol"] = false
	if _, e = s.retryTag(ctx, actor("alice"), log.ID); e != nil {
		t.Fatal(e)
	}
	if im.sent["bob"] != 1 || im.sent["carol"] != 2 {
		t.Fatal("retry resent successful recipient")
	}
}
func TestReceiptSnapshotExcludesSenderSurvivesMemberChangesAndRetries(t *testing.T) {
	s, im, _, now := fixture(t)
	ctx := context.Background()
	m := Message{ClientMsgID: "m1", SendID: "alice", GroupID: "g", Seq: 10, SendTime: now.UnixMilli()}
	im.messages[m.ClientMsgID] = m
	if _, e := s.snapshot(ctx, m); e != nil {
		t.Fatal(e)
	}
	im.members["g"] = []User{{UserID: "alice"}, {UserID: "bob"}, {UserID: "late", JoinTime: now.Add(time.Minute).Unix()}}
	body := req(map[string]any{"conversationID": "sg_g", "clientMsgs": map[string]string{"m1": "alice"}, "seqs": map[string]int64{"m1": 10}})
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for j := 0; j < 10; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.receipts(ctx, actor("bob"), "/msg/mark_group_message_read", body)
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	r, _, e := load[Receipt](ctx, s.Store, receiptID("sg_g", "m1"))
	if e != nil || len(r.Reads) != 1 || len(r.Recipients) != 2 || contains(r.Recipients, "alice") || contains(r.Recipients, "late") {
		t.Fatalf("invalid snapshot: %+v %v", r, e)
	}
	v, e := s.receipts(ctx, actor("alice"), "/msg/get_group_message_has_read", req(map[string]any{"conversationID": "sg_g", "clientMsgID": "m1", "filter": 1, "offset": 0, "count": 20}))
	if e != nil {
		t.Fatal(e)
	}
	reads := v.(map[string]any)["reads"].([]map[string]any)
	if len(reads) != 1 || reads[0]["userID"] != "carol" {
		t.Fatalf("removed recipient lost: %v", reads)
	}
	if _, e = s.receipts(ctx, actor("late"), "/msg/mark_group_message_read", body); e == nil {
		t.Fatal("late joiner allowed")
	}
	stream, _, e := load[eventStream](ctx, s.Store, "events:alice")
	if e != nil || len(stream.Events) != 1 {
		t.Fatalf("receipt event not idempotent: %+v %v", stream, e)
	}
}
func TestWebhookDuplicateOutOfOrderAndPrivateEndpoint(t *testing.T) {
	s, _, _, now := fixture(t)
	path := "/internal/openim/webhook/private-test-secret/"
	first := map[string]any{"callbackCommand": "callbackBeforeSendGroupMsgCommand", "clientMsgID": "m", "serverMsgID": "server-m", "sendID": "alice", "groupID": "g", "sessionType": 3, "sendTime": now.UnixMilli(), "seq": 0}
	for j := 0; j < 2; j++ {
		w := post(s, "", path+"callbackBeforeSendGroupMsgCommand", first)
		if w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	first["callbackCommand"] = "callbackAfterMsgSaveDBCommand"
	first["seq"] = 18
	if w := post(s, "", path+"callbackAfterMsgSaveDBCommand", first); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	first["callbackCommand"] = "callbackBeforeSendGroupMsgCommand"
	first["seq"] = 0
	_ = post(s, "", path+"callbackBeforeSendGroupMsgCommand", first)
	r, _, e := load[Receipt](context.Background(), s.Store, receiptID("sg_g", "m"))
	if e != nil || r.Message.Seq != 18 {
		t.Fatalf("late old callback overwrote actual seq: %+v %v", r, e)
	}
	if w := post(s, "", "/internal/openim/webhook/wrong", first); w.Code != 401 {
		t.Fatal("webhook secret not enforced")
	}
}
func TestOptionalWebhookReceiverAcknowledgesNativeProtocolAndDeduplicates(t *testing.T) {
	s, _, _, _ := fixture(t)
	commands := []string{"callbackAfterUserOfflineCommand", "callbackAfterUserOnlineCommand", "callbackBeforeOfflinePushCommand", "callbackAfterSingleMsgReadCommand"}
	for _, command := range commands {
		payload := map[string]any{"callbackCommand": command, "operationID": "stable-event-id", "userID": "bob"}
		for repeat := 0; repeat < 2; repeat++ {
			w := post(s, "", "/internal/openim/webhook/private-test-secret/"+command, payload)
			var ack map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &ack); err != nil || w.Code != 200 || ack["errCode"] != float64(0) || ack["actionCode"] != float64(0) || ack["nextCode"] != float64(0) {
				t.Fatalf("native webhook acknowledgment: %d %s", w.Code, w.Body.String())
			}
			if _, nested := ack["data"]; nested {
				t.Fatal("webhook ACK was wrapped in normal REST response")
			}
		}
	}
	docs, err := s.Store.List(context.Background(), "webhook", "")
	if err != nil || len(docs) != len(commands) {
		t.Fatalf("duplicate audit delivery: count=%d error=%v", len(docs), err)
	}
}
func TestNativeWebhookWithoutOperationIDKeepsDistinctReadSequences(t *testing.T) {
	s, _, _, _ := fixture(t)
	command := "callbackAfterSingleMsgReadCommand"
	for _, seq := range []int{1, 2} {
		payload := map[string]any{"callbackCommand": command, "userID": "bob", "conversationID": "si_alice_bob", "Seqs": []int{seq}, "contentType": 1}
		for repeat := 0; repeat < 2; repeat++ {
			w := post(s, "", "/internal/openim/webhook/private-test-secret/"+command, payload)
			if w.Code != 200 {
				t.Fatalf("native read ACK: %d %s", w.Code, w.Body.String())
			}
		}
	}
	docs, err := s.Store.List(context.Background(), "webhook", command)
	if err != nil || len(docs) != 2 {
		t.Fatalf("distinct native reads were collapsed: count=%d error=%v", len(docs), err)
	}
}
func callInvitation() request {
	return req(map[string]any{"invitation": map[string]any{"inviterUserID": "alice", "inviteeUserIDList": []string{"bob"}, "mediaType": "video", "sessionType": 1, "timeout": 30}})
}
func TestCallDoubleAcceptCancellationRaceAndClosedTokenGateway(t *testing.T) {
	s, _, media, _ := fixture(t)
	ctx := context.Background()
	v, e := s.invite(ctx, actor("alice"), callInvitation())
	if e != nil {
		t.Fatal(e)
	}
	roomID := v.(map[string]any)["roomID"].(string)
	token := v.(map[string]any)["token"].(string)
	r, _, _ := load[Room](ctx, s.Store, "room:"+roomID)
	bob := actor("bob")
	if _, e = s.joinRoom(ctx, bob, r, "accept"); e != nil {
		t.Fatal(e)
	}
	bob.Platform = 2
	if _, e = s.joinRoom(ctx, bob, r, "accept"); e == nil {
		t.Fatal("accepted twice on different platforms")
	}
	if _, e = s.signaling(ctx, actor("alice"), "/signaling/cancel", req(map[string]any{"roomID": roomID})); e == nil {
		t.Fatal("cancel beat an already accepted call")
	}
	w := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/livekit/rtc?access_token="+token, nil)
	s.ServeHTTP(w, request)
	if w.Code != 204 {
		t.Fatalf("valid gateway rejected: %s", w.Body.String())
	}
	if _, e = s.signaling(ctx, actor("alice"), "/signaling/hungUp", req(map[string]any{"roomID": roomID})); e != nil {
		t.Fatal(e)
	}
	w = httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/livekit/rtc?access_token="+token, nil))
	if w.Code != 403 {
		t.Fatalf("ended room token accepted: %d %s", w.Code, w.Body.String())
	}
	if media.deleted != 1 {
		t.Fatal("media room not deleted")
	}
	if _, e = s.signaling(ctx, actor("bob"), "/signaling/getTokenByRoomID", req(map[string]any{"roomID": roomID})); e == nil {
		t.Fatal("closed room token issued")
	}
}
func TestCallTimeoutPersistsAcrossServiceRestartAndRevokedSession(t *testing.T) {
	s, _, media, now := fixture(t)
	ctx := context.Background()
	v, e := s.invite(ctx, actor("alice"), callInvitation())
	if e != nil {
		t.Fatal(e)
	}
	roomID := v.(map[string]any)["roomID"].(string)
	*now = now.Add(time.Minute)
	restarted := &Server{Store: s.Store, Auth: s.Auth, IM: s.IM, Media: s.Media, Cipher: s.Cipher, Catalog: s.Catalog, Now: s.Now}
	if e = restarted.reconcileRooms(ctx); e != nil {
		t.Fatal(e)
	}
	r, _, _ := load[Room](ctx, s.Store, "room:"+roomID)
	if r.State != "timed_out" || media.deleted != 1 {
		t.Fatalf("timeout lost after restart: %+v", r)
	}
	s2, _, m2, _ := fixture(t)
	v, e = s2.invite(ctx, actor("alice"), callInvitation())
	if e != nil {
		t.Fatal(e)
	}
	s2.Auth.(*testAuth).revoked["alice"] = true
	token := v.(map[string]any)["token"].(string)
	w := httptest.NewRecorder()
	s2.ServeHTTP(w, httptest.NewRequest("GET", "/livekit/rtc?access_token="+token, nil))
	if w.Code != 401 {
		t.Fatal("revoked session reconnected")
	}
	if e = s2.revalidateMediaSessions(ctx); e != nil {
		t.Fatal(e)
	}
	if m2.removed != 1 {
		t.Fatal("revoked media participant not removed")
	}
}
func TestMeetingHostPermissionsScheduleAndUnauthorizedRoom(t *testing.T) {
	s, _, _, now := fixture(t)
	ctx := context.Background()
	v, e := s.createMeeting(ctx, actor("alice"), req(map[string]any{"meetingName": "planning", "inviteeUserIDList": []string{"bob"}, "startTime": now.Add(time.Hour).Unix(), "meetingDuration": 3600}))
	if e != nil {
		t.Fatal(e)
	}
	id := v.(map[string]any)["roomID"].(string)
	r, _, _ := load[Room](ctx, s.Store, "room:"+id)
	if _, e = s.joinRoom(ctx, actor("bob"), r, "signal_join_meeting"); e == nil {
		t.Fatal("joined before scheduled start")
	}
	if _, e = s.signaling(ctx, actor("carol"), "/signaling/signal_join_meeting", req(map[string]any{"roomID": id})); e == nil {
		t.Fatal("uninvited user joined")
	}
	if _, e = s.updateMeeting(ctx, actor("bob"), r, req(map[string]any{"onlyHostShareScreen": true})); e == nil {
		t.Fatal("nonhost changed controls")
	}
	*now = now.Add(time.Hour)
	if _, e = s.joinRoom(ctx, actor("alice"), r, "signal_join_meeting"); e != nil {
		t.Fatal(e)
	}
	r, _, _ = load[Room](ctx, s.Store, "room:"+id)
	if _, e = s.updateMeeting(ctx, actor("alice"), r, req(map[string]any{"onlyHostShareScreen": true, "isMuteAllMicrophone": true})); e != nil {
		t.Fatal(e)
	}
	r, _, _ = load[Room](ctx, s.Store, "room:"+id)
	if contains(r.sources("bob"), "screen_share") || contains(r.sources("bob"), "microphone") {
		t.Fatal("publish permissions not enforced")
	}
	if _, e = s.joinRoom(ctx, actor("bob"), r, "signal_join_meeting"); e != nil {
		t.Fatal(e)
	}
	r, _, _ = load[Room](ctx, s.Store, "room:"+id)
	if _, e = s.operateStream(ctx, actor("bob"), r, req(map[string]any{"streamType": "audio", "mute": false})); e == nil {
		t.Fatal("participant bypassed host lock")
	}
}
func TestArtifactAnonymousLatestForceChecksumAndNoDowngrade(t *testing.T) {
	s, _, _, _ := fixture(t)
	body := map[string]any{"platform": "android", "version": "2.0.0", "build": 20, "url": "https://test.invalid/app.apk", "sha256": strings.Repeat("ab", 32), "size": 1234, "force": true, "text": "new"}
	if w := post(s, "bob", "/enterprise/v1/application/publish", body); w.Code != 403 {
		t.Fatal("nonadmin published")
	}
	mustData(t, post(s, "admin", "/enterprise/v1/application/publish", body))
	d := mustData(t, post(s, "", "/enterprise/v1/application/latest", map[string]any{"platform": "android", "version": "1.0.0", "build": 1}))
	if d["available"] != true || d["force"] != true || d["sha256"] != strings.Repeat("ab", 32) {
		t.Fatalf("invalid update metadata: %+v", d)
	}
	d = mustData(t, post(s, "", "/enterprise/v1/application/latest", map[string]any{"platform": "android", "version": "3.0.0", "build": 30}))
	if d["available"] != false {
		t.Fatal("downgrade offered")
	}
	body["sha256"] = strings.Repeat("cd", 32)
	if w := post(s, "admin", "/enterprise/v1/application/publish", body); w.Code != 409 {
		t.Fatal("immutable checksum replaced")
	}
	s.Catalog.(*testCatalog).fail = true
	d = mustData(t, post(s, "", "/enterprise/v1/application/latest", map[string]any{"platform": "android", "version": "1.0.0", "build": 1}))
	if d["available"] != false {
		t.Fatal("catalog outage blocks login")
	}
}
func TestEventsBootstrapAndIdempotentOutbox(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := context.Background()
	for j := 0; j < 250; j++ {
		if e := s.appendEvent(ctx, "bob", fmt.Sprint(j), "OnReceiveNewInvitation", map[string]any{"roomID": "old"}); e != nil {
			t.Fatal(e)
		}
	}
	v, e := s.events(ctx, actor("bob"), req(map[string]any{"after": -1}))
	if e != nil {
		t.Fatal(e)
	}
	if len(v.(map[string]any)["events"].([]Event)) != 0 || v.(map[string]any)["cursor"] != int64(250) {
		t.Fatalf("bootstrap replayed history: %+v", v)
	}
	if e = s.appendEvent(ctx, "bob", "249", "OnReceiveNewInvitation", nil); e != nil {
		t.Fatal(e)
	}
	stream, _, _ := load[eventStream](ctx, s.Store, "events:bob")
	if stream.Cursor != 250 {
		t.Fatal("duplicate outbox incremented cursor")
	}
}

func TestConcurrentAcceptanceAcrossDifferentCallsUsesAccountReservation(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := context.Background()
	v, e := s.invite(ctx, actor("alice"), callInvitation())
	if e != nil {
		t.Fatal(e)
	}
	first := v.(map[string]any)["roomID"].(string)
	v, e = s.invite(ctx, actor("carol"), req(map[string]any{"invitation": map[string]any{"inviterUserID": "carol", "inviteeUserIDList": []string{"bob"}, "sessionType": 1, "mediaType": "audio", "timeout": 60}}))
	if e != nil {
		t.Fatal(e)
	}
	second := v.(map[string]any)["roomID"].(string)
	var wg sync.WaitGroup
	success := make(chan bool, 2)
	for _, id := range []string{first, second} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			r, _, err := load[Room](ctx, s.Store, "room:"+id)
			if err != nil {
				success <- false
				return
			}
			_, err = s.joinRoom(ctx, actor("bob"), r, "accept")
			success <- err == nil
		}(id)
	}
	wg.Wait()
	close(success)
	n := 0
	for ok := range success {
		if ok {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("accepted %d simultaneous calls", n)
	}
}
func TestGroupCallCanBeJoinedByCurrentMemberAndSurvivesCallerLeaving(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := context.Background()
	v, e := s.invite(ctx, actor("alice"), req(map[string]any{"invitation": map[string]any{"inviterUserID": "alice", "inviteeUserIDList": []string{"bob"}, "groupID": "g", "sessionType": 3, "mediaType": "video", "timeout": 60}}))
	if e != nil {
		t.Fatal(e)
	}
	id := v.(map[string]any)["roomID"].(string)
	r, _, _ := load[Room](ctx, s.Store, "room:"+id)
	if _, e = s.joinRoom(ctx, actor("bob"), r, "accept"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.signaling(ctx, actor("carol"), "/signaling/getTokenByRoomID", req(map[string]any{"roomID": id})); e != nil {
		t.Fatal(e)
	}
	if _, e = s.signaling(ctx, actor("outsider"), "/signaling/getTokenByRoomID", req(map[string]any{"roomID": id})); e == nil {
		t.Fatal("nonmember joined group call")
	}
	if _, e = s.signaling(ctx, actor("alice"), "/signaling/hungUp", req(map[string]any{"roomID": id})); e != nil {
		t.Fatal(e)
	}
	r, _, _ = load[Room](ctx, s.Store, "room:"+id)
	if r.ended() || len(r.Joined) != 2 {
		t.Fatalf("caller ended group call for others: %+v", r)
	}
}
func TestMeetingCardInvitationMustBeRegisteredAndHonorsHostOnly(t *testing.T) {
	s, _, _, now := fixture(t)
	ctx := context.Background()
	v, e := s.createMeeting(ctx, actor("alice"), req(map[string]any{"meetingName": "invites", "startTime": now.Unix(), "meetingDuration": 3600}))
	if e != nil {
		t.Fatal(e)
	}
	id := v.(map[string]any)["roomID"].(string)
	if _, e = s.signaling(ctx, actor("bob"), "/signaling/signal_join_meeting", req(map[string]any{"roomID": id})); e == nil {
		t.Fatal("unknown meeting guessed by ID")
	}
	if _, e = s.signaling(ctx, actor("alice"), "/signaling/signal_invite_meeting", req(map[string]any{"roomID": id, "userIDs": []string{"bob"}})); e != nil {
		t.Fatal(e)
	}
	if _, e = s.signaling(ctx, actor("bob"), "/signaling/signal_join_meeting", req(map[string]any{"roomID": id})); e != nil {
		t.Fatal(e)
	}
	r, _, _ := load[Room](ctx, s.Store, "room:"+id)
	if _, e = s.updateMeeting(ctx, actor("alice"), r, req(map[string]any{"onlyHostInviteUser": true})); e != nil {
		t.Fatal(e)
	}
	if _, e = s.signaling(ctx, actor("bob"), "/signaling/signal_invite_meeting", req(map[string]any{"roomID": id, "userIDs": []string{"carol"}})); e == nil {
		t.Fatal("nonhost bypassed invitation restriction")
	}
}
func TestAdminRecordsAreGlobalButDeletingDoesNotEndActiveRoom(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := context.Background()
	v, e := s.invite(ctx, actor("alice"), callInvitation())
	if e != nil {
		t.Fatal(e)
	}
	id := v.(map[string]any)["roomID"].(string)
	if w := post(s, "carol", "/enterprise/v1/admin/rtc/get_signal_invitation_records", map[string]any{}); w.Code != 403 {
		t.Fatal("ordinary user accessed admin records")
	}
	data := mustData(t, post(s, "admin", "/enterprise/v1/admin/rtc/get_signal_invitation_records", map[string]any{}))
	if data["total"] != float64(1) {
		t.Fatalf("admin not global: %+v", data)
	}
	mustData(t, post(s, "admin", "/enterprise/v1/admin/rtc/delete_signal_records", map[string]any{"sIDs": []string{id}}))
	r, _, _ := load[Room](ctx, s.Store, "room:"+id)
	if r.ended() {
		t.Fatal("deleting call history closed room")
	}
	data = mustData(t, post(s, "admin", "/enterprise/v1/admin/rtc/get_signal_invitation_records", map[string]any{}))
	if data["total"] != float64(0) {
		t.Fatal("deleted admin record remains listed")
	}
}
