package enterprise

import (
	"context"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"openim38.local/identity"
)

// HRMBridge is only mounted on the private listener. No public route accepts
// an administrator token, arbitrary upstream URL, or forwarded identity.
type HRMBridge struct {
	Server *Server
	State  *identity.Store
	Key    string
	Auth   *GRPCAuth
}
type hrmGeneration struct {
	SessionID  string
	UserID     string
	Epoch      int64
	ExpiresAt  int64
	Ciphertext []byte
	Revoked    bool
	Complete   bool
}

func (b *HRMBridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || r.URL.RawQuery != "" || !strings.HasPrefix(r.URL.Path, "/internal/hrm/") || len(b.Key) < 32 {
		writeError(w, forbidden("private bridge request rejected"))
		return
	}
	body, e := io.ReadAll(io.LimitReader(r.Body, (2<<20)+1))
	if e != nil || len(body) > 2<<20 {
		writeError(w, bad("request too large"))
		return
	}
	stamp, nonce, sig := r.Header.Get("X-HRM-Timestamp"), r.Header.Get("X-HRM-Nonce"), r.Header.Get("X-HRM-Signature")
	ts, e := strconv.ParseInt(stamp, 10, 64)
	now := time.Now().Unix()
	want, _ := hex.DecodeString(identity.Sign(b.Key, stamp, nonce, r.Method, r.URL.Path, body))
	got, e2 := hex.DecodeString(sig)
	if e != nil || e2 != nil || ts < now-60 || ts > now+60 || len(nonce) < 16 || len(nonce) > 128 || !hmac.Equal(want, got) {
		writeError(w, unauthorized("private signature rejected"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	ok, e := b.State.Redis.SetNX(ctx, "HRM38:NONCE:"+identity.Hash(nonce), 1, 121*time.Second).Result()
	if e != nil {
		writeError(w, unavailable("identity registry unavailable"))
		return
	}
	if !ok {
		writeError(w, unauthorized("request replay rejected"))
		return
	}
	var q request
	if json.Unmarshal(body, &q) != nil {
		writeError(w, bad("JSON object required"))
		return
	}
	op := strings.TrimPrefix(r.URL.Path, "/internal/hrm/")
	var data any
	switch {
	case op == "exchange":
		data, e = b.exchange(ctx, q)
	case op == "revoke":
		data, e = b.revoke(ctx, q)
	case op == "users/reconcile":
		data, e = b.reconcileUser(ctx, q)
	case op == "departments/reconcile":
		data, e = b.reconcileDepartment(ctx, q)
	case op == "access/check":
		if e = onlyFields(q, "userID platform logicalClient clientIP entryZone action resourcePath"); e == nil {
			if q.has("entryZone") && q.str("entryZone") != "lan" && q.str("entryZone") != "internet" {
				writeError(w, bad("invalid entry zone"))
				return
			}
			ctx = identity.WithEntryZone(ctx, q.str("entryZone"))
			data, e = identity.CheckLogicalAccess(ctx, q.str("userID"), int(q.num("platform")), q.str("logicalClient"), q.str("clientIP"), q.str("action"), q.str("resourcePath"))
			if errors.Is(e, identity.ErrAccessDenied) {
				e = forbidden("HRM access denied")
			}
		}
	case strings.HasPrefix(op, "manage/"):
		data, e = b.manage(ctx, strings.TrimPrefix(op, "manage/"), q)
	default:
		e = missing("unknown private operation")
	}
	if e != nil {
		writeError(w, e)
		return
	}
	writeJSON(w, 200, data)
}
func unavailable(message string) error { return &apiError{503, 1006, message} }
func (b *HRMBridge) chatToken(ctx context.Context, user string, kind int) (string, error) {
	in := fieldString(nil, 1, user)
	in = fieldInt(in, 32, int64(kind))
	var out []byte
	if e := b.Auth.chat.Invoke(ctx, "/openim.admin.admin/CreateToken", &in, &out); e != nil {
		return "", e
	}
	fields, e := wireFields(out)
	if e != nil {
		return "", e
	}
	token := strField(fields, 1)
	if token == "" {
		return "", unavailable("Chat token unavailable")
	}
	return token, nil
}
func (b *HRMBridge) chatCall(ctx context.Context, admin bool, path string, body, out any) error {
	token, e := b.chatToken(ctx, "hrm-bridge", 2)
	if e != nil {
		return e
	}
	base := b.Server.Upstream.ChatBase
	if admin {
		base = b.Server.Upstream.ChatAdminBase
	}
	up := &Upstream{IMBase: base, HTTP: b.Server.Upstream.HTTP}
	return up.call(ctx, path, token, body, out)
}
func (b *HRMBridge) exchange(ctx context.Context, q request) (any, error) {
	v := identity.Session{UserID: q.str("userID"), SessionID: q.str("sessionID"), Platform: int(q.num("platform")), Epoch: q.num("epoch"), ExpiresAt: q.num("expiresAt")}
	v.PolicyPlatform = int(q.num("policyPlatform"))
	v.LogicalClient = q.str("logicalClient")
	if v.PolicyPlatform == 0 {
		v.PolicyPlatform = v.Platform
	}
	client, clientErr := identity.SessionClient(v)
	if clientErr != nil {
		return nil, bad("invalid logical client")
	}
	v.LogicalClient = client
	if !identity.Managed(v.UserID) || v.UserID == "hrm-im-system" || len(q.str("ticket")) < 16 {
		return nil, bad("invalid employee exchange")
	}
	unlock, e := b.State.Lock(ctx, v.UserID)
	if e != nil {
		return nil, e
	}
	defer unlock()
	// Reusing a ticket for the identical request is a short, private retry only.
	key := "HRM38:EXCHANGE:" + identity.Hash(q.str("ticket"))
	var prior struct {
		RequestHash        string
		IMToken, ChatToken string
	}
	fingerprint := identity.Hash(string(mustJSON(q)))
	if b.State.Read(ctx, key, &prior) == nil {
		if prior.RequestHash != fingerprint {
			return nil, forbidden("ticket identity changed")
		}
		if _, e = b.State.Pair(ctx, prior.ChatToken, prior.IMToken, v.UserID, v.Platform); e != nil {
			return nil, unauthorized("session revoked")
		}
		return map[string]any{"imToken": prior.IMToken, "chatToken": prior.ChatToken}, nil
	}
	p, e := b.State.Projection(ctx, v.UserID)
	if e != nil || !p.Enabled {
		return nil, unauthorized("employee is not enabled")
	}
	if old, e := b.State.Session(ctx, v.SessionID); e == nil {
		oldClient, clientErr := identity.SessionClient(old)
		if old.Revoked || old.UserID != v.UserID || old.Platform != v.Platform || old.Epoch != v.Epoch || clientErr != nil || oldClient != client {
			return nil, unauthorized("session cannot be reused")
		}
	}
	chat, e := b.chatToken(ctx, v.UserID, 1)
	if e != nil {
		return nil, e
	}
	var response struct {
		Token string `json:"token"`
	}
	if e = b.Server.Upstream.adminCall(ctx, "/auth/get_user_token", map[string]any{"userID": v.UserID, "platformID": v.Platform}, &response); e != nil {
		return nil, e
	}
	replaced, e := b.State.Exchange(ctx, v, chat, response.Token)
	if e != nil {
		return nil, unauthorized("session exchange rejected")
	}
	actor := Actor{UserID: v.UserID, Platform: v.Platform, LogicalClient: client, IMToken: response.Token, ChatToken: chat, SessionID: v.SessionID, Epoch: v.Epoch, ExpiresAt: v.ExpiresAt}
	encrypted, e := b.Server.Cipher.seal(actor)
	if e != nil {
		return nil, e
	}
	generation := hrmGeneration{SessionID: v.SessionID, UserID: v.UserID, Epoch: v.Epoch, ExpiresAt: v.ExpiresAt, Ciphertext: encrypted}
	if e = b.Server.Store.Save(ctx, document("hrm_generation:"+identity.Hash(response.Token), "hrm_generation", v.UserID, generation), 0); e != nil {
		return nil, e
	}
	// All token generations of the replaced SID are retried durably by Run.
	for _, sid := range replaced {
		if e = b.Server.markHRMGenerations(ctx, v.UserID, sid, false, 0); e != nil {
			return nil, e
		}
	}
	prior.RequestHash = fingerprint
	prior.IMToken = response.Token
	prior.ChatToken = chat
	if e = b.State.Write(ctx, key, prior, 30*time.Second); e != nil {
		return nil, e
	}
	return map[string]any{"imToken": response.Token, "chatToken": chat}, nil
}
func (b *HRMBridge) revoke(ctx context.Context, q request) (any, error) {
	user, sid, p, all, epoch := q.str("userID"), q.str("sessionID"), int(q.num("platform")), q.boolean("all"), q.num("epoch")
	if !identity.Managed(user) || user == "hrm-im-system" || (sid == "" && p == 0 && !all) || p != 0 && !identity.Platform(p) {
		return nil, bad("invalid revocation target")
	}
	if sid != "" && all {
		return nil, bad("exact SID and all-session revocation cannot be combined")
	}
	unlock, e := b.State.Lock(ctx, user)
	if e != nil {
		return nil, e
	}
	defer unlock()
	ids, e := b.State.Revoke(ctx, user, sid, p, all, epoch)
	if e != nil {
		return nil, e
	}
	// Mongo is a durable retry journal, not an alternate authentication source.
	for _, id := range ids {
		if e = b.Server.markHRMGenerations(ctx, user, id, false, 0); e != nil {
			return nil, e
		}
	}
	if all {
		if e = b.Server.markHRMGenerations(ctx, user, "", true, epoch); e != nil {
			return nil, e
		}
	}
	if e = b.Server.revalidateMediaSessions(ctx); e != nil {
		return nil, e
	}
	return map[string]any{"revoked": true}, nil
}
func (b *HRMBridge) reconcileUser(ctx context.Context, q request) (any, error) {
	var p identity.Projection
	if json.Unmarshal(mustJSON(q), &p) != nil || !identity.Managed(p.UserID) || p.UserID == "hrm-im-system" || p.Version < 1 || strings.TrimSpace(p.Account) == "" || strings.TrimSpace(p.Nickname) == "" {
		return nil, bad("invalid employee projection")
	}
	unlock, e := b.State.Lock(ctx, p.UserID)
	if e != nil {
		return nil, e
	}
	defer unlock()
	old, doc, e := load[identity.Projection](ctx, b.Server.Store, "hrm_projection:"+p.UserID)
	if e != nil && !errors.Is(e, ErrNotFound) {
		return nil, e
	}
	if e == nil && old.Version > p.Version {
		if e = b.State.PutProjection(ctx, old); e != nil {
			return nil, e
		}
		return map[string]any{"userID": old.UserID, "appliedVersion": old.Version}, nil
	}
	if !p.Enabled {
		if e = b.State.PutProjection(ctx, p); e != nil {
			return nil, e
		}
		ids, e := b.State.Revoke(ctx, p.UserID, "", 0, true, 0)
		if e != nil {
			return nil, e
		}
		for _, sid := range ids {
			if e = b.Server.markHRMGenerations(ctx, p.UserID, sid, false, 0); e != nil {
				return nil, e
			}
		}
	}
	// A shadow credential is deliberately unrelated to the HRM login account.
	// HRM accounts can contain characters unsupported by the upstream demo.
	shadow := "hrm" + identity.Hash(p.UserID)[:32]
	var found struct {
		Users []User `json:"users"`
	}
	if e = b.chatCall(ctx, false, "/user/find/full", map[string]any{"userIDs": []string{p.UserID}}, &found); e != nil {
		return nil, e
	}
	if len(found.Users) == 0 {
		e = b.chatCall(ctx, true, "/account/add_user", map[string]any{"user": map[string]any{"userID": p.UserID, "account": shadow, "password": identity.Hash(newID() + newID()), "nickname": p.Nickname, "faceURL": p.FaceURL}, "platform": 10}, nil)
		if e != nil {
			if again := b.chatCall(ctx, false, "/user/find/full", map[string]any{"userIDs": []string{p.UserID}}, &found); again != nil || len(found.Users) == 0 {
				return nil, e
			}
		}
	}
	if e = b.chatCall(ctx, false, "/user/update", map[string]any{"userID": p.UserID, "nickname": p.Nickname, "faceURL": p.FaceURL, "gender": 0, "birth": 0}, nil); e != nil {
		return nil, e
	}
	// Repair IM independently after a partially completed upstream registration.
	var check struct {
		Results []struct {
			UserID        string `json:"userID"`
			AccountStatus int    `json:"accountStatus"`
		} `json:"results"`
	}
	if e = b.Server.Upstream.adminCall(ctx, "/user/account_check", map[string]any{"checkUserIDs": []string{p.UserID}}, &check); e != nil {
		return nil, e
	}
	exists := false
	for _, v := range check.Results {
		if v.UserID == p.UserID && v.AccountStatus == 1 {
			exists = true
		}
	}
	if !exists {
		if e = b.Server.Upstream.adminCall(ctx, "/user/user_register", map[string]any{"users": []any{map[string]any{"userID": p.UserID, "nickname": p.Nickname, "faceURL": p.FaceURL}}}, nil); e != nil {
			return nil, e
		}
	}
	if e = b.Server.Upstream.adminCall(ctx, "/user/update_user_info_ex", map[string]any{"userInfo": map[string]any{"userID": p.UserID, "nickname": p.Nickname, "faceURL": p.FaceURL}}, nil); e != nil {
		return nil, e
	}
	if e = b.State.PutProjection(ctx, p); e != nil {
		return nil, e
	}
	if doc.ID == "" {
		doc = document("hrm_projection:"+p.UserID, "hrm_projection", p.UserID, p)
	} else {
		doc.Data = mustJSON(p)
	}
	if e = b.Server.Store.Save(ctx, doc, doc.Version); e != nil {
		return nil, e
	}
	return map[string]any{"userID": p.UserID, "appliedVersion": p.Version}, nil
}
func (s *Server) markHRMGenerations(ctx context.Context, user, sid string, all bool, epoch int64) error {
	docs, e := s.Store.List(ctx, "hrm_generation", user)
	if e != nil {
		return e
	}
	for _, doc := range docs {
		var v hrmGeneration
		if json.Unmarshal(doc.Data, &v) != nil {
			continue
		}
		if sid != "" && v.SessionID != sid || all && epoch > 0 && v.Epoch >= epoch {
			continue
		}
		if !v.Revoked {
			v.Revoked = true
			doc.Data = mustJSON(v)
			if e = s.Store.Save(ctx, doc, doc.Version); e != nil && !errors.Is(e, ErrConflict) {
				return e
			}
		}
	}
	return s.retryHRMGenerations(ctx)
}
func (s *Server) retryHRMGenerations(ctx context.Context) error {
	if !identity.Required() || s.SessionRevoker == nil {
		return nil
	}
	docs, e := s.Store.List(ctx, "hrm_generation", "")
	if e != nil {
		return e
	}
	for _, doc := range docs {
		var v hrmGeneration
		if json.Unmarshal(doc.Data, &v) != nil {
			continue
		}
		// Native user tokens are capped at four hours. Keep the durable
		// retry record beyond both the HRM deadline and that native lifetime.
		if v.ExpiresAt < s.now().Add(-4*time.Hour).Unix() {
			if err := s.Store.Delete(ctx, doc.ID, doc.Version); err != nil && !errors.Is(err, ErrConflict) {
				return err
			}
			continue
		}
		if !v.Revoked || v.Complete {
			continue
		}
		a, e := s.Cipher.open(v.Ciphertext)
		if e != nil {
			return e
		}
		im := s.SessionRevoker.RevokeIMToken(ctx, a.IMToken)
		chat := s.SessionRevoker.RevokeChatToken(ctx, a.UserID, a.ChatToken)
		if im != nil || chat != nil {
			continue
		}
		v.Complete = true
		doc.Data = mustJSON(v)
		if e = s.Store.Save(ctx, doc, doc.Version); e != nil && !errors.Is(e, ErrConflict) {
			return e
		}
	}
	return nil
}
func (b *HRMBridge) reconcileDepartment(ctx context.Context, q request) (any, error) {
	id, name, version := q.str("departmentID"), strings.TrimSpace(q.str("name")), q.num("version")
	if id == "" || name == "" || version < 1 {
		return nil, bad("invalid department")
	}
	if e := onlyFields(q, "departmentID name groupID memberUserIDs frozen version groupEnabled leaderUserID"); e != nil {
		return nil, e
	}
	if q.has("groupEnabled") {
		var enabled bool
		if q.decode("groupEnabled", &enabled) != nil {
			return nil, bad("groupEnabled must be a boolean")
		}
	}
	group := "dept_" + identity.Hash("department:" + id)[:32]
	if q.has("groupID") && q.str("groupID") != "" && q.str("groupID") != group {
		return nil, bad("department group identity differs")
	}
	unlock, e := b.State.Lock(ctx, "department:"+id)
	if e != nil {
		return nil, e
	}
	defer unlock()
	var prior struct {
		Version int64 `json:"version"`
	}
	var previous json.RawMessage
	if old, _, e := load[json.RawMessage](ctx, b.Server.Store, "hrm_department:"+id); e == nil {
		if json.Unmarshal(old, &prior) != nil {
			return nil, unavailable("department projection unavailable")
		}
		previous = old
		if prior.Version > version {
			return map[string]any{"groupID": group, "appliedVersion": prior.Version}, nil
		}
	} else if !errors.Is(e, ErrNotFound) {
		return nil, e
	}
	// Department group creation is opt-in. Turning it off leaves the existing
	// group's membership, owner, messages and mute state completely intact.
	if !q.boolean("groupEnabled") {
		if sameDepartmentProjection(previous, q) {
			return map[string]any{"groupID": group, "appliedVersion": version, "groupEnabled": false}, nil
		}
		return b.saveDepartmentProjection(ctx, id, group, version, q)
	}
	members := q.strings("memberUserIDs")
	leader := q.str("leaderUserID")
	if !identity.Employee(leader) || !contains(members, leader) {
		return nil, bad("enabled department group requires its eligible leader in members")
	}
	for _, user := range members {
		p, e := b.State.Projection(ctx, user)
		if e != nil || !identity.Employee(user) || p.UserID != user || !p.Enabled || p.Version < 1 {
			return nil, bad("department member is not enabled")
		}
	}
	groups, e := b.Server.Upstream.Groups(ctx, []string{group})
	if e != nil {
		return nil, e
	}
	if len(groups) == 0 {
		ordinary := []string{}
		for _, member := range members {
			if member != leader {
				ordinary = append(ordinary, member)
			}
		}
		if e = b.Server.Upstream.adminCall(ctx, "/group/create_group", map[string]any{"groupInfo": map[string]any{"groupID": group, "groupName": name, "groupType": 2}, "ownerUserID": leader, "memberUserIDs": ordinary}, nil); e != nil {
			return nil, e
		}
	}
	current, e := b.Server.IM.GroupMembers(ctx, group)
	if e != nil {
		return nil, e
	}
	owner := leader
	if len(groups) > 0 {
		owner = groups[0].OwnerUserID
		for _, member := range current {
			if member.RoleLevel == 100 {
				owner = member.UserID
			}
		}
		if owner == "" {
			return nil, unavailable("department group owner unavailable")
		}
	}
	var add, remove []string
	for _, u := range members {
		found := false
		for _, c := range current {
			if c.UserID == u {
				found = true
			}
		}
		if !found {
			add = append(add, u)
		}
	}
	for _, u := range current {
		if !contains(members, u.UserID) {
			remove = append(remove, u.UserID)
		}
	}
	if len(add) > 0 {
		if e = b.Server.Upstream.adminCall(ctx, "/group/invite_user_to_group", map[string]any{"groupID": group, "invitedUserIDs": add}, nil); e != nil {
			return nil, e
		}
	}
	// A newly assigned leader must join before transfer. The previous owner can
	// only be removed after that succeeds, including legacy service-owned groups.
	if owner != leader {
		if e = b.Server.Upstream.adminCall(ctx, "/group/transfer_group", map[string]any{"groupID": group, "oldOwnerUserID": owner, "newOwnerUserID": leader}, nil); e != nil {
			return nil, e
		}
	}
	if len(remove) > 0 {
		if e = b.Server.Upstream.adminCall(ctx, "/group/kick_group", map[string]any{"groupID": group, "kickedUserIDs": remove}, nil); e != nil {
			return nil, e
		}
	}
	// Periodic reconciliation still repairs native drift. Avoid no-op RPCs,
	// which emit visible notifications even if the group name is unchanged.
	if len(groups) > 0 && groups[0].GroupName != name {
		if e = b.Server.Upstream.adminCall(ctx, "/group/set_group_info_ex", map[string]any{"groupID": group, "groupName": name}, nil); e != nil {
			return nil, e
		}
	}
	// Muting is a live moderation decision. Legacy frozen is accepted in the
	// HRM payload but never overrides a group owner's/admin's mute setting.
	if sameDepartmentProjection(previous, q) {
		return map[string]any{"groupID": group, "appliedVersion": version, "groupEnabled": true}, nil
	}
	return b.saveDepartmentProjection(ctx, id, group, version, q)
}

func sameDepartmentProjection(previous json.RawMessage, current request) bool {
	if len(previous) == 0 {
		return false
	}
	var before, after any
	return json.Unmarshal(previous, &before) == nil && json.Unmarshal(mustJSON(current), &after) == nil && reflect.DeepEqual(before, after)
}

func (b *HRMBridge) saveDepartmentProjection(ctx context.Context, id, group string, version int64, q request) (any, error) {
	_, e := mutate[json.RawMessage](ctx, b.Server.Store, "hrm_department:"+id, func(v *json.RawMessage) error { *v = mustJSON(q); return nil })
	if errors.Is(e, ErrNotFound) {
		e = b.Server.Store.Save(ctx, document("hrm_department:"+id, "hrm_department", id, json.RawMessage(mustJSON(q))), 0)
	}
	if e != nil {
		return nil, e
	}
	return map[string]any{"groupID": group, "appliedVersion": version, "groupEnabled": q.boolean("groupEnabled")}, nil
}
func NewHRMBridge(s *Server, auth *GRPCAuth) (*HRMBridge, error) {
	state, e := identity.Global()
	if e != nil {
		return nil, e
	}
	key := os.Getenv("HRM_BRIDGE_HMAC_KEY")
	if len(key) < 32 {
		return nil, errors.New("HRM_BRIDGE_HMAC_KEY must contain at least 32 bytes")
	}
	return &HRMBridge{Server: s, Auth: auth, State: state, Key: key}, nil
}
