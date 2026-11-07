package im

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func Bind(ctx context.Context, group *ghttp.RouterGroup) {
	group.Group("/api/admin/im", func(api *ghttp.RouterGroup) {
		api.Middleware(func(r *ghttp.Request) {
			if core.UID(r.Context()) <= 0 {
				r.Response.WriteStatus(http.StatusUnauthorized)
				return
			}
			r.Middleware.Next()
		})
		api.GET("/summary", summary)
		bindDirectory(api)
		api.GET("/users", searchUsers)
		api.GET("/manager/summary", managerSummary)
		api.GET("/manager/message-trend", managerTrend)
		api.GET("/manager/top-senders", managerTopSenders)
		api.GET("/private/messages", listPrivate)
		api.POST("/private/messages", sendPrivate)
		api.GET("/groups", listGroups)
		api.POST("/groups", createGroup)
		api.GET("/groups/:group_id/messages", listGroup)
		api.POST("/groups/:group_id/messages", sendGroup)
		api.POST("/groups/:group_id/join", joinGroup)
		api.GET("/groups/:group_id/requests", groupRequests)
		api.POST("/groups/:group_id/requests/:id/review", reviewGroup)
		api.GET("/groups/:group_id/members", groupMembers)
		api.GET("/channels", listChannels)
		api.POST("/channels", createChannel)
		api.POST("/channels/:channel_id/join", joinChannel)
		api.GET("/channels/:channel_id/messages", listChannel)
		api.POST("/channels/:channel_id/messages", sendChannel)
		api.GET("/friends", friends)
		api.POST("/friends", retiredFriends)
		api.GET("/friend-requests", friendRequests)
		api.POST("/friend-requests", retiredFriends)
		api.POST("/friend-requests/:id/review", retiredFriends)
		api.GET("/conversations", conversations)
		api.POST("/conversations/read", readConversation)
		api.GET("/face-packs", facePacks)
		api.POST("/face-packs", createFacePack)
		api.GET("/sensitive-words", sensitiveWords)
		api.POST("/sensitive-words", createSensitiveWord)
		api.GET("/rtc/config", rtcConfig)
		api.POST("/rtc/token", rtcToken)
		api.GET("/ws", websocket)
		api.POST("/media/upload", uploadMedia)
		bindControls(api)
	})
}

