package enterprise

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type User struct {
	UserID    string `json:"userID"`
	Nickname  string `json:"nickname"`
	FaceURL   string `json:"faceURL"`
	JoinTime  int64  `json:"joinTime,omitempty"`
	RoleLevel int    `json:"roleLevel,omitempty"`
}
type Group struct {
	GroupID     string `json:"groupID"`
	GroupName   string `json:"groupName"`
	FaceURL     string `json:"faceURL"`
	OwnerUserID string `json:"ownerUserID"`
}
type GroupResolver interface {
	Groups(context.Context, []string) ([]Group, error)
}
type Message struct {
	ClientMsgID string `json:"clientMsgID"`
	SendID      string `json:"sendID"`
	GroupID     string `json:"groupID"`
	RecvID      string `json:"recvID"`
	Seq         int64  `json:"seq"`
	SendTime    int64  `json:"sendTime"`
	ContentType int    `json:"contentType"`
}
type IM interface {
	Users(context.Context, []string) ([]User, error)
	Friends(context.Context, string) ([]string, error)
	GroupMembers(context.Context, string) ([]User, error)
	Message(context.Context, string, string, int64, string) (Message, error)
	Send(context.Context, Actor, string, string, string, int) (string, error)
}
type Upstream struct {
	IMBase, ChatBase, ChatAdminBase, IMAdminID, IMSecret string
	HTTP                                                 *http.Client
	mu                                                   sync.Mutex
	adminToken                                           string
	tokenExpiry                                          time.Time
}
type upstreamEnvelope struct {
	ErrCode int             `json:"errCode"`
	ErrMsg  string          `json:"errMsg"`
	ErrDlt  string          `json:"errDlt"`
	Data    json.RawMessage `json:"data"`
}

