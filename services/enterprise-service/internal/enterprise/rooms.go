package enterprise

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Invitation struct {
	InviterUserID      string   `json:"inviterUserID"`
	InviteeUserIDList  []string `json:"inviteeUserIDList"`
	CustomData         string   `json:"customData"`
	GroupID            string   `json:"groupID"`
	RoomID             string   `json:"roomID"`
	Timeout            int      `json:"timeout"`
	MediaType          string   `json:"mediaType"`
	PlatformID         int      `json:"platformID"`
	SessionType        int      `json:"sessionType"`
	InitiateTime       int64    `json:"initiateTime"`
	BusyLineUserIDList []string `json:"busyLineUserIDList"`
}
type MeetingInfo struct {
	RoomID                      string   `json:"roomID"`
	MeetingName                 string   `json:"meetingName"`
	Ex                          string   `json:"ex"`
	HostUserID                  string   `json:"hostUserID"`
	InviteeUserIDList           []string `json:"inviteeUserIDList"`
	CreateTime                  int64    `json:"createTime"`
	StartTime                   int64    `json:"startTime"`
	EndTime                     int64    `json:"endTime"`
	ParticipantCanUnmuteSelf    bool     `json:"participantCanUnmuteSelf"`
	ParticipantCanEnableVideo   bool     `json:"participantCanEnableVideo"`
	OnlyHostInviteUser          bool     `json:"onlyHostInviteUser"`
	OnlyHostShareScreen         bool     `json:"onlyHostShareScreen"`
	JoinDisableMicrophone       bool     `json:"joinDisableMicrophone"`
	JoinDisableVideo            bool     `json:"joinDisableVideo"`
	IsMuteAllVideo              bool     `json:"isMuteAllVideo"`
	IsMuteAllMicrophone         bool     `json:"isMuteAllMicrophone"`
	CanScreenUserIDList         []string `json:"canScreenUserIDList"`
	DisableMicrophoneUserIDList []string `json:"disableMicrophoneUserIDList"`
	DisableVideoUserIDList      []string `json:"disableVideoUserIDList"`
	PinedUserIDList             []string `json:"pinedUserIDList"`
	BeWatchedUserIDList         []string `json:"beWatchedUserIDList"`
}
type pendingEvent struct {
	Key, Name string
	Users     []string
	Data      any
	Delivered bool
}
type Room struct {
	RoomID           string            `json:"roomID"`
	MediaRoom        string            `json:"-"`
	Kind             string            `json:"kind"`
	State            string            `json:"state"`
	Invitation       Invitation        `json:"invitation"`
	MeetingInfo      MeetingInfo       `json:"meetingInfo"`
	Joined           []string          `json:"joined"`
	ParticipantState map[string]string `json:"participantState"`
	AcceptedPlatform map[string]int    `json:"-"`
	CurrentLease     map[string]string `json:"_currentLease"`
	CreateTime       int64             `json:"createTime"`
	EndedAt          int64             `json:"endedAt"`
	Reason           string            `json:"reason"`
	DeletedBy        []string          `json:"-"`
	Outbox           []pendingEvent    `json:"-"`
	MediaCleaned     bool              `json:"-"`
	RecordDeleted    bool              `json:"recordDeleted"`
}

// Private persistence uses a separate wire type, so secrets and outbox data do
// not leak through API JSON. JSON is also the store payload encoding.
type roomStored Room

