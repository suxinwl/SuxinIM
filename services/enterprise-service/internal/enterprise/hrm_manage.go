package enterprise

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"openim38.local/identity"
)

var publicConfigKeys = map[string]bool{"webTitle": true, "appName": true, "logoURL": true, "privacyURL": true, "userAgreementURL": true, "allowAddFriend": true, "discoverEnabled": true, "needInvitationCodeRegister": true}

func onlyFields(q request, names string) error {
	allow := map[string]bool{}
	for _, v := range strings.Fields(names) {
		allow[v] = true
	}
	for key := range q {
		if !allow[key] {
			return bad("unsupported field: " + key)
		}
	}
	return nil
}
func managementPage(q request) (map[string]int64, error) {
	p, n := q.num("pageNumber"), q.num("showNumber")
	if p == 0 {
		p = 1
	}
	if n == 0 {
		n = 50
	}
	if p < 1 || n < 1 || n > 100 {
		return nil, bad("invalid pagination")
	}
	return map[string]int64{"pageNumber": p, "showNumber": n}, nil
}
func (b *HRMBridge) manage(ctx context.Context, op string, q request) (any, error) {
	fields := map[string]string{
		"storage/capacity": "",
		"groups/search":    "keyword pageNumber showNumber", "groups/members": "groupID pageNumber showNumber",
		"groups/create": "groupName ownerUserID memberUserIDs groupType notification", "groups/dismiss": "groupID", "groups/mute": "groupID muted", "groups/notice": "groupID notice",
		"groups/members/add": "groupID invitedUserIDs", "groups/members/remove": "groupID kickedUserIDs", "groups/members/mute": "groupID userID mutedSeconds", "groups/members/role": "groupID userID roleLevel", "groups/members/nickname": "groupID userID nickname",
		"groups/owner": "groupID oldOwnerUserID newOwnerUserID", "groups/info": "groupID groupName notification introduction",
		"messages/search": "sendID recvID groupID keyword startTime endTime pageNumber showNumber", "messages/revoke": "conversationID seq userID", "messages/notification": "sendID recvID groupID content sendAll",
		"users/online": "userIDs", "users/kick": "userID platformID", "friends/search": "userID pageNumber showNumber", "friends/add": "ownerUserID friendUserID", "friends/remove": "ownerUserID friendUserID",
		"rtc/status": "", "rtc/records/search": "userID startTime endTime pageNumber showNumber", "rtc/records/delete": "recordIDs",
		"config/get": "", "config/update": "config webTitle appName logoURL privacyURL userAgreementURL allowAddFriend discoverEnabled", "versions/search": "platform pageNumber showNumber", "versions/create": "id platform version downloadURL force description build size sha256 latest hot", "versions/update": "id platform version downloadURL force description build size sha256 latest hot", "versions/delete": "id",
		"discovery/list": "", "discovery/save": "items",
	}
	allowed, ok := fields[op]
	if !ok {
		return nil, missing("management operation is not supported")
	}
	if e := onlyFields(q, allowed); e != nil {
		return nil, e
	}
	pagination, e := managementPage(q)
	if e != nil {
		return nil, e
	}
	var out any
	var path string
	var payload any = q
	if strings.HasPrefix(op, "groups/") && op != "groups/create" && op != "groups/search" && op != "groups/members" && strings.HasPrefix(q.str("groupID"), "dept_") {
		return nil, forbidden("department structure is managed by HRM reconciliation")
	}
	switch op {
	case "storage/capacity":
		return b.Server.storageCapacity(), nil
	case "groups/search":
		path = "/group/get_groups"
		payload = map[string]any{"groupName": q.str("keyword"), "pagination": pagination}
	case "groups/members":
		path = "/group/get_group_member_list"
		payload = map[string]any{"groupID": q.str("groupID"), "pagination": pagination}
	case "groups/create":
		if strings.TrimSpace(q.str("groupName")) == "" || q.str("ownerUserID") == "" {
			return nil, bad("group name and owner are required")
		}
		if q.has("groupType") && q.num("groupType") != 2 {
			return nil, bad("only working groups are supported")
		}
		members := q.strings("memberUserIDs")
		clean := []string{}
		for _, id := range members {
			if id != q.str("ownerUserID") {
				clean = append(clean, id)
			}
		}
		path = "/group/create_group"
		payload = map[string]any{"groupInfo": map[string]any{"groupName": q.str("groupName"), "groupType": 2, "notification": q.str("notification")}, "ownerUserID": q.str("ownerUserID"), "memberUserIDs": clean}
	case "groups/dismiss":
		path = "/group/dismiss_group"
	case "groups/mute":
		path = "/group/cancel_mute_group"
		if q.boolean("muted") {
			path = "/group/mute_group"
		}
		payload = map[string]any{"groupID": q.str("groupID")}
	case "groups/notice":
		path = "/group/set_group_info_ex"
		payload = map[string]any{"groupID": q.str("groupID"), "notification": q.str("notice")}
	case "groups/info":
		path = "/group/set_group_info_ex"
	case "groups/owner":
		path = "/group/transfer_group"
	case "groups/members/add":
		path = "/group/invite_user_to_group"
	case "groups/members/remove":
		path = "/group/kick_group"
	case "groups/members/mute":
		path = "/group/mute_group_member"
		payload = map[string]any{"groupID": q.str("groupID"), "userID": q.str("userID"), "mutedSeconds": q.num("mutedSeconds")}
		if q.num("mutedSeconds") <= 0 {
			path = "/group/cancel_mute_group_member"
			payload = map[string]any{"groupID": q.str("groupID"), "userID": q.str("userID")}
		}
	case "groups/members/role", "groups/members/nickname":
		path = "/group/set_group_member_info"
		payload = map[string]any{"members": []any{q}}
	case "messages/search":
		return b.Server.searchManagedMessages(ctx, q, pagination)
	case "messages/revoke":
		path = "/msg/revoke_msg"
	case "messages/notification":
		if strings.TrimSpace(q.str("content")) == "" {
			return nil, bad("notification content required")
		}
		send := q.str("sendID")
		if send == "" {
			send = b.Server.Upstream.IMAdminID
		}
		session := 1
		if q.str("groupID") != "" {
			session = 3
		}
		payload = map[string]any{"sendID": send, "recvID": q.str("recvID"), "groupID": q.str("groupID"), "senderPlatformID": 10, "contentType": 110, "sessionType": session, "content": map[string]string{"data": q.str("content"), "description": "system_notification", "extension": ""}}
		path = "/msg/send_msg"
		if q.boolean("sendAll") {
			if q.str("recvID") != "" || q.str("groupID") != "" {
				return nil, bad("sendAll cannot include a recipient or group filter")
			}
			path = "/msg/batch_send_msg"
			payload.(map[string]any)["isSendAll"] = true
			// The fixed 3.8 HTTP binder requires a nonempty RecvIDs even
			// though IsSendAll replaces it with the server's user directory.
			payload.(map[string]any)["recvIDs"] = []string{send}
		}
	case "users/online":
		// The upstream administrator response contains live connection tokens.
		// Decode into a public DTO so credentials and future internal fields
		// can never cross the HRM management boundary.
		var online []struct {
			UserID               string `json:"userID"`
			Status               int    `json:"status"`
			DetailPlatformStatus []struct {
				PlatformID   int  `json:"platformID"`
				IsBackground bool `json:"isBackground"`
			} `json:"detailPlatformStatus"`
		}
		e = b.Server.Upstream.adminCall(ctx, "/user/get_users_online_status", q, &online)
		return online, e
	case "users/kick":
		user := q.str("userID")
		p := int(q.num("platformID"))
		if !identity.Managed(user) || user == "hrm-im-system" || p != 0 && !identity.Platform(p) {
			return nil, bad("invalid employee revocation target")
		}
		unlock, err := b.State.Lock(ctx, user)
		if err != nil {
			return nil, err
		}
		defer unlock()
		ids, e := b.State.Revoke(ctx, user, "", p, p == 0, 0)
		if e != nil {
			return nil, e
		}
		for _, sid := range ids {
			if e = b.Server.markHRMGenerations(ctx, user, sid, false, 0); e != nil {
				return nil, e
			}
		}
		return map[string]any{"revoked": true}, nil
	case "friends/search":
		path = "/friend/get_friend_list"
		payload = map[string]any{"userID": q.str("userID"), "pagination": pagination}
	case "friends/add":
		path = "/friend/import_friend"
		payload = map[string]any{"ownerUserID": q.str("ownerUserID"), "friendUserIDs": []string{q.str("friendUserID")}}
	case "friends/remove":
		path = "/friend/delete_friend"
	case "rtc/status":
		rs, e := b.Server.rooms(ctx)
		if e != nil {
			return nil, e
		}
		active := 0
		for _, r := range rs {
			if !r.ended() {
				active++
			}
		}
		return map[string]any{"activeRooms": active, "totalRooms": len(rs), "enabled": b.Server.Media != nil}, nil
	case "rtc/records/search":
		rs, e := b.Server.rooms(ctx)
		if e != nil {
			return nil, e
		}
		items := []Room{}
		for _, r := range rs {
			if r.RecordDeleted || q.str("userID") != "" && !r.eligible(q.str("userID")) || q.num("startTime") > 0 && r.CreateTime < q.num("startTime") || q.num("endTime") > 0 && r.CreateTime > q.num("endTime") {
				continue
			}
			items = append(items, r)
		}
		return map[string]any{"items": page(items, asRequest(map[string]any{"pagination": pagination})), "total": len(items)}, nil
	case "rtc/records/delete":
		for _, id := range q.strings("recordIDs") {
			if _, e := mutate[Room](ctx, b.Server.Store, "room:"+id, func(r *Room) error {
				if !r.ended() {
					return bad("active room records cannot be deleted")
				}
				r.RecordDeleted = true
				return nil
			}); e != nil {
				return nil, e
			}
		}
		return map[string]any{}, nil
	case "config/get":
		var v struct {
			Config map[string]string `json:"config"`
		}
		if e = b.chatCall(ctx, true, "/client_config/get", map[string]any{}, &v); e != nil {
			return nil, e
		}
		safe := map[string]string{}
		for k, v := range v.Config {
			if publicConfigKeys[k] {
				safe[k] = v
			}
		}
		return map[string]any{"config": safe}, nil
	case "config/update":
		cfg := map[string]string{}
		if q.has("config") {
			if len(q) != 1 {
				return nil, bad("config object cannot be mixed with flat keys")
			}
			if q.decode("config", &cfg) != nil {
				return nil, bad("invalid config object")
			}
		} else {
			for key, raw := range q {
				var value any
				if json.Unmarshal(raw, &value) != nil {
					return nil, bad("invalid config value")
				}
				switch v := value.(type) {
				case string:
					cfg[key] = v
				case bool:
					cfg[key] = strconv.FormatBool(v)
				default:
					return nil, bad("client config must contain strings or booleans")
				}
			}
		}
		for k := range cfg {
			if !publicConfigKeys[k] || k == "needInvitationCodeRegister" {
				return nil, bad("unsupported client config key: " + k)
			}
		}
		e = b.chatCall(ctx, true, "/client_config/set", map[string]any{"config": cfg}, &out)
		return out, e
	case "versions/search":
		platforms := []string{}
		if p := q.str("platform"); p != "" {
			platforms = append(platforms, p)
		}
		var data struct {
			Total    int64            `json:"total"`
			Versions []map[string]any `json:"versions"`
		}
		e = b.chatCall(ctx, true, "/application/page_versions", map[string]any{"platform": platforms, "pagination": pagination}, &data)
		if e != nil {
			return nil, e
		}
		for _, row := range data.Versions {
			p, _ := row["platform"].(string)
			v, _ := row["version"].(string)
			if artifact, _, err := load[Artifact](ctx, b.Server.Store, artifactKey(p, v)); err == nil {
				row["build"] = artifact.Build
				row["size"] = artifact.Size
				row["sha256"] = artifact.SHA256
			}
			row["downloadURL"] = row["url"]
			row["description"] = row["text"]
		}
		return data, nil
	case "versions/delete":
		e = b.chatCall(ctx, true, "/application/delete_version", map[string]any{"id": []string{q.str("id")}}, &out)
		return out, e
	case "versions/create", "versions/update":
		if q.has("hot") && q.boolean("hot") || q.has("latest") && !q.boolean("latest") {
			return nil, bad("only active full-package releases are supported")
		}
		if q.num("build") <= 0 || q.num("size") <= 0 || len(q.str("sha256")) != 64 {
			return nil, bad("build, size and SHA-256 are required")
		}
		var artifact = Artifact{Platform: q.str("platform"), Version: q.str("version"), URL: q.str("downloadURL"), Text: q.str("description"), Force: q.boolean("force"), Build: q.num("build"), Size: q.num("size"), SHA256: q.str("sha256")}
		if op == "versions/create" {
			return b.Server.publish(ctx, Actor{UserID: "hrm-bridge", Admin: true}, asRequest(artifact))
		}
		// Binary artifacts remain immutable; edits only change release text/policy.
		old, _, err := load[Artifact](ctx, b.Server.Store, artifactKey(artifact.Platform, artifact.Version))
		if err != nil {
			return nil, err
		}
		if old.URL != artifact.URL || old.Build != artifact.Build || old.Size != artifact.Size || old.SHA256 != strings.ToLower(artifact.SHA256) {
			return nil, conflict("release binary is immutable")
		}
		e = b.chatCall(ctx, true, "/application/update_version", map[string]any{"id": q.str("id"), "text": artifact.Text, "force": artifact.Force}, &out)
		if e == nil {
			_, e = mutate[Artifact](ctx, b.Server.Store, artifactKey(artifact.Platform, artifact.Version), func(v *Artifact) error {
				v.Text, v.Force = artifact.Text, artifact.Force
				return nil
			})
		}
		return out, e
	case "discovery/list":
		v, _, e := load[json.RawMessage](ctx, b.Server.Store, "discovery:items")
		if errors.Is(e, ErrNotFound) {
			return map[string]any{"items": []any{}}, nil
		}
		return v, e
	case "discovery/save":
		var rows []struct {
			ID      string `json:"id"`
			Title   string `json:"title"`
			URL     string `json:"url"`
			Icon    string `json:"icon"`
			Enabled bool   `json:"enabled"`
			Sort    int    `json:"sort"`
		}
		if q.decode("items", &rows) != nil || len(rows) > 100 {
			return nil, bad("invalid discovery items")
		}
		seen := map[string]bool{}
		for i := range rows {
			if rows[i].ID == "" {
				rows[i].ID = newID()
			}
			if seen[rows[i].ID] || strings.TrimSpace(rows[i].Title) == "" {
				return nil, bad("duplicate discovery ID or missing title")
			}
			seen[rows[i].ID] = true
			u, e := url.Parse(rows[i].URL)
			if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
				return nil, bad("discovery URL must use HTTPS")
			}
		}
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Sort < rows[j].Sort })
		value := json.RawMessage(mustJSON(map[string]any{"items": rows}))
		_, e = mutate[json.RawMessage](ctx, b.Server.Store, "discovery:items", func(v *json.RawMessage) error { *v = value; return nil })
		if errors.Is(e, ErrNotFound) {
			e = b.Server.Store.Save(ctx, document("discovery:items", "discovery", "", value), 0)
		}
		return map[string]any{"items": rows}, e
	}
	if path == "" {
		return nil, missing("management operation not implemented")
	}
	e = b.Server.Upstream.adminCall(ctx, path, payload, &out)
	return out, e
}
func asRequest(v any) request { var q request; _ = json.Unmarshal(mustJSON(v), &q); return q }