func (u *Upstream) call(ctx context.Context, path, token string, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	r, err := http.NewRequestWithContext(ctx, "POST", strings.TrimSuffix(u.IMBase, "/")+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("token", token)
	r.Header.Set("operationID", newID())
	c := u.HTTP
	if c == nil {
		c = &http.Client{Timeout: 15 * time.Second}
	}
	res, err := c.Do(r)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if err != nil {
		return err
	}
	var e upstreamEnvelope
	if err = json.Unmarshal(raw, &e); err != nil {
		return fmt.Errorf("OpenIM %s invalid response", path)
	}
	if res.StatusCode >= 400 || e.ErrCode != 0 {
		return fmt.Errorf("OpenIM %s rejected (%d): %s", path, e.ErrCode, e.ErrMsg)
	}
	if out != nil && len(e.Data) > 0 {
		return json.Unmarshal(e.Data, out)
	}
	return nil
}
func (u *Upstream) AdminToken(ctx context.Context) (string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.adminToken != "" && time.Now().Before(u.tokenExpiry) {
		return u.adminToken, nil
	}
	var v struct {
		Token             string `json:"token"`
		ExpireTimeSeconds int64  `json:"expireTimeSeconds"`
	}
	err := u.call(ctx, "/auth/get_admin_token", "", map[string]any{"secret": u.IMSecret, "userID": u.IMAdminID}, &v)
	if err != nil {
		return "", err
	}
	if v.Token == "" {
		return "", fmt.Errorf("OpenIM returned empty administrator token")
	}
	u.adminToken = v.Token
	u.tokenExpiry = time.Now().Add(time.Minute * 30)
	return v.Token, nil
}
func (u *Upstream) adminCall(ctx context.Context, path string, body, out any) error {
	token, err := u.AdminToken(ctx)
	if err != nil {
		return err
	}
	return u.call(ctx, path, token, body, out)
}
func (u *Upstream) Users(ctx context.Context, ids []string) ([]User, error) {
	if len(ids) == 0 {
		return []User{}, nil
	}
	var v struct {
		UsersInfo    []User `json:"usersInfo"`
		UserInfoList []User `json:"userInfoList"`
	}
	err := u.adminCall(ctx, "/user/get_users_info", map[string]any{"userIDs": unique(ids)}, &v)
	if v.UsersInfo == nil {
		v.UsersInfo = v.UserInfoList
	}
	return v.UsersInfo, err
}
func (u *Upstream) Friends(ctx context.Context, user string) ([]string, error) {
	var v struct {
		FriendIDs []string `json:"friendIDs"`
	}
	err := u.adminCall(ctx, "/friend/get_friend_id", map[string]any{"userID": user}, &v)
	return v.FriendIDs, err
}
func (u *Upstream) Groups(ctx context.Context, ids []string) ([]Group, error) {
	if len(ids) == 0 {
		return []Group{}, nil
	}
	var v struct {
		GroupInfos []Group `json:"groupInfos"`
	}
	err := u.adminCall(ctx, "/group/get_groups_info", map[string]any{"groupIDs": unique(ids)}, &v)
	return v.GroupInfos, err
}
func (u *Upstream) GroupMembers(ctx context.Context, group string) ([]User, error) {
	all := []User{}
	for page := 1; page <= 1000; page++ {
		var v struct {
			Members []User `json:"members"`
			Total   int    `json:"total"`
		}
		err := u.adminCall(ctx, "/group/get_group_member_list", map[string]any{"groupID": group, "filter": 0, "pagination": map[string]int{"pageNumber": page, "showNumber": 200}}, &v)
		if err != nil {
			return nil, err
		}
		all = append(all, v.Members...)
		if len(v.Members) < 200 || len(all) >= v.Total {
			return all, nil
		}
	}
	return nil, bad("group exceeds supported 200000-member snapshot")
}
func (u *Upstream) Message(ctx context.Context, conversation, id string, seq int64, sender string) (Message, error) {
	group := strings.TrimPrefix(conversation, "sg_")
	if group == conversation || group == "" {
		return Message{}, bad("group conversation required")
	}
	if seq > 0 {
		var v struct {
			Msgs map[string]struct {
				Msgs []Message `json:"msgs"`
			} `json:"msgs"`
		}
		err := u.adminCall(ctx, "/msg/pull_msg_by_seq", map[string]any{"userID": sender, "seqRanges": []any{map[string]any{"conversationID": conversation, "begin": seq, "end": seq, "num": 1}}, "order": 0}, &v)
		if err != nil {
			return Message{}, err
		}
		for _, m := range v.Msgs[conversation].Msgs {
			if m.ClientMsgID == id && m.GroupID == group {
				return m, nil
			}
		}
		return Message{}, missing("message not found at supplied sequence")
	}
	// Legacy callers had no seq. Resolve by server-owned history rather than
	// trusting the client-supplied sender or fabricating an arbitrary receipt.
	for p := 1; p <= 100; p++ {
		var v struct {
			ChatLogs []struct {
				ChatLog   Message `json:"chatLog"`
				IsRevoked bool    `json:"isRevoked"`
			} `json:"chatLogs"`
			ChatLogsNum int `json:"chatLogsNum"`
		}
		err := u.adminCall(ctx, "/msg/search_msg", map[string]any{"sendID": sender, "recvID": group, "sessionType": 3, "pagination": map[string]int{"pageNumber": p, "showNumber": 100}}, &v)
		if err != nil {
			return Message{}, err
		}
		for _, x := range v.ChatLogs {
			m := x.ChatLog
			if !x.IsRevoked && m.ClientMsgID == id && (m.GroupID == group || m.RecvID == group) {
				m.GroupID = group
				return m, nil
			}
		}
		if len(v.ChatLogs) < 100 {
			break
		}
	}
	return Message{}, missing("message not found; supply its seq")
}
func (u *Upstream) Send(ctx context.Context, a Actor, recv, group, content string, kind int) (string, error) {
	users, err := u.Users(ctx, []string{a.UserID})
	if err != nil {
		return "", err
	}
	if len(users) != 1 {
		return "", missing("sender not found")
	}
	var parsed any
	if err = json.Unmarshal([]byte(content), &parsed); err != nil {
		return "", bad("message content must be JSON")
	}
	session := 1
	if group != "" {
		session = 3
	}
	var v struct {
		ClientMsgID string `json:"clientMsgID"`
	}
	err = u.adminCall(ctx, "/msg/send_msg", map[string]any{"sendID": a.UserID, "recvID": recv, "groupID": group, "senderNickname": users[0].Nickname, "senderFaceURL": users[0].FaceURL, "senderPlatformID": a.Platform, "content": parsed, "contentType": kind, "sessionType": session, "isOnlineOnly": false, "notOfflinePush": false}, &v)
	if err != nil {
		return "", &uncertainDelivery{err}
	}
	return v.ClientMsgID, err
}

type uncertainDelivery struct{ error }

func (*uncertainDelivery) Uncertain() bool { return true }

// Hints never update conversations, history, unread counts or offline push. The
// marker is separate from the original visible custom-message subtypes 901-905.
func (u *Upstream) Hint(ctx context.Context, a Actor, recv, payload string) error {
	users, err := u.Users(ctx, []string{a.UserID})
	if err != nil {
		return err
	}
	if len(users) != 1 {
		return ErrNotFound
	}
	platform := a.Platform
	if platform == 0 {
		platform = 5
	}
	return u.adminCall(ctx, "/msg/send_msg", map[string]any{"sendID": a.UserID, "recvID": recv, "senderNickname": users[0].Nickname, "senderFaceURL": users[0].FaceURL, "senderPlatformID": platform, "content": map[string]string{"data": payload, "description": "enterprise_event", "extension": ""}, "contentType": 110, "sessionType": 1, "isOnlineOnly": true, "notOfflinePush": true}, nil)
}
