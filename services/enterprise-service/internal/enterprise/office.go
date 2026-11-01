package enterprise

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

type Comment struct {
	CommentID     string `json:"commentID"`
	UserID        string `json:"userID"`
	Nickname      string `json:"nickname"`
	FaceURL       string `json:"faceURL"`
	ReplyUserID   string `json:"replyUserID"`
	ReplyNickname string `json:"replyNickname"`
	ReplyFaceURL  string `json:"replyFaceURL"`
	Content       string `json:"content"`
	CreateTime    int64  `json:"createTime"`
}
type Like struct {
	User
	LikeTime int64 `json:"likeTime"`
}
type Moment struct {
	WorkMomentID    string          `json:"workMomentID"`
	UserID          string          `json:"userID"`
	Nickname        string          `json:"nickname"`
	FaceURL         string          `json:"faceURL"`
	Content         json.RawMessage `json:"content"`
	LikeUsers       []Like          `json:"likeUsers"`
	Comments        []Comment       `json:"comments"`
	Permission      int             `json:"permission"`
	PermissionUsers []User          `json:"permissionUsers"`
	AtUsers         []User          `json:"atUsers"`
	CreateTime      int64           `json:"createTime"`
}

func (m Moment) visible(user string) bool {
	if user == m.UserID {
		return true
	}
	allowed := false
	for _, u := range m.PermissionUsers {
		if u.UserID == user {
			allowed = true
			break
		}
	}
	switch m.Permission {
	case 0:
		return true
	case 1:
		return false
	case 2:
		return allowed
	case 3:
		return !allowed
	}
	return false
}

type MomentLog struct {
	Moment
	Type      int    `json:"type"`
	EventID   string `json:"eventID"`
	EventTime int64  `json:"eventTime"`
}
type momentWatermark struct {
	CountTime int64 `json:"countTime"`
	ListTime  int64 `json:"listTime"`
}
type Tag struct {
	TagID      string   `json:"tagID"`
	TagName    string   `json:"tagName"`
	UserID     string   `json:"userID"`
	UserIDs    []string `json:"userIDs"`
	GroupIDs   []string `json:"groupIDs"`
	Users      []User   `json:"users"`
	Groups     []Group  `json:"groups"`
	CreateTime int64    `json:"createTime"`
}
type Delivery struct {
	State       string `json:"state"`
	ClientMsgID string `json:"clientMsgID,omitempty"`
	Error       string `json:"error,omitempty"`
}
type TagSendLog struct {
	ID       string              `json:"id"`
	Owner    string              `json:"userID"`
	Tags     []Tag               `json:"tags"`
	Users    []User              `json:"users"`
	GroupIDs []string            `json:"groupIDs"`
	Content  string              `json:"content"`
	SendTime int64               `json:"sendTime"`
	Delivery map[string]Delivery `json:"delivery"`
}