func (s *Server) searchManagedMessages(ctx context.Context, q request, pagination map[string]int64) (any, error) {
	if s.Messages == nil {
		return nil, unavailable("message database unavailable")
	}
	filter := bson.M{"msgs.msg": bson.M{"$ne": nil}}
	for k, field := range map[string]string{"sendID": "send_id", "recvID": "recv_id", "groupID": "group_id"} {
		if v := q.str(k); v != "" {
			filter["msgs.msg."+field] = v
		}
	}
	if keyword := q.str("keyword"); keyword != "" {
		filter["msgs.msg.content"] = bson.M{"$regex": regexp.QuoteMeta(keyword), "$options": "i"}
	}
	timeRange := bson.M{}
	for k, mongoOp := range map[string]string{"startTime": "$gte", "endTime": "$lte"} {
		if v := q.num(k); v > 0 {
			if v < 100000000000 {
				v *= 1000
			}
			timeRange[mongoOp] = v
		}
	}
	if len(timeRange) > 0 {
		filter["msgs.msg.send_time"] = timeRange
	}
	pipeline := mongo.Pipeline{bson.D{{Key: "$unwind", Value: "$msgs"}}, bson.D{{Key: "$match", Value: filter}}, bson.D{{Key: "$sort", Value: bson.D{{Key: "msgs.msg.send_time", Value: -1}, {Key: "msgs.msg.seq", Value: -1}}}}, bson.D{{Key: "$facet", Value: bson.M{"meta": bson.A{bson.M{"$count": "total"}}, "items": bson.A{bson.M{"$skip": (pagination["pageNumber"] - 1) * pagination["showNumber"]}, bson.M{"$limit": pagination["showNumber"]}, bson.M{"$project": bson.M{"_id": 0, "docID": "$doc_id", "message": "$msgs.msg", "revoked": "$msgs.revoke"}}}}}}}
	cursor, e := s.Messages.Aggregate(ctx, pipeline)
	if e != nil {
		return nil, e
	}
	defer cursor.Close(ctx)
	var results []struct {
		Meta []struct {
			Total int64 `bson:"total"`
		} `bson:"meta"`
		Items []bson.M `bson:"items"`
	}
	if e = cursor.All(ctx, &results); e != nil {
		return nil, e
	}
	items := []bson.M{}
	total := int64(0)
	if len(results) > 0 {
		items = results[0].Items
		if items == nil {
			items = []bson.M{}
		}
		if len(results[0].Meta) > 0 {
			total = results[0].Meta[0].Total
		}
	}
	for _, row := range items {
		if message, ok := row["message"].(bson.M); ok {
			for out, in := range map[string]string{"sendID": "send_id", "recvID": "recv_id", "groupID": "group_id", "seq": "seq", "sendTime": "send_time", "content": "content", "contentType": "content_type", "clientMsgID": "client_msg_id"} {
				row[out] = message[in]
			}
		}
		if id, ok := row["docID"].(string); ok {
			if i := strings.LastIndex(id, ":"); i >= 0 {
				id = id[:i]
			}
			row["conversationID"] = id
		}
		row["isRevoked"] = row["revoked"] != nil
	}
	return map[string]any{"items": items, "total": total, "queriedAt": time.Now().Unix()}, nil
}