func reply(r *ghttp.Request, data any, err error) {
	if err == nil {
		r.Response.WriteJson(gf.Success().SetData(data))
		return
	}
	status := http.StatusInternalServerError
	message := "IM operation failed"
	switch {
	case errors.Is(err, ErrGone):
		status = http.StatusGone
		message = err.Error()
	case errors.Is(err, ErrForbidden):
		status = http.StatusForbidden
		message = err.Error()
	case errors.Is(err, ErrInvalid):
		status = http.StatusBadRequest
		message = err.Error()
	case errors.Is(err, ErrConflict):
		status = http.StatusConflict
		message = err.Error()
	}
	r.Response.WriteStatus(status)
	r.Response.WriteJson(gf.Failed().SetMsg(message))
}
func parse(r *ghttp.Request, v any) bool {
	if err := r.Parse(v); err != nil {
		reply(r, nil, ErrInvalid)
		return false
	}
	return true
}
func listMessages(r *ghttp.Request, kind, target string) {
	rows, err := Messages(r.Context(), core.UID(r.Context()), kind, target, r.Get("after_seq").Int64())
	replyProjected(r, rows, "message", err)
}
func replyProjected(r *ghttp.Request, rows gdb.Result, kind string, err error) {
	if err != nil {
		reply(r, nil, err)
		return
	}
	out := []g.Map{}
	for _, row := range rows {
		m := g.Map(row.Map())
		if kind == "message" {
			delete(m, "request_hash")
		}
		out = append(out, m)
	}
	err = decorateRows(r.Context(), out, kind)
	reply(r, out, err)
}
func listPrivate(r *ghttp.Request) { listMessages(r, "private", r.Get("receiver_id").String()) }
func listGroup(r *ghttp.Request)   { listMessages(r, "group", r.GetRouter("group_id").String()) }
func listChannel(r *ghttp.Request) { listMessages(r, "channel", r.GetRouter("channel_id").String()) }
func sendMessages(r *ghttp.Request, kind, target string) {
	var in MessageInput
	if !parse(r, &in) {
		return
	}
	in.ConversationType = kind
	in.ConversationID = target
	out, err := SendMessage(r.Context(), core.UID(r.Context()), in)
	reply(r, out, err)
}
func sendPrivate(r *ghttp.Request) { sendMessages(r, "private", "") }
func sendGroup(r *ghttp.Request)   { sendMessages(r, "group", r.GetRouter("group_id").String()) }
func sendChannel(r *ghttp.Request) { sendMessages(r, "channel", r.GetRouter("channel_id").String()) }
func createSpace(r *ghttp.Request, kind string) {
	var in struct {
		Name         string   `json:"name"`
		Notice       string   `json:"notice"`
		Description  string   `json:"description"`
		JoinApproval *int     `json:"join_approval"`
		MemberIDs    *[]int64 `json:"member_ids"`
	}
	if !parse(r, &in) {
		return
	}
	description := in.Description
	if kind == "group" {
		description = in.Notice
	}
	approval := 1
	if in.JoinApproval != nil {
		approval = *in.JoinApproval
	}
	var out g.Map
	var err error
	if kind == "group" && in.MemberIDs != nil {
		out, err = CreateGroupWithMembers(r.Context(), core.UID(r.Context()), in.Name, description, approval, *in.MemberIDs)
	} else {
		out, err = CreateSpace(r.Context(), core.UID(r.Context()), kind, in.Name, description, approval)
	}
	reply(r, out, err)
}
func createGroup(r *ghttp.Request)   { createSpace(r, "group") }
func createChannel(r *ghttp.Request) { createSpace(r, "channel") }
func listSpaces(r *ghttp.Request, kind string) {
	// Discovery reveals only public names, never membership or message content.
	rows, err := model(r.Context(), "biz_im_"+kind).Where("status", "active").OrderDesc("updated_at").Limit(200).All()
	replyProjected(r, rows, "space", err)
}
func listGroups(r *ghttp.Request)   { listSpaces(r, "group") }
func listChannels(r *ghttp.Request) { listSpaces(r, "channel") }
func joinGroup(r *ghttp.Request) {
	out, err := JoinSpace(r.Context(), core.UID(r.Context()), "group", r.GetRouter("group_id").String())
	reply(r, out, err)
}
func joinChannel(r *ghttp.Request) {
	out, err := JoinSpace(r.Context(), core.UID(r.Context()), "channel", r.GetRouter("channel_id").String())
	reply(r, out, err)
}
func groupRequests(r *ghttp.Request) {
	id := r.GetRouter("group_id").String()
	if err := manager(r.Context(), core.UID(r.Context()), "group", id); err != nil {
		reply(r, nil, err)
		return
	}
	rows, err := model(r.Context(), "biz_im_group_request").Where("group_id", id).OrderDesc("updated_at").Limit(200).All()
	replyProjected(r, rows, "member", err)
}
func groupMembers(r *ghttp.Request) {
	id := r.GetRouter("group_id").String()
	if _, err := participants(r.Context(), core.UID(r.Context()), "group", id); err != nil {
		reply(r, nil, err)
		return
	}
	rows, err := model(r.Context(), "biz_im_group_member").Where(g.Map{"group_id": id, "status": "active"}).All()
	replyProjected(r, rows, "member", err)
}
func reviewGroup(r *ghttp.Request) {
	var in struct {
		Approve bool `json:"approve"`
	}
	if !parse(r, &in) {
		return
	}
	reply(r, nil, ReviewGroupRequest(r.Context(), core.UID(r.Context()), r.GetRouter("group_id").String(), r.GetRouter("id").String(), in.Approve))
}
func friends(r *ghttp.Request) {
	rows, err := model(r.Context(), "biz_im_friend").Where(g.Map{"user_id": core.UID(r.Context()), "status": "active"}).OrderDesc("updated_at").All()
	reply(r, rows, err)
}
func requestFriend(r *ghttp.Request) {
	var in struct {
		ReceiverID   int64  `json:"receiver_id"`
		FriendUserID int64  `json:"friend_user_id"`
		Message      string `json:"message"`
	}
	if !parse(r, &in) {
		return
	}
	if in.ReceiverID == 0 {
		in.ReceiverID = in.FriendUserID
	}
	out, err := RequestFriend(r.Context(), core.UID(r.Context()), in.ReceiverID, in.Message)
	reply(r, out, err)
}
func friendRequests(r *ghttp.Request) {
	user := core.UID(r.Context())
	rows, err := model(r.Context(), "biz_im_friend_request").Where("sender_id = ? OR receiver_id = ?", user, user).OrderDesc("updated_at").Limit(200).All()
	reply(r, rows, err)
}
func reviewFriend(r *ghttp.Request) {
	var in struct {
		Approve bool `json:"approve"`
	}
	if !parse(r, &in) {
		return
	}
	reply(r, nil, ReviewFriendRequest(r.Context(), core.UID(r.Context()), r.GetRouter("id").String(), in.Approve))
}
func conversations(r *ghttp.Request) {
	rows, err := visibleConversations(r.Context(), core.UID(r.Context()), r.Get("after_seq").Int64())
	replyProjected(r, rows, "conversation", err)
}
func readConversation(r *ghttp.Request) {
	var in struct {
		ConversationType string `json:"conversation_type"`
		ConversationID   string `json:"conversation_id"`
		LastSeq          int64  `json:"last_seq"`
	}
	if !parse(r, &in) {
		return
	}
	reply(r, nil, MarkRead(r.Context(), core.UID(r.Context()), in.ConversationType, in.ConversationID, in.LastSeq))
}
func facePacks(r *ghttp.Request) {
	rows, err := model(r.Context(), "biz_im_face_pack").Where("status", "active").Limit(200).All()
	reply(r, rows, err)
}
func validMediaURL(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && ((u.Scheme == "https" && u.Host != "") || (u.Scheme == "" && strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//")))
}
func createFacePack(r *ghttp.Request) {
	var in struct {
		Name     string `json:"name"`
		CoverURL string `json:"cover_url"`
		Items    []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"items"`
	}
	if !parse(r, &in) {
		return
	}
	if in.Name == "" || len(in.Name) > 160 || len(in.Items) == 0 || len(in.Items) > 200 {
		reply(r, nil, ErrInvalid)
		return
	}
	for _, item := range in.Items {
		if !validMediaURL(item.URL) {
			reply(r, nil, ErrInvalid)
			return
		}
	}
	if in.CoverURL != "" && !validMediaURL(in.CoverURL) {
		reply(r, nil, ErrInvalid)
		return
	}
	items, _ := json.Marshal(in.Items)
	data := g.Map{"id": guid.S(), "owner_id": core.UID(r.Context()), "name": in.Name, "cover_url": in.CoverURL, "items": string(items), "status": "active", "created_at": core.Now(), "updated_at": core.Now()}
	_, err := model(r.Context(), "biz_im_face_pack").Data(data).Insert()
	reply(r, data, err)
}
func sensitiveWords(r *ghttp.Request) {
	rows, err := model(r.Context(), "biz_im_sensitive_word").Where("status", "active").Limit(1000).All()
	reply(r, rows, err)
}
func createSensitiveWord(r *ghttp.Request) {
	var in struct {
		Word        string `json:"word"`
		Replacement string `json:"replacement"`
	}
	if !parse(r, &in) {
		return
	}
	if in.Word == "" || len(in.Word) > 160 || len(in.Replacement) > 160 {
		reply(r, nil, ErrInvalid)
		return
	}
	data := g.Map{"id": guid.S(), "word": in.Word, "replacement": in.Replacement, "status": "active", "created_at": core.Now(), "updated_at": core.Now()}
	_, err := model(r.Context(), "biz_im_sensitive_word").Data(data).Insert()
	reply(r, data, err)
}
func summary(r *ghttp.Request) {
	user := core.UID(r.Context())
	out := g.Map{}
	for _, kind := range []string{"private", "group", "channel"} {
		n, err := model(r.Context(), "biz_im_message").Where(g.Map{"sender_id": user, "conversation_type": kind}).Count()
		if err != nil {
			reply(r, nil, err)
			return
		}
		out[kind+"_messages"] = n
	}
	n, err := model(r.Context(), "biz_im_friend").Where(g.Map{"user_id": user, "status": "active"}).Count()
	out["friends"] = n
	out["user_id"] = strconv.FormatInt(user, 10)
	reply(r, out, err)
}

func managerSummary(r *ghttp.Request) {
	out := g.Map{}
	for _, kind := range []string{"private", "group", "channel"} {
		n, err := model(r.Context(), "biz_im_message").Where("conversation_type", kind).Count()
		if err != nil {
			reply(r, nil, err)
			return
		}
		out[kind+"_messages"] = n
	}
	for key, table := range map[string]string{"groups": "biz_im_group", "channels": "biz_im_channel", "friends": "biz_im_friend", "rtc_calls": "biz_im_rtc_call"} {
		n, err := model(r.Context(), table).Count()
		if err != nil {
			reply(r, nil, err)
			return
		}
		out[key] = n
	}
	reply(r, out, nil)
}
func managerTrend(r *ghttp.Request) {
	rows, err := model(r.Context(), "biz_im_message").Fields("SUBSTR(send_time,1,10) AS day, COUNT(*) AS messages").Group("SUBSTR(send_time,1,10)").OrderDesc("day").Limit(90).All()
	reply(r, rows, err)
}
func managerTopSenders(r *ghttp.Request) {
	rows, err := model(r.Context(), "biz_im_message").Fields("sender_id, COUNT(*) AS messages").Group("sender_id").OrderDesc("messages").Limit(20).All()
	reply(r, rows, err)
}

func searchUsers(r *ghttp.Request) {
	query := strings.TrimSpace(r.Get("q").String())
	if query == "" {
		reply(r, []any{}, nil)
		return
	}
	if len(query) > 64 {
		reply(r, nil, ErrInvalid)
		return
	}
	d, err := loadDirectory(r.Context())
	if err == nil {
		err = d.personalized(r.Context(), core.UID(r.Context()))
	}
	if err != nil {
		reply(r, nil, err)
		return
	}
	out := []*directoryPerson{}
	q := strings.ToLower(query)
	for _, p := range d.people {
		if p.listed && p.CanChat && (strings.Contains(strings.ToLower(p.Name), q) || strings.Contains(strings.ToLower(p.EmployeeNo), q)) {
			out = append(out, p)
			if len(out) == 20 {
				break
			}
		}
	}
	reply(r, out, nil)
}