func (s *Server) user(ctx context.Context, id string) (User, error) {
	us, err := s.IM.Users(ctx, []string{id})
	if err != nil {
		return User{}, err
	}
	for _, u := range us {
		if u.UserID == id {
			return u, nil
		}
	}
	return User{}, missing("user not found")
}
func (s *Server) checkedUsers(ctx context.Context, ids []string) ([]User, error) {
	ids = unique(ids)
	us, err := s.IM.Users(ctx, ids)
	if err != nil {
		return nil, err
	}
	if len(us) != len(ids) {
		return nil, bad("one or more users do not exist")
	}
	return us, nil
}
func (s *Server) expandGroups(ctx context.Context, a Actor, groups []string) ([]string, error) {
	ids := []string{}
	for _, g := range unique(groups) {
		us, err := s.IM.GroupMembers(ctx, g)
		if err != nil {
			return nil, err
		}
		member := false
		for _, u := range us {
			if u.UserID == a.UserID {
				member = true
			}
			ids = append(ids, u.UserID)
		}
		if !member {
			return nil, forbidden("group membership required")
		}
	}
	return unique(ids), nil
}
func (s *Server) office(ctx context.Context, a Actor, path string, q request) (any, error) {
	if strings.HasPrefix(path, "/office/tag/") {
		return s.tags(ctx, a, strings.TrimPrefix(path, "/office/tag/"), q)
	}
	action := strings.TrimPrefix(path, "/office/work_moment/")
	switch action {
	case "add":
		if err := checkActor(a, q.str("userID")); err != nil {
			return nil, err
		}
		permission := int(q.num("permission"))
		if permission < 0 || permission > 3 {
			return nil, bad("invalid permission")
		}
		var c struct {
			Text  string            `json:"text"`
			Metas []json.RawMessage `json:"metas"`
			Type  int               `json:"type"`
		}
		if err := q.decode("content", &c); err != nil {
			return nil, err
		}
		if strings.TrimSpace(c.Text) == "" && len(c.Metas) == 0 {
			return nil, bad("content is empty")
		}
		if len(c.Text) > 10000 || len(c.Metas) > 9 {
			return nil, bad("moment content too large")
		}
		u, err := s.user(ctx, a.UserID)
		if err != nil {
			return nil, err
		}
		ids := q.strings("permissionUserIDs")
		groupIDs, err := s.expandGroups(ctx, a, q.strings("permissionGroupIDs"))
		if err != nil {
			return nil, err
		}
		ids = unique(append(ids, groupIDs...))
		if permission == 2 {
			ids = unique(append(ids, q.strings("atUserIDs")...))
		}
		if (permission == 2 || permission == 3) && len(ids) == 0 {
			return nil, bad("visibility audience is empty")
		}
		if permission < 2 {
			ids = []string{}
		}
		audience, err := s.checkedUsers(ctx, ids)
		if err != nil {
			return nil, err
		}
		at, err := s.checkedUsers(ctx, q.strings("atUserIDs"))
		if err != nil {
			return nil, err
		}
		m := Moment{WorkMomentID: newID(), UserID: a.UserID, Nickname: u.Nickname, FaceURL: u.FaceURL, Content: q["content"], LikeUsers: []Like{}, Comments: []Comment{}, Permission: permission, PermissionUsers: audience, AtUsers: at, CreateTime: s.now().UnixMilli()}
		if err = s.Store.Save(ctx, document("moment:"+m.WorkMomentID, "moment", a.UserID, m), 0); err != nil {
			return nil, err
		}
		s.momentNotify(ctx, m, newID(), 2, a.UserID)
		return map[string]any{"workMoment": m}, nil
	case "find/send", "find/recv":
		ds, err := s.Store.List(ctx, "moment", "")
		if err != nil {
			return nil, err
		}
		ms := []Moment{}
		target := q.str("userID")
		if target == "" {
			target = a.UserID
		}
		friends, err := s.IM.Friends(ctx, a.UserID)
		if err != nil {
			return nil, err
		}
		for _, d := range ds {
			var m Moment
			if json.Unmarshal(d.Data, &m) != nil {
				continue
			}
			if !m.visible(a.UserID) {
				continue
			}
			if action == "find/send" && m.UserID != target {
				continue
			}
			if action == "find/recv" && m.UserID != a.UserID && !contains(friends, m.UserID) {
				continue
			}
			ms = append(ms, m)
		}
		sort.Slice(ms, func(i, j int) bool { return ms[i].CreateTime > ms[j].CreateTime })
		return map[string]any{"workMoments": page(ms, q)}, nil
	case "unread/count", "logs", "unread/clear":
		return s.momentLogs(ctx, a, action, q)
	}
	id := q.str("workMomentID")
	if id == "" {
		id = q.str("WorkMomentID")
	}
	m, d, err := load[Moment](ctx, s.Store, "moment:"+id)
	if err != nil {
		return nil, err
	}
	if !m.visible(a.UserID) {
		return nil, forbidden("moment is not visible")
	}
	switch action {
	case "get":
		return map[string]any{"workMoment": m}, nil
	case "del":
		if a.UserID != m.UserID {
			return nil, forbidden("only author can delete moment")
		}
		return map[string]any{"userIDs": []string{m.UserID}}, s.Store.Delete(ctx, d.ID, d.Version)
	case "like":
		u, err := s.user(ctx, a.UserID)
		if err != nil {
			return nil, err
		}
		want := q.boolean("like")
		changed := false
		m, err = mutate[Moment](ctx, s.Store, d.ID, func(m *Moment) error {
			if !m.visible(a.UserID) {
				return forbidden("moment is not visible")
			}
			changed = false
			at := -1
			for i, l := range m.LikeUsers {
				if l.UserID == a.UserID {
					at = i
				}
			}
			if at >= 0 && !want {
				m.LikeUsers = append(m.LikeUsers[:at], m.LikeUsers[at+1:]...)
				changed = true
			}
			if at < 0 && want {
				m.LikeUsers = append(m.LikeUsers, Like{u, s.now().UnixMilli()})
				changed = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if changed && want {
			s.momentNotify(ctx, m, newID(), 1, a.UserID)
		}
		return map[string]any{"workMoment": m}, nil
	case "comment/add":
		text := strings.TrimSpace(q.str("content"))
		if text == "" || len(text) > 5000 {
			return nil, bad("comment must contain 1-5000 bytes")
		}
		u, err := s.user(ctx, a.UserID)
		if err != nil {
			return nil, err
		}
		c := Comment{CommentID: newID(), UserID: a.UserID, Nickname: u.Nickname, FaceURL: u.FaceURL, Content: text, ReplyUserID: q.str("replyUserID"), CreateTime: s.now().UnixMilli()}
		if c.ReplyUserID != "" {
			reply, err := s.user(ctx, c.ReplyUserID)
			if err != nil {
				return nil, err
			}
			c.ReplyNickname = reply.Nickname
			c.ReplyFaceURL = reply.FaceURL
		}
		m, err = mutate[Moment](ctx, s.Store, d.ID, func(m *Moment) error {
			if !m.visible(a.UserID) {
				return forbidden("moment is not visible")
			}
			if c.ReplyUserID != "" {
				found := false
				for _, old := range m.Comments {
					if old.UserID == c.ReplyUserID {
						found = true
					}
				}
				if !found {
					return bad("reply user has not commented")
				}
			}
			m.Comments = append(m.Comments, c)
			return nil
		})
		if err != nil {
			return nil, err
		}
		s.momentNotify(ctx, m, c.CommentID, 3, a.UserID)
		return map[string]any{"commentID": c.CommentID, "workMoment": m}, nil
	case "comment/del":
		m, err = mutate[Moment](ctx, s.Store, d.ID, func(m *Moment) error {
			for i, c := range m.Comments {
				if c.CommentID == q.str("commentID") {
					if c.UserID != a.UserID && m.UserID != a.UserID {
						return forbidden("only comment or moment author can delete")
					}
					m.Comments = append(m.Comments[:i], m.Comments[i+1:]...)
					return nil
				}
			}
			return missing("comment not found")
		})
		return map[string]any{"workMoment": m, "userIDs": []string{m.UserID}}, err
	default:
		return nil, missing("unknown moment operation")
	}
}
func (s *Server) momentNotify(ctx context.Context, m Moment, key string, kind int, actor string) {
	ids := []string{m.UserID}
	for _, c := range m.Comments {
		ids = append(ids, c.UserID)
	}
	for _, u := range m.AtUsers {
		ids = append(ids, u.UserID)
	}
	for _, u := range unique(ids) {
		if u == actor || !m.visible(u) {
			continue
		}
		log := MomentLog{Moment: m, Type: kind, EventID: key, EventTime: s.now().UnixMilli()}
		err := s.Store.Save(ctx, document("moment_log:"+u+":"+key, "moment_log", u, log), 0)
		if err != nil && !errors.Is(err, ErrConflict) {
			continue
		}
		_ = s.appendEvent(ctx, u, key, "OnWorkMomentChanged", map[string]any{"workMomentID": m.WorkMomentID, "type": kind})
		s.queueHint(Actor{UserID: actor, Platform: 5}, u, key, "OnWorkMomentChanged")
	}
}
func (s *Server) momentLogs(ctx context.Context, a Actor, action string, q request) (any, error) {
	watermark, doc, err := load[momentWatermark](ctx, s.Store, "moment_watermark:"+a.UserID)
	if errors.Is(err, ErrNotFound) {
		doc = document("moment_watermark:"+a.UserID, "moment_watermark", a.UserID, watermark)
	} else if err != nil {
		return nil, err
	}
	if action == "unread/clear" {
		kind := q.num("type")
		if kind < 1 || kind > 3 {
			return nil, bad("type must be 1 (count), 2 (list), or 3 (both)")
		}
		if kind == 1 || kind == 3 {
			watermark.CountTime = s.now().UnixMilli()
		}
		if kind == 2 || kind == 3 {
			watermark.ListTime = s.now().UnixMilli()
		}
		doc.Data, _ = json.Marshal(watermark)
		return map[string]any{}, s.Store.Save(ctx, doc, doc.Version)
	}
	ds, err := s.Store.List(ctx, "moment_log", a.UserID)
	if err != nil {
		return nil, err
	}
	logs := []MomentLog{}
	total := 0
	for _, d := range ds {
		var log MomentLog
		if json.Unmarshal(d.Data, &log) != nil {
			continue
		}
		m, _, e := load[Moment](ctx, s.Store, "moment:"+log.WorkMomentID)
		if e != nil || !m.visible(a.UserID) {
			continue
		}
		log.Moment = m
		if log.EventTime > watermark.ListTime {
			logs = append(logs, log)
		}
		if log.EventTime > watermark.CountTime {
			total++
		}
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].EventTime > logs[j].EventTime })
	if action == "unread/count" {
		return map[string]any{"total": total}, nil
	}
	return map[string]any{"workMoments": page(logs, q)}, nil
}
func (s *Server) hydrateTag(ctx context.Context, t Tag) (Tag, error) {
	us, err := s.IM.Users(ctx, t.UserIDs)
	if err != nil {
		return t, err
	}
	t.Users = us
	t.Groups = []Group{}
	if resolver, ok := s.IM.(GroupResolver); ok {
		t.Groups, err = resolver.Groups(ctx, t.GroupIDs)
		if err != nil {
			return t, err
		}
	} else {
		for _, g := range t.GroupIDs {
			t.Groups = append(t.Groups, Group{GroupID: g, GroupName: g})
		}
	}
	return t, nil
}
func (s *Server) tags(ctx context.Context, a Actor, action string, q request) (any, error) {
	if err := checkActor(a, q.str("userID")); err != nil {
		return nil, err
	}
	switch action {
	case "add":
		name := strings.TrimSpace(q.str("tagName"))
		if name == "" || len(name) > 200 {
			return nil, bad("tag name required, max 200 bytes")
		}
		if _, err := s.checkedUsers(ctx, q.strings("userIDs")); err != nil {
			return nil, err
		}
		if _, err := s.expandGroups(ctx, a, q.strings("groupIDs")); err != nil {
			return nil, err
		}
		t := Tag{TagID: newID(), TagName: name, UserID: a.UserID, UserIDs: q.strings("userIDs"), GroupIDs: q.strings("groupIDs"), CreateTime: s.now().UnixMilli()}
		if err := s.Store.Save(ctx, document("tag:"+t.TagID, "tag", a.UserID, t), 0); err != nil {
			return nil, err
		}
		return map[string]any{"tagID": t.TagID}, nil
	case "find/user":
		ds, err := s.Store.List(ctx, "tag", a.UserID)
		if err != nil {
			return nil, err
		}
		sortedDocs(ds)
		tags := []Tag{}
		for _, d := range ds {
			var t Tag
			if err = json.Unmarshal(d.Data, &t); err != nil {
				return nil, err
			}
			t, err = s.hydrateTag(ctx, t)
			if err != nil {
				return nil, err
			}
			tags = append(tags, t)
		}
		return map[string]any{"tags": tags}, nil
	case "send":
		return s.sendTag(ctx, a, q)
	case "send/log":
		ds, err := s.Store.List(ctx, "tag_send", a.UserID)
		if err != nil {
			return nil, err
		}
		sortedDocs(ds)
		logs := []TagSendLog{}
		for _, d := range ds {
			var l TagSendLog
			if json.Unmarshal(d.Data, &l) == nil {
				logs = append(logs, l)
			}
		}
		return map[string]any{"tagSendLogs": page(logs, q)}, nil
	case "send/log/del":
		for _, id := range q.strings("ids") {
			_, d, err := load[TagSendLog](ctx, s.Store, "tag_send:"+id)
			if err != nil {
				return nil, err
			}
			if d.Owner != a.UserID {
				return nil, forbidden("send log owner required")
			}
			if err = s.Store.Delete(ctx, d.ID, d.Version); err != nil {
				return nil, err
			}
		}
		return map[string]any{}, nil
	case "send/retry":
		return s.retryTag(ctx, a, q.str("id"))
	}
	t, d, err := load[Tag](ctx, s.Store, "tag:"+q.str("tagID"))
	if err != nil {
		return nil, err
	}
	if t.UserID != a.UserID {
		return nil, forbidden("tag owner required")
	}
	switch action {
	case "get":
		t, err = s.hydrateTag(ctx, t)
		return map[string]any{"tag": t}, err
	case "del":
		return map[string]any{}, s.Store.Delete(ctx, d.ID, d.Version)
	case "set":
		if _, err = s.checkedUsers(ctx, q.strings("addUserIDs")); err != nil {
			return nil, err
		}
		if _, err = s.expandGroups(ctx, a, q.strings("addGroupIDs")); err != nil {
			return nil, err
		}
		t, err = mutate[Tag](ctx, s.Store, d.ID, func(t *Tag) error {
			if q.has("name") {
				if strings.TrimSpace(q.str("name")) == "" {
					return bad("tag name cannot be empty")
				}
				t.TagName = q.str("name")
			}
			t.UserIDs = without(append(t.UserIDs, q.strings("addUserIDs")...), q.strings("delUserIDs")...)
			t.GroupIDs = without(append(t.GroupIDs, q.strings("addGroupIDs")...), q.strings("delGroupIDs")...)
			return nil
		})
		return map[string]any{"tag": t}, err
	}
	return nil, missing("unknown tag operation")
}
func (s *Server) sendTag(ctx context.Context, a Actor, q request) (any, error) {
	if err := checkActor(a, q.str("sendID")); err != nil {
		return nil, err
	}
	content := q.str("content")
	var custom struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal([]byte(content), &custom); err != nil || strings.TrimSpace(custom.Data) == "" {
		return nil, bad("content must contain CustomElem.data as a non-empty JSON string")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(custom.Data), &payload); err != nil || len(payload) == 0 {
		return nil, bad("CustomElem.data must contain a JSON message object")
	}
	id := q.str("requestID")
	if id == "" {
		id = newID()
	}
	if len(id) > 100 {
		return nil, bad("requestID too long")
	}
	key := "tag_send:" + a.UserID + ":" + id
	if existing, _, e := load[TagSendLog](ctx, s.Store, key); e == nil {
		return map[string]any{"sendLog": existing}, nil
	} else if !errors.Is(e, ErrNotFound) {
		return nil, e
	}
	ids := q.strings("userIDs")
	groups := q.strings("groupIDs")
	tags := []Tag{}
	for _, tid := range q.strings("tagIDs") {
		t, _, err := load[Tag](ctx, s.Store, "tag:"+tid)
		if err != nil {
			return nil, err
		}
		if t.UserID != a.UserID {
			return nil, forbidden("tag owner required")
		}
		tags = append(tags, t)
		ids = append(ids, t.UserIDs...)
		groups = append(groups, t.GroupIDs...)
	}
	gs, err := s.expandGroups(ctx, a, groups)
	if err != nil {
		return nil, err
	}
	ids = without(append(ids, gs...), a.UserID)
	if len(ids) == 0 || len(ids) > 5000 {
		return nil, bad("recipient count must be 1-5000")
	}
	users, err := s.checkedUsers(ctx, ids)
	if err != nil {
		return nil, err
	}
	l := TagSendLog{ID: a.UserID + ":" + id, Owner: a.UserID, Tags: tags, Users: users, GroupIDs: unique(groups), Content: content, SendTime: s.now().UnixMilli(), Delivery: map[string]Delivery{}}
	for _, id := range ids {
		l.Delivery[id] = Delivery{State: "pending"}
	}
	if err = s.Store.Save(ctx, document(key, "tag_send", a.UserID, l), 0); err != nil {
		return nil, err
	}
	return s.deliverTag(ctx, a, key, false)
}
func (s *Server) retryTag(ctx context.Context, a Actor, id string) (any, error) {
	key := "tag_send:" + id
	l, _, err := load[TagSendLog](ctx, s.Store, key)
	if err != nil {
		return nil, err
	}
	if l.Owner != a.UserID {
		return nil, forbidden("send log owner required")
	}
	return s.deliverTag(ctx, a, key, true)
}
func (s *Server) deliverTag(ctx context.Context, a Actor, key string, retry bool) (any, error) {
	l, _, err := load[TagSendLog](ctx, s.Store, key)
	if err != nil {
		return nil, err
	}
	for id, d := range l.Delivery {
		if d.State != "pending" && !(retry && d.State == "failed") {
			continue
		}
		claimed := false
		_, err = mutate[TagSendLog](ctx, s.Store, key, func(l *TagSendLog) error {
			x := l.Delivery[id]
			claimed = x.State == "pending" || (retry && x.State == "failed")
			if claimed {
				l.Delivery[id] = Delivery{State: "inflight"}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if !claimed {
			continue
		}
		msgID, e := s.IM.Send(ctx, a, id, "", l.Content, 110)
		state := Delivery{State: "sent", ClientMsgID: msgID}
		if e != nil {
			state = Delivery{State: "failed", Error: e.Error()}
			var uncertain interface{ Uncertain() bool }
			if errors.As(e, &uncertain) && uncertain.Uncertain() {
				state.State = "uncertain"
			}
		}
		// A crash with inflight stays visible as uncertain; it is never blindly resent.
		l, err = mutate[TagSendLog](ctx, s.Store, key, func(l *TagSendLog) error { l.Delivery[id] = state; return nil })
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{"sendLog": l}, nil
}