func (r Room) MarshalJSON() ([]byte, error) {
	type public Room
	return json.Marshal(struct {
		public
		MediaRoom        string         `json:"_mediaRoom"`
		AcceptedPlatform map[string]int `json:"_acceptedPlatform"`
		DeletedBy        []string       `json:"_deletedBy"`
		Outbox           []pendingEvent `json:"_outbox"`
		MediaCleaned     bool           `json:"_mediaCleaned"`
	}{public(r), r.MediaRoom, r.AcceptedPlatform, r.DeletedBy, r.Outbox, r.MediaCleaned})
}
func (r *Room) UnmarshalJSON(b []byte) error {
	type public Room
	v := struct {
		public
		MediaRoom        string         `json:"_mediaRoom"`
		AcceptedPlatform map[string]int `json:"_acceptedPlatform"`
		DeletedBy        []string       `json:"_deletedBy"`
		Outbox           []pendingEvent `json:"_outbox"`
		MediaCleaned     bool           `json:"_mediaCleaned"`
	}{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*r = Room(v.public)
	r.MediaRoom = v.MediaRoom
	r.AcceptedPlatform = v.AcceptedPlatform
	r.DeletedBy = v.DeletedBy
	r.Outbox = v.Outbox
	r.MediaCleaned = v.MediaCleaned
	return nil
}
func (r Room) ended() bool {
	return r.State == "ended" || r.State == "cancelled" || r.State == "timed_out" || r.State == "rejected"
}
func (r Room) host() string {
	if r.Kind == "meeting" {
		return r.MeetingInfo.HostUserID
	}
	return r.Invitation.InviterUserID
}
func (r Room) audience() []string {
	return unique(append(append([]string{r.host()}, r.Invitation.InviteeUserIDList...), r.MeetingInfo.InviteeUserIDList...))
}
func (r Room) eligible(user string) bool {
	return contains(r.audience(), user) || contains(r.Joined, user)
}
func (r *Room) event(name string, users []string, data any) {
	r.Outbox = append(r.Outbox, pendingEvent{Key: r.RoomID + ":" + newID(), Name: name, Users: unique(users), Data: data})
}
func (r Room) sources(user string) []string {
	if r.Kind != "meeting" {
		if r.Invitation.MediaType == "audio" {
			return []string{"microphone"}
		}
		return []string{"microphone", "camera", "screen_share", "screen_share_audio"}
	}
	m := r.MeetingInfo
	host := user == m.HostUserID
	sources := []string{}
	if host || (m.ParticipantCanUnmuteSelf && !m.IsMuteAllMicrophone && !contains(m.DisableMicrophoneUserIDList, user)) {
		sources = append(sources, "microphone")
	}
	if host || (m.ParticipantCanEnableVideo && !m.IsMuteAllVideo && !contains(m.DisableVideoUserIDList, user)) {
		sources = append(sources, "camera")
	}
	if host || (!m.OnlyHostShareScreen || contains(m.CanScreenUserIDList, user)) {
		sources = append(sources, "screen_share", "screen_share_audio")
	}
	return sources
}
func (r Room) view() map[string]any {
	return map[string]any{"invitation": r.Invitation, "roomID": r.RoomID, "state": r.State, "ended": r.ended(), "meetingInfo": r.MeetingInfo, "participant": r.Joined, "participantState": r.ParticipantState}
}
func signalData(r Room, actor string) map[string]any {
	return map[string]any{"invitation": r.Invitation, "invitationInfo": r.Invitation, "roomID": r.RoomID, "opUserID": actor, "inviteeUserID": actor, "userID": actor, "opPlatformID": r.AcceptedPlatform[actor], "acceptedPlatform": r.AcceptedPlatform, "state": r.State, "meetingInfo": r.MeetingInfo, "participant": r.Joined, "participantState": r.ParticipantState}
}
func (s *Server) roomMember(ctx context.Context, a Actor, r Room) bool {
	if r.Kind != "call" || r.Invitation.SessionType != 3 || r.ended() {
		return r.eligible(a.UserID)
	}
	us, err := s.IM.GroupMembers(ctx, r.Invitation.GroupID)
	if err != nil {
		return false
	}
	for _, u := range us {
		if u.UserID == a.UserID {
			return true
		}
	}
	return false
}
func (s *Server) activeInvitation(r Room, user string) bool {
	return r.Kind == "call" && !r.ended() && r.Invitation.InviterUserID != user && r.ParticipantState[user] == "invited" && s.now().UnixMilli() < r.Invitation.InitiateTime+int64(r.Invitation.Timeout)*1000
}
func (s *Server) rooms(ctx context.Context) ([]Room, error) {
	ds, err := s.Store.List(ctx, "room", "")
	if err != nil {
		return nil, err
	}
	rs := []Room{}
	for _, d := range ds {
		var r Room
		if err = json.Unmarshal(d.Data, &r); err != nil {
			return nil, err
		}
		rs = append(rs, r)
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].CreateTime > rs[j].CreateTime })
	return rs, nil
}
func (s *Server) signaling(ctx context.Context, a Actor, path string, q request) (any, error) {
	action := strings.TrimPrefix(path, "/signaling/")
	if action == "signal_message_assemble" || action == "signal_send_message" {
		var envelope request
		if err := q.decode("signalReq", &envelope); err != nil {
			return nil, err
		}
		for _, k := range []string{"invite", "inviteInGroup", "accept", "reject", "cancel", "hungUp", "getTokenByRoomID"} {
			if raw, ok := envelope[k]; ok {
				var inner request
				if err := json.Unmarshal(raw, &inner); err != nil {
					return nil, bad("invalid signal payload")
				}
				v, err := s.signaling(ctx, a, "/signaling/"+k, inner)
				return map[string]any{"signalResp": map[string]any{k: v}}, err
			}
		}
		return nil, bad("signal action required")
	}
	if action == "invite" || action == "inviteInGroup" {
		return s.invite(ctx, a, q)
	}
	if action == "signal_create_meeting" {
		return s.createMeeting(ctx, a, q)
	}
	if action == "signal_get_meetings" {
		rs, err := s.rooms(ctx)
		if err != nil {
			return nil, err
		}
		ms := []MeetingInfo{}
		for _, r := range rs {
			if r.Kind == "meeting" && !r.ended() && r.eligible(a.UserID) {
				ms = append(ms, r.MeetingInfo)
			}
		}
		return map[string]any{"meetingInfoList": ms}, nil
	}
	if action == "get_signal_invitation_info_start_app" {
		rs, err := s.rooms(ctx)
		if err != nil {
			return nil, err
		}
		for _, r := range rs {
			if s.activeInvitation(r, a.UserID) {
				return map[string]any{"invitation": r.Invitation, "state": r.State, "ended": false, "offlinePushInfo": map[string]any{}}, nil
			}
		}
		return map[string]any{"invitation": nil, "ended": true}, nil
	}
	if action == "signal_get_rooms" || action == "signal_get_room_by_group_id" {
		rs, err := s.rooms(ctx)
		if err != nil {
			return nil, err
		}
		list := []any{}
		for _, r := range rs {
			if !s.roomMember(ctx, a, r) {
				continue
			}
			if action == "signal_get_rooms" && !contains(q.strings("roomIDs"), r.RoomID) {
				continue
			}
			if action == "signal_get_room_by_group_id" && (r.Invitation.GroupID != q.str("groupID") || r.ended()) {
				continue
			}
			list = append(list, r.view())
		}
		if action == "signal_get_room_by_group_id" {
			if len(list) > 0 {
				return list[0], nil
			}
			return map[string]any{"roomID": "", "participant": []any{}}, nil
		}
		return map[string]any{"roomList": list}, nil
	}
	roomID := q.str("roomID")
	var invitation Invitation
	if q.has("invitation") {
		if err := q.decode("invitation", &invitation); err != nil {
			return nil, err
		}
		if roomID == "" {
			roomID = invitation.RoomID
		}
	}
	if roomID == "" {
		return nil, bad("roomID required")
	}
	r, _, err := load[Room](ctx, s.Store, "room:"+roomID)
	if err != nil {
		return nil, err
	}
	if !s.roomMember(ctx, a, r) {
		return nil, forbidden("room invitation required")
	}
	if action == "get_signal_invitation_info" {
		return map[string]any{"invitationInfo": r.Invitation, "offlinePushInfo": map[string]any{}, "state": r.State, "ended": r.ended(), "meetingInfo": r.MeetingInfo, "participantState": r.ParticipantState, "participant": r.Joined, "activeInvitationForActor": s.activeInvitation(r, a.UserID)}, nil
	}
	if action == "signal_join_meeting" || action == "accept" {
		return s.joinRoom(ctx, a, r, action)
	}
	if action == "getTokenByRoomID" || action == "signal_get_token_by_room_id" {
		if r.ended() {
			return nil, forbidden("room is closed")
		}
		if r.Kind == "call" && r.Invitation.SessionType == 3 && r.State == "active" && !contains(r.Joined, a.UserID) {
			return s.joinRoom(ctx, a, r, "getTokenByRoomID")
		}
		if r.AcceptedPlatform[a.UserID] != a.Platform {
			return nil, forbidden("another platform accepted this call")
		}
		return s.issueMediaToken(ctx, a, r)
	}
	if action == "signal_update_meeting_info" {
		return s.updateMeeting(ctx, a, r, q)
	}
	if action == "signal_invite_meeting" || action == "register_meeting_invitation" {
		return s.inviteMeeting(ctx, a, r, q)
	}
	if action == "signal_operate_stream" {
		return s.operateStream(ctx, a, r, q)
	}
	if action == "signal_send_custom_signal" {
		if !contains(r.Joined, a.UserID) || r.ended() {
			return nil, forbidden("active membership required")
		}
		r, err = mutate[Room](ctx, s.Store, "room:"+roomID, func(x *Room) error {
			if x.ended() {
				return forbidden("room closed")
			}
			x.event("OnReceiveCustomSignal", x.audience(), map[string]any{"roomID": roomID, "userID": a.UserID, "customInfo": q.str("customInfo")})
			return nil
		})
		if err == nil {
			err = s.dispatchRoom(ctx, r)
		}
		return map[string]any{}, err
	}
	switch action {
	case "cancel", "reject", "hungUp", "signal_close_room":
		r, err = mutate[Room](ctx, s.Store, "room:"+roomID, func(x *Room) error {
			if x.ended() {
				return nil
			}
			switch action {
			case "cancel":
				if x.host() != a.UserID {
					return forbidden("only caller can cancel")
				}
				if x.State == "active" {
					return conflict("accepted call must be hung up")
				}
				x.State = "cancelled"
				x.event("OnInvitationCancelled", x.audience(), signalData(*x, a.UserID))
			case "reject":
				if a.UserID == x.host() {
					return forbidden("caller cannot reject own invitation")
				}
				if x.ParticipantState[a.UserID] != "invited" {
					return conflict("invitation already handled")
				}
				x.ParticipantState[a.UserID] = "rejected"
				x.AcceptedPlatform[a.UserID] = a.Platform
				all := true
				for _, u := range x.Invitation.InviteeUserIDList {
					if x.ParticipantState[u] == "invited" || x.ParticipantState[u] == "accepted" {
						all = false
					}
				}
				if all {
					x.State = "rejected"
				}
				x.event("OnInviteeRejected", x.audience(), signalData(*x, a.UserID))
			case "signal_close_room":
				if x.host() != a.UserID {
					return forbidden("only host can close room")
				}
				x.State = "ended"
				x.event("OnHangUp", x.audience(), signalData(*x, a.UserID))
			case "hungUp":
				if !contains(x.Joined, a.UserID) {
					return forbidden("participant has not joined")
				}
				x.Joined = without(x.Joined, a.UserID)
				x.ParticipantState[a.UserID] = "left"
				if x.Kind == "call" && (x.Invitation.SessionType == 1 || len(x.Joined) == 0) {
					x.State = "ended"
				}
				x.event("OnHangUp", x.audience(), signalData(*x, a.UserID))
				x.event("OnRoomParticipantDisconnected", x.audience(), signalData(*x, a.UserID))
			}
			if x.ended() {
				x.EndedAt = s.now().Unix()
				x.Reason = action
				if x.Kind == "meeting" {
					x.MeetingInfo.EndTime = x.EndedAt
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		_ = s.dispatchRoom(ctx, r)
		if r.ended() {
			if err = s.cleanRoom(ctx, r); err != nil {
				return nil, err
			}
		} else if action == "hungUp" {
			if err = s.Media.Remove(ctx, r.MediaRoom, a.UserID); err != nil {
				return nil, err
			}
			if r.Kind == "call" {
				_ = s.releaseCall(ctx, a.UserID, r.RoomID)
			}
		}
		return map[string]any{}, nil
	}
	return nil, missing("unknown signaling action")
}
func (s *Server) invite(ctx context.Context, a Actor, q request) (any, error) {
	if err := checkActor(a, q.str("userID")); err != nil {
		return nil, err
	}
	var inv Invitation
	if err := q.decode("invitation", &inv); err != nil {
		return nil, err
	}
	if inv.InviterUserID != "" && inv.InviterUserID != a.UserID {
		return nil, forbidden("caller identity mismatch")
	}
	inv.InviterUserID = a.UserID
	inv.PlatformID = a.Platform
	inv.InitiateTime = s.now().UnixMilli()
	inv.InviteeUserIDList = without(inv.InviteeUserIDList, a.UserID)
	if len(inv.InviteeUserIDList) == 0 || len(inv.InviteeUserIDList) > 200 {
		return nil, bad("invite 1-200 participants")
	}
	if inv.MediaType != "audio" && inv.MediaType != "video" {
		return nil, bad("mediaType must be audio or video")
	}
	if inv.Timeout <= 0 {
		inv.Timeout = 60
	}
	if inv.Timeout > 300 {
		inv.Timeout = 300
	}
	if inv.SessionType != 1 && inv.SessionType != 3 {
		return nil, bad("invalid call session type")
	}
	if inv.SessionType == 1 && len(inv.InviteeUserIDList) != 1 {
		return nil, bad("single call requires one invitee")
	}
	if _, err := s.checkedUsers(ctx, inv.InviteeUserIDList); err != nil {
		return nil, err
	}
	if inv.RoomID != "" {
		if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`).MatchString(inv.RoomID) {
			return nil, bad("invalid room ID")
		}
		old, _, err := load[Room](ctx, s.Store, "room:"+inv.RoomID)
		if err == nil {
			if old.Kind != "call" || old.host() != a.UserID || old.ended() {
				return nil, conflict("room ID already used")
			}
			if old.Invitation.MediaType != inv.MediaType || old.Invitation.GroupID != inv.GroupID {
				return nil, conflict("room ID belongs to a different invitation")
			}
			return s.issueMediaToken(ctx, a, old)
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}
	if inv.SessionType == 3 {
		us, err := s.IM.GroupMembers(ctx, inv.GroupID)
		if err != nil {
			return nil, err
		}
		members := []string{}
		for _, u := range us {
			members = append(members, u.UserID)
		}
		for _, u := range append([]string{a.UserID}, inv.InviteeUserIDList...) {
			if !contains(members, u) {
				return nil, forbidden("all group call users must be group members")
			}
		}
	}
	rs, err := s.rooms(ctx)
	if err != nil {
		return nil, err
	}
	busy := []string{}
	for _, r := range rs {
		if !r.ended() && r.Kind == "call" {
			if contains(r.Joined, a.UserID) {
				return nil, conflict("caller is already in a call")
			}
			for _, id := range inv.InviteeUserIDList {
				if contains(r.Joined, id) {
					busy = append(busy, id)
				}
			}
		}
	}
	inv.BusyLineUserIDList = unique(busy)
	inv.InviteeUserIDList = without(inv.InviteeUserIDList, busy...)
	if len(inv.InviteeUserIDList) == 0 {
		return nil, conflict("all invitees are busy")
	}
	if inv.RoomID == "" {
		inv.RoomID = newID()
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`).MatchString(inv.RoomID) {
		return nil, bad("invalid room ID")
	}
	r := Room{RoomID: inv.RoomID, MediaRoom: "ent_" + inv.RoomID + "_" + newID(), Kind: "call", State: "invited", Invitation: inv, Joined: []string{a.UserID}, ParticipantState: map[string]string{a.UserID: "accepted"}, AcceptedPlatform: map[string]int{a.UserID: a.Platform}, CreateTime: s.now().Unix()}
	for _, u := range inv.InviteeUserIDList {
		r.ParticipantState[u] = "invited"
	}
	r.event("OnReceiveNewInvitation", inv.InviteeUserIDList, signalData(r, a.UserID))
	if err = s.acquireCall(ctx, a.UserID, r.RoomID); err != nil {
		return nil, err
	}
	if err = s.Store.Save(ctx, document("room:"+r.RoomID, "room", a.UserID, r), 0); err != nil {
		s.releaseUnjoinedCall(ctx, a.UserID, r.RoomID)
		return nil, err
	}
	_ = s.dispatchRoom(ctx, r)
	v, err := s.issueMediaToken(ctx, a, r)
	if err != nil {
		return nil, err
	}
	data := v.(map[string]any)
	data["busyLineUserIDList"] = inv.BusyLineUserIDList
	data["invitation"] = inv
	return data, nil
}
func (s *Server) createMeeting(ctx context.Context, a Actor, q request) (any, error) {
	if h := q.str("meetingHostUserID"); h != "" && h != a.UserID {
		return nil, forbidden("host identity mismatch")
	}
	name := strings.TrimSpace(q.str("meetingName"))
	if name == "" || len(name) > 300 {
		return nil, bad("meeting name required")
	}
	invitees := without(q.strings("inviteeUserIDList"), a.UserID)
	if len(invitees) > 200 {
		return nil, bad("max 200 invitees")
	}
	if _, err := s.checkedUsers(ctx, invitees); err != nil {
		return nil, err
	}
	start := q.num("startTime")
	if start == 0 {
		start = s.now().Unix()
	}
	duration := q.num("meetingDuration")
	if duration <= 0 {
		duration = 3600
	}
	if duration > 7*86400 {
		return nil, bad("meeting duration too long")
	}
	id := newID()
	m := MeetingInfo{RoomID: id, MeetingName: name, HostUserID: a.UserID, InviteeUserIDList: invitees, Ex: q.str("ex"), CreateTime: s.now().Unix(), StartTime: start, EndTime: start + duration, ParticipantCanUnmuteSelf: true, ParticipantCanEnableVideo: true, CanScreenUserIDList: []string{}, DisableMicrophoneUserIDList: []string{}, DisableVideoUserIDList: []string{}, PinedUserIDList: []string{}, BeWatchedUserIDList: []string{}}
	r := Room{RoomID: id, MediaRoom: "ent_" + id + "_" + newID(), Kind: "meeting", State: "scheduled", MeetingInfo: m, Joined: []string{}, ParticipantState: map[string]string{}, AcceptedPlatform: map[string]int{}, CreateTime: s.now().Unix()}
	if start <= s.now().Unix() {
		r.State = "active"
		r.Joined = []string{a.UserID}
		r.ParticipantState[a.UserID] = "accepted"
		r.AcceptedPlatform[a.UserID] = a.Platform
	}
	r.event("OnMeetingInfoChanged", r.audience(), map[string]any{"meetingInfo": m, "roomID": id})
	if err := s.Store.Save(ctx, document("room:"+id, "room", a.UserID, r), 0); err != nil {
		return nil, err
	}
	_ = s.dispatchRoom(ctx, r)
	if r.State == "scheduled" {
		return map[string]any{"roomID": id, "token": "", "liveURL": s.Media.GatewayURL(), "meetingInfo": m}, nil
	}
	return s.issueMediaToken(ctx, a, r)
}
func (s *Server) joinRoom(ctx context.Context, a Actor, r Room, action string) (any, error) {
	roomID := r.RoomID
	reserved := r.Kind == "call" && !contains(r.Joined, a.UserID)
	if reserved {
		if err := s.acquireCall(ctx, a.UserID, r.RoomID); err != nil {
			return nil, err
		}
	}
	groupJoin := action == "getTokenByRoomID" && r.Kind == "call" && r.Invitation.SessionType == 3 && r.State == "active" && s.roomMember(ctx, a, r)
	r, err := mutate[Room](ctx, s.Store, "room:"+r.RoomID, func(x *Room) error {
		if x.ended() {
			return forbidden("room is closed")
		}
		if !x.eligible(a.UserID) && !groupJoin {
			return forbidden("invitation required")
		}
		if x.Kind == "meeting" {
			if action != "signal_join_meeting" {
				return bad("use meeting join")
			}
			if s.now().Unix() < x.MeetingInfo.StartTime {
				return conflict("meeting has not started")
			}
			if s.now().Unix() >= x.MeetingInfo.EndTime {
				return forbidden("meeting has expired")
			}
		} else {
			if x.ParticipantState[a.UserID] == "rejected" && !groupJoin {
				return forbidden("invitation rejected")
			}
			if !groupJoin && s.now().UnixMilli() > x.Invitation.InitiateTime+int64(x.Invitation.Timeout)*1000 {
				return forbidden("invitation expired")
			}
			if contains(x.Joined, a.UserID) && x.AcceptedPlatform[a.UserID] != a.Platform {
				return conflict("call accepted on another platform")
			}
		}
		already := contains(x.Joined, a.UserID)
		if groupJoin {
			x.Invitation.InviteeUserIDList = unique(append(x.Invitation.InviteeUserIDList, a.UserID))
		}
		x.Joined = unique(append(x.Joined, a.UserID))
		x.ParticipantState[a.UserID] = "accepted"
		x.AcceptedPlatform[a.UserID] = a.Platform
		x.State = "active"
		if !already {
			if x.Kind == "call" {
				x.event("OnInviteeAccepted", x.audience(), signalData(*x, a.UserID))
			}
			x.event("OnRoomParticipantConnected", x.audience(), signalData(*x, a.UserID))
		}
		return nil
	})
	if err != nil {
		if reserved {
			s.releaseUnjoinedCall(ctx, a.UserID, roomID)
		}
		return nil, err
	}
	_ = s.dispatchRoom(ctx, r)
	return s.issueMediaToken(ctx, a, r)
}
func boolValue(q request, k string) (bool, bool) {
	raw, ok := q[k]
	if !ok {
		return false, false
	}
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return b, true
	}
	var w struct {
		Value bool `json:"value"`
	}
	if json.Unmarshal(raw, &w) == nil {
		return w.Value, true
	}
	return false, false
}
func stringValue(q request, k string) (string, bool) {
	raw, ok := q[k]
	if !ok {
		return "", false
	}
	var v string
	if json.Unmarshal(raw, &v) == nil {
		return v, true
	}
	var w struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &w) == nil {
		return w.Value, true
	}
	return "", false
}
func intValue(q request, k string) (int64, bool) {
	raw, ok := q[k]
	if !ok {
		return 0, false
	}
	var v int64
	if json.Unmarshal(raw, &v) == nil {
		return v, true
	}
	var w struct {
		Value int64 `json:"value"`
	}
	if json.Unmarshal(raw, &w) == nil {
		return w.Value, true
	}
	return 0, false
}
func (s *Server) updateMeeting(ctx context.Context, a Actor, r Room, q request) (any, error) {
	if r.Kind != "meeting" || r.host() != a.UserID {
		return nil, forbidden("meeting host required")
	}
	if q.has("inviteeUserIDList") {
		if _, err := s.checkedUsers(ctx, q.strings("inviteeUserIDList")); err != nil {
			return nil, err
		}
	}
	r, err := mutate[Room](ctx, s.Store, "room:"+r.RoomID, func(x *Room) error {
		if x.ended() {
			return forbidden("meeting is closed")
		}
		m := &x.MeetingInfo
		if v, ok := stringValue(q, "meetingName"); ok {
			if strings.TrimSpace(v) == "" {
				return bad("meeting name cannot be empty")
			}
			m.MeetingName = v
		}
		if v, ok := stringValue(q, "ex"); ok {
			m.Ex = v
		}
		if v, ok := intValue(q, "startTime"); ok {
			if x.State == "active" {
				return conflict("cannot reschedule active meeting")
			}
			m.StartTime = v
		}
		if v, ok := intValue(q, "endTime"); ok {
			m.EndTime = v
		}
		if m.EndTime <= m.StartTime {
			return bad("endTime must follow startTime")
		}
		if q.has("inviteeUserIDList") {
			m.InviteeUserIDList = without(q.strings("inviteeUserIDList"), m.HostUserID)
		}
		for k, p := range map[string]*bool{"participantCanUnmuteSelf": &m.ParticipantCanUnmuteSelf, "participantCanEnableVideo": &m.ParticipantCanEnableVideo, "onlyHostInviteUser": &m.OnlyHostInviteUser, "onlyHostShareScreen": &m.OnlyHostShareScreen, "joinDisableMicrophone": &m.JoinDisableMicrophone, "joinDisableVideo": &m.JoinDisableVideo, "isMuteAllVideo": &m.IsMuteAllVideo, "isMuteAllMicrophone": &m.IsMuteAllMicrophone} {
			if v, ok := boolValue(q, k); ok {
				*p = v
			}
		}
		for k, p := range map[string]*[]string{"canScreenUserIDList": &m.CanScreenUserIDList, "disableMicrophoneUserIDList": &m.DisableMicrophoneUserIDList, "disableVideoUserIDList": &m.DisableVideoUserIDList, "pinedUserIDList": &m.PinedUserIDList, "beWatchedUserIDList": &m.BeWatchedUserIDList} {
			if q.has(k) {
				*p = q.strings(k)
			}
			title := strings.ToUpper(k[:1]) + k[1:]
			*p = without(append(*p, q.strings("add"+title)...), q.strings("reduce"+title)...)
			for _, user := range *p {
				if !x.eligible(user) {
					return bad("meeting control references a nonmember")
				}
			}
		}
		x.event("OnMeetingInfoChanged", x.audience(), map[string]any{"meetingInfo": *m, "roomID": x.RoomID})
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, u := range r.Joined {
		if err = s.Media.Permissions(ctx, r.MediaRoom, u, r.sources(u)); err != nil {
			return nil, err
		}
	}
	_ = s.dispatchRoom(ctx, r)
	return map[string]any{"meetingInfo": r.MeetingInfo}, nil
}
func (s *Server) inviteMeeting(ctx context.Context, a Actor, r Room, q request) (any, error) {
	if r.Kind != "meeting" || r.ended() || !r.eligible(a.UserID) {
		return nil, forbidden("active meeting invitation required")
	}
	if r.MeetingInfo.OnlyHostInviteUser && r.host() != a.UserID {
		return nil, forbidden("only host may invite participants")
	}
	ids := q.strings("userIDs")
	if len(ids) == 0 {
		ids = q.strings("inviteeUserIDList")
	}
	groups := q.strings("groupIDs")
	if g := q.str("groupID"); g != "" {
		groups = append(groups, g)
	}
	members, err := s.expandGroups(ctx, a, groups)
	if err != nil {
		return nil, err
	}
	ids = without(append(ids, members...), r.host())
	if len(ids) == 0 {
		return nil, bad("invitation recipients required")
	}
	if _, err = s.checkedUsers(ctx, ids); err != nil {
		return nil, err
	}
	r, err = mutate[Room](ctx, s.Store, "room:"+r.RoomID, func(x *Room) error {
		if x.ended() || !x.eligible(a.UserID) {
			return forbidden("meeting is no longer available")
		}
		if x.MeetingInfo.OnlyHostInviteUser && x.host() != a.UserID {
			return forbidden("only host may invite participants")
		}
		x.MeetingInfo.InviteeUserIDList = unique(append(x.MeetingInfo.InviteeUserIDList, ids...))
		if len(x.MeetingInfo.InviteeUserIDList) > 500 {
			return bad("meeting invitation limit is 500 users")
		}
		x.event("OnMeetingInfoChanged", x.audience(), map[string]any{"roomID": x.RoomID, "meetingInfo": x.MeetingInfo})
		return nil
	})
	if err != nil {
		return nil, err
	}
	_ = s.dispatchRoom(ctx, r)
	return map[string]any{"meetingInfo": r.MeetingInfo}, nil
}
func (s *Server) operateStream(ctx context.Context, a Actor, r Room, q request) (any, error) {
	if r.ended() || !contains(r.Joined, a.UserID) {
		return nil, forbidden("active participant required")
	}
	target := q.str("userID")
	if target == "" {
		target = a.UserID
	}
	all := q.boolean("muteAll")
	host := r.host() == a.UserID
	if (target != a.UserID || all) && !host {
		return nil, forbidden("host required to control other participants")
	}
	kind := q.str("streamType")
	if kind == "microphone" {
		kind = "audio"
	}
	if kind == "camera" {
		kind = "video"
	}
	if kind != "audio" && kind != "video" && kind != "screen" {
		return nil, bad("invalid stream type")
	}
	mute := q.boolean("mute")
	if r.Kind == "meeting" && !host && !mute {
		m := r.MeetingInfo
		if kind == "audio" && (!m.ParticipantCanUnmuteSelf || m.IsMuteAllMicrophone || contains(m.DisableMicrophoneUserIDList, a.UserID)) {
			return nil, forbidden("microphone locked by host")
		}
		if kind == "video" && (!m.ParticipantCanEnableVideo || m.IsMuteAllVideo || contains(m.DisableVideoUserIDList, a.UserID)) {
			return nil, forbidden("camera locked by host")
		}
		if kind == "screen" && m.OnlyHostShareScreen && !contains(m.CanScreenUserIDList, a.UserID) {
			return nil, forbidden("screen sharing locked by host")
		}
	}
	users := []string{target}
	if all {
		users = without(r.Joined, a.UserID)
	}
	for _, u := range users {
		if !contains(r.Joined, u) {
			return nil, bad("target has not joined")
		}
		if err := s.Media.Mute(ctx, r.MediaRoom, u, kind, mute); err != nil {
			return nil, err
		}
	}
	if r.Kind == "meeting" && host {
		r2, err := mutate[Room](ctx, s.Store, "room:"+r.RoomID, func(x *Room) error {
			m := &x.MeetingInfo
			if all {
				if kind == "audio" {
					m.IsMuteAllMicrophone = mute
				}
				if kind == "video" {
					m.IsMuteAllVideo = mute
				}
			} else {
				var list *[]string
				if kind == "audio" {
					list = &m.DisableMicrophoneUserIDList
				}
				if kind == "video" {
					list = &m.DisableVideoUserIDList
				}
				if list != nil {
					if mute {
						*list = unique(append(*list, target))
					} else {
						*list = without(*list, target)
					}
				}
			}
			x.event("OnMeetingInfoChanged", x.audience(), map[string]any{"roomID": x.RoomID, "meetingInfo": *m})
			return nil
		})
		if err != nil {
			return nil, err
		}
		r = r2
		for _, u := range users {
			if err = s.Media.Permissions(ctx, r.MediaRoom, u, r.sources(u)); err != nil {
				return nil, err
			}
		}
	}
	_, err := mutate[Room](ctx, s.Store, "room:"+r.RoomID, func(x *Room) error {
		x.event("OnStreamChanged", x.audience(), map[string]any{"roomID": x.RoomID, "streamType": kind, "userID": target, "mute": mute, "muteAll": all})
		return nil
	})
	if err == nil {
		current, _, e := load[Room](ctx, s.Store, "room:"+r.RoomID)
		if e == nil {
			_ = s.dispatchRoom(ctx, current)
		}
	}
	return map[string]any{}, err
}
func (s *Server) dispatchRoom(ctx context.Context, r Room) error {
	for _, e := range r.Outbox {
		if e.Delivered {
			continue
		}
		for _, user := range e.Users {
			if err := s.appendEvent(ctx, user, e.Key, e.Name, e.Data); err != nil {
				return err
			}
			s.queueHint(Actor{UserID: r.host(), Platform: r.Invitation.PlatformID}, user, e.Key, e.Name)
		}
		_, err := mutate[Room](ctx, s.Store, "room:"+r.RoomID, func(x *Room) error {
			pending := []pendingEvent{}
			for _, entry := range x.Outbox {
				if entry.Key != e.Key && !entry.Delivered {
					pending = append(pending, entry)
				}
			}
			x.Outbox = pending
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
func (s *Server) cleanRoom(ctx context.Context, r Room) error {
	if !r.ended() || r.MediaCleaned {
		return nil
	}
	if err := s.Media.Delete(ctx, r.MediaRoom); err != nil {
		return err
	}
	if r.Kind == "call" {
		for _, u := range r.audience() {
			if err := s.releaseCall(ctx, u, r.RoomID); err != nil {
				return err
			}
		}
	}
	_, err := mutate[Room](ctx, s.Store, "room:"+r.RoomID, func(x *Room) error { x.MediaCleaned = true; return nil })
	return err
}
func (s *Server) reconcileRooms(ctx context.Context) error {
	rs, err := s.rooms(ctx)
	if err != nil {
		return err
	}
	for _, r := range rs {
		expire := r.Kind == "call" && r.State == "invited" && s.now().UnixMilli() >= r.Invitation.InitiateTime+int64(r.Invitation.Timeout)*1000
		finish := r.Kind == "meeting" && !r.ended() && s.now().Unix() >= r.MeetingInfo.EndTime
		if expire || finish {
			r, err = mutate[Room](ctx, s.Store, "room:"+r.RoomID, func(x *Room) error {
				if x.ended() {
					return nil
				}
				if expire && x.State != "invited" {
					return nil
				}
				x.State = "ended"
				name := "OnHangUp"
				if expire {
					x.State = "timed_out"
					name = "OnInvitationTimeout"
				}
				x.EndedAt = s.now().Unix()
				x.Reason = "timeout"
				x.event(name, x.audience(), signalData(*x, ""))
				return nil
			})
			if err != nil {
				return err
			}
		}
		if err = s.dispatchRoom(ctx, r); err != nil {
			return err
		}
		if err = s.cleanRoom(ctx, r); err != nil {
			return err
		}
	}
	if s.lastSessionCheck.IsZero() || s.now().Sub(s.lastSessionCheck) >= 10*time.Second {
		s.lastSessionCheck = s.now()
		return s.revalidateMediaSessions(ctx)
	}
	return nil
}
func (s *Server) revalidateMediaSessions(ctx context.Context) error {
	if s.Cipher == nil {
		return nil
	}
	ds, err := s.Store.List(ctx, "media_grant", "")
	if err != nil {
		return err
	}
	for _, d := range ds {
		var g mediaGrant
		if json.Unmarshal(d.Data, &g) != nil {
			continue
		}
		if g.Revoked {
			continue
		}
		room, _, e := load[Room](ctx, s.Store, "room:"+g.RoomID)
		if e != nil {
			continue
		}
		if room.ended() || room.CurrentLease[g.UserID] != strings.TrimPrefix(d.ID, "media_grant:") {
			g.Revoked = true
			d.Data, _ = json.Marshal(g)
			if err = s.Store.Save(ctx, d, d.Version); err != nil && !errors.Is(err, ErrConflict) {
				return err
			}
			continue
		}
		a, err := s.Cipher.open(g.Ciphertext)
		if err != nil {
			return err
		}
		req, _ := http.NewRequestWithContext(ctx, "POST", "http://internal/session", nil)
		req.Header.Set("token", a.IMToken)
		req.Header.Set("chatToken", a.ChatToken)
		req.Header.Set("X-Real-IP", a.IP)
		_, err = s.Auth.Authenticate(ctx, req, false)
		if err == nil && room.Kind == "call" && room.Invitation.SessionType == 3 {
			members, e := s.IM.GroupMembers(ctx, room.Invitation.GroupID)
			if e != nil {
				continue
			}
			allowed := false
			for _, u := range members {
				if u.UserID == g.UserID {
					allowed = true
				}
			}
			if !allowed {
				err = forbidden("group membership ended")
			}
		}
		if err == nil {
			continue
		}
		var ae *apiError
		if !errors.As(err, &ae) || ae.Status >= 500 {
			continue
		}
		current, _, e := load[Room](ctx, s.Store, "room:"+g.RoomID)
		if e != nil {
			return e
		}
		if current.CurrentLease[g.UserID] != strings.TrimPrefix(d.ID, "media_grant:") {
			g.Revoked = true
			d.Data, _ = json.Marshal(g)
			if e = s.Store.Save(ctx, d, d.Version); e != nil && !errors.Is(e, ErrConflict) {
				return e
			}
			continue
		}
		if err = s.Media.Remove(ctx, g.MediaRoom, g.UserID); err != nil {
			return err
		}
		updated, e := mutate[Room](ctx, s.Store, "room:"+g.RoomID, func(r *Room) error {
			if r.CurrentLease[g.UserID] != strings.TrimPrefix(d.ID, "media_grant:") {
				return nil
			}
			r.Joined = without(r.Joined, g.UserID)
			r.ParticipantState[g.UserID] = "left"
			r.event("OnRoomParticipantDisconnected", r.audience(), signalData(*r, g.UserID))
			if r.Kind == "call" && (r.Invitation.SessionType == 1 || len(r.Joined) == 0) {
				r.State = "ended"
				r.EndedAt = s.now().Unix()
				r.Reason = "session_revoked"
				r.event("OnHangUp", r.audience(), signalData(*r, g.UserID))
			}
			return nil
		})
		if e != nil {
			return e
		}
		_ = s.dispatchRoom(ctx, updated)
		if updated.Kind == "call" {
			_ = s.releaseCall(ctx, g.UserID, g.RoomID)
		}
		if e = s.cleanRoom(ctx, updated); e != nil {
			return e
		}
		g.Revoked = true
		d.Data, _ = json.Marshal(g)
		if err = s.Store.Save(ctx, d, d.Version); err != nil && !errors.Is(err, ErrConflict) {
			return err
		}
	}
	return nil
}
func (s *Server) records(ctx context.Context, a Actor, path string, q request) (any, error) {
	allowed := map[string]bool{"/rtc/get_signal_invitation_records": true, "/rtc/delete_signal_records": true, "/rtc/get_meeting_records": true, "/rtc/delete_meeting_records": true}
	if !allowed[path] {
		return nil, missing("unknown record operation")
	}
	rs, err := s.rooms(ctx)
	if err != nil {
		return nil, err
	}
	meeting := strings.Contains(path, "meeting")
	remove := strings.Contains(path, "delete")
	if remove {
		ids := q.strings("roomIDs")
		if !meeting {
			ids = q.strings("sIDs")
		}
		for _, id := range ids {
			_, err = mutate[Room](ctx, s.Store, "room:"+id, func(r *Room) error {
				if !a.Admin && !r.eligible(a.UserID) {
					return forbidden("record participant required")
				}
				if a.Admin {
					r.RecordDeleted = true
				} else {
					r.DeletedBy = unique(append(r.DeletedBy, a.UserID))
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
		}
		return map[string]any{}, nil
	}
	result := []map[string]any{}
	profileIDs := []string{}
	groupIDs := []string{}
	for _, r := range rs {
		profileIDs = append(profileIDs, r.audience()...)
		if r.Invitation.GroupID != "" {
			groupIDs = append(groupIDs, r.Invitation.GroupID)
		}
	}
	profiles := map[string]User{}
	if us, e := s.IM.Users(ctx, unique(profileIDs)); e == nil {
		for _, u := range us {
			profiles[u.UserID] = u
		}
	}
	names := func(id string) string {
		if u, ok := profiles[id]; ok && u.Nickname != "" {
			return u.Nickname
		}
		return id
	}
	groupNames := map[string]string{}
	if resolver, ok := s.IM.(GroupResolver); ok {
		if gs, e := resolver.Groups(ctx, unique(groupIDs)); e == nil {
			for _, g := range gs {
				groupNames[g.GroupID] = g.GroupName
			}
		}
	}
	for _, r := range rs {
		if (r.Kind == "meeting") != meeting || (!a.Admin && !r.eligible(a.UserID)) || r.RecordDeleted || contains(r.DeletedBy, a.UserID) {
			continue
		}
		if start := q.num("startTime"); start > 0 && r.CreateTime < start {
			continue
		}
		if end := q.num("endTime"); end > 0 && r.CreateTime > end {
			continue
		}
		if host := q.str("hostUserID"); host != "" && r.host() != host {
			continue
		}
		if joined := q.str("joinedUserID"); joined != "" && r.ParticipantState[joined] != "accepted" && r.ParticipantState[joined] != "left" {
			continue
		}
		users := []map[string]any{}
		for u, status := range r.ParticipantState {
			if meeting && status != "accepted" && status != "left" {
				continue
			}
			n := 0
			switch status {
			case "accepted":
				n = 1
			case "rejected":
				n = 2
			case "left":
				n = 5
			}
			users = append(users, map[string]any{"userID": u, "nickname": names(u), "status": n, "state": status})
		}
		if meeting {
			status := 0
			if r.ended() {
				status = 1
			}
			result = append(result, map[string]any{"roomID": r.RoomID, "meetingName": r.MeetingInfo.MeetingName, "ex": r.MeetingInfo.Ex, "hostUserID": r.host(), "hostUserNickname": names(r.host()), "createTime": r.CreateTime, "startTime": r.MeetingInfo.StartTime, "endTime": r.MeetingInfo.EndTime, "joinedUsers": users, "status": status, "state": r.State, "fileRecords": []any{}})
		} else {
			if st := q.num("sessionType"); st > 0 && int64(r.Invitation.SessionType) != st {
				continue
			}
			if send := q.str("sendID"); send != "" && r.Invitation.InviterUserID != send {
				continue
			}
			if recv := q.str("recvID"); recv != "" && !contains(r.Invitation.InviteeUserIDList, recv) {
				continue
			}
			result = append(result, map[string]any{"roomID": r.RoomID, "sID": r.RoomID, "mediaType": r.Invitation.MediaType, "sessionType": r.Invitation.SessionType, "inviterUserID": r.Invitation.InviterUserID, "inviterUserNickname": names(r.Invitation.InviterUserID), "groupID": r.Invitation.GroupID, "groupName": groupNames[r.Invitation.GroupID], "inviterUsers": users, "createTime": r.CreateTime, "endTime": r.EndedAt, "state": r.State, "reason": r.Reason, "fileName": "", "size": "", "fileURL": ""})
		}
	}
	key := "signalRecords"
	if meeting {
		key = "meetingRecords"
	}
	return map[string]any{"total": len(result), key: page(result, q)}, nil
}
