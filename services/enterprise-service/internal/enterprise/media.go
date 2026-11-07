package enterprise

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"openim38.local/identity"
	"strings"
	"time"

	"github.com/livekit/protocol/auth"
)

type Media interface {
	Create(context.Context, string) error
	Delete(context.Context, string) error
	Remove(context.Context, string, string) error
	Token(string, string, []string, string, string) (string, error)
	ValidateToken(string) (string, string, string, error)
	Permissions(context.Context, string, string, []string) error
	Mute(context.Context, string, string, string, bool) error
	GatewayURL() string
	Proxy(http.ResponseWriter, *http.Request)
}
type LiveKit struct {
	Base, PublicURL, Key, Secret string
	HTTP                         *http.Client
	proxy                        *httputil.ReverseProxy
}

func NewLiveKit(base, public, key, secret string) (*LiveKit, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid LiveKit URL")
	}
	lk := &LiveKit{Base: strings.TrimSuffix(base, "/"), PublicURL: strings.TrimSuffix(public, "/"), Key: key, Secret: secret, HTTP: &http.Client{Timeout: 10 * time.Second}}
	lk.proxy = httputil.NewSingleHostReverseProxy(u)
	lk.proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) { writeError(w, e) }
	return lk, nil
}
func (l *LiveKit) GatewayURL() string { return l.PublicURL }
func (l *LiveKit) Proxy(w http.ResponseWriter, r *http.Request) {
	if identity.Required() && isSignalUpgrade(r) {
		l.proxySignal(w, r)
		return
	}
	l.proxy.ServeHTTP(w, r)
}
func (l *LiveKit) Token(room, user string, sources []string, lease, metadata string) (string, error) {
	g := &auth.VideoGrant{RoomJoin: true, Room: room, CanPublishSources: sources}
	g.SetCanPublish(len(sources) > 0)
	g.SetCanSubscribe(true)
	g.SetCanPublishData(true)
	g.SetCanUpdateOwnMetadata(false)
	return auth.NewAccessToken(l.Key, l.Secret).SetIdentity(user).SetName(user).SetMetadata(metadata).SetValidFor(10 * time.Minute).SetAttributes(map[string]string{"enterpriseLease": lease}).SetVideoGrant(g).ToJWT()
}
func (l *LiveKit) ValidateToken(token string) (string, string, string, error) {
	v, err := auth.ParseAPIToken(token)
	if err != nil || v.APIKey() != l.Key {
		return "", "", "", unauthorized("invalid media token")
	}
	claims, err := v.Verify(l.Secret)
	if err != nil {
		return "", "", "", unauthorized("expired media token")
	}
	if claims.Video == nil || !claims.Video.RoomJoin {
		return "", "", "", unauthorized("not a join token")
	}
	return claims.Video.Room, claims.Identity, claims.Attributes["enterpriseLease"], nil
}
func (l *LiveKit) call(ctx context.Context, method, room string, body, out any) error {
	jwt, err := auth.NewAccessToken(l.Key, l.Secret).SetValidFor(time.Minute).SetVideoGrant(&auth.VideoGrant{Room: room, RoomAdmin: true, RoomCreate: method == "CreateRoom" || method == "DeleteRoom", RoomList: method == "ListRooms"}).ToJWT()
	if err != nil {
		return err
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", l.Base+"/twirp/livekit.RoomService/"+method, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")
	res, err := l.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 400 {
		if method == "DeleteRoom" || method == "RemoveParticipant" || method == "UpdateParticipant" || method == "GetParticipant" || method == "ListParticipants" {
			var e struct {
				Code string `json:"code"`
			}
			_ = json.Unmarshal(raw, &e)
			if e.Code == "not_found" {
				return nil
			}
		}
		return fmt.Errorf("LiveKit %s failed: %d %s", method, res.StatusCode, string(raw))
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}
func (l *LiveKit) Create(ctx context.Context, room string) error {
	return l.call(ctx, "CreateRoom", room, map[string]any{"name": room, "emptyTimeout": 300, "departureTimeout": 20}, nil)
}
func (l *LiveKit) Delete(ctx context.Context, room string) error {
	return l.call(ctx, "DeleteRoom", room, map[string]string{"room": room}, nil)
}
func (l *LiveKit) Remove(ctx context.Context, room, user string) error {
	return l.call(ctx, "RemoveParticipant", room, map[string]string{"room": room, "identity": user}, nil)
}
func (l *LiveKit) Permissions(ctx context.Context, room, user string, sources []string) error {
	names := []string{}
	for _, v := range sources {
		switch v {
		case "microphone":
			names = append(names, "MICROPHONE")
		case "camera":
			names = append(names, "CAMERA")
		case "screen_share":
			names = append(names, "SCREEN_SHARE")
		case "screen_share_audio":
			names = append(names, "SCREEN_SHARE_AUDIO")
		}
	}
	return l.call(ctx, "UpdateParticipant", room, map[string]any{"room": room, "identity": user, "permission": map[string]any{"canPublish": len(sources) > 0, "canSubscribe": true, "canPublishData": true, "canPublishSources": names}}, nil)
}
func (l *LiveKit) Mute(ctx context.Context, room, user, kind string, mute bool) error {
	// Unlocking publication is distinct from forcing another person's device to
	// start recording. The client receives the state event and may unmute itself.
	if !mute {
		return nil
	}
	var p struct {
		Tracks []struct {
			SID    string `json:"sid"`
			Source string `json:"source"`
		} `json:"tracks"`
	}
	if err := l.call(ctx, "GetParticipant", room, map[string]string{"room": room, "identity": user}, &p); err != nil {
		return err
	}
	source := "MICROPHONE"
	if kind == "video" {
		source = "CAMERA"
	}
	if kind == "screen" {
		source = "SCREEN_SHARE"
	}
	for _, t := range p.Tracks {
		if t.Source == source {
			if err := l.call(ctx, "MutePublishedTrack", room, map[string]any{"room": room, "identity": user, "trackSid": t.SID, "muted": mute}, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

type mediaGrant struct {
	RoomID, MediaRoom, UserID string
	Ciphertext                []byte
	ExpiresAt                 int64
	Revoked                   bool
}
type sessionCipher struct{ aead cipher.AEAD }

func NewSessionCipher(keyHex string) (*sessionCipher, error) {
	b, err := hex.DecodeString(keyHex)
	if err != nil || len(b) != 32 {
		return nil, fmt.Errorf("ENTERPRISE_SESSION_KEY must be 64 hexadecimal characters")
	}
	block, err := aes.NewCipher(b)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	return &sessionCipher{gcm}, err
}
func (c *sessionCipher) seal(a Actor) ([]byte, error) {
	b, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, b, []byte("openim38-media-session")), nil
}
func (c *sessionCipher) open(b []byte) (Actor, error) {
	var a Actor
	n := c.aead.NonceSize()
	if len(b) < n {
		return a, fmt.Errorf("invalid encrypted media session")
	}
	raw, err := c.aead.Open(nil, b[:n], b[n:], []byte("openim38-media-session"))
	if err != nil {
		return a, err
	}
	err = json.Unmarshal(raw, &a)
	return a, err
}
func tokenHash(t string) string { b := sha256.Sum256([]byte(t)); return hex.EncodeToString(b[:]) }
func (s *Server) issueMediaToken(ctx context.Context, a Actor, r Room) (any, error) {
	if r.ended() {
		return nil, forbidden("room is closed")
	}
	if !contains(r.Joined, a.UserID) {
		return nil, forbidden("join or accept the invitation first")
	}
	if s.Cipher == nil {
		return nil, fmt.Errorf("media session cipher unavailable")
	}
	if err := s.Media.Create(ctx, r.MediaRoom); err != nil {
		return nil, err
	}
	lease := newID()
	meta, err := s.participantMetadata(ctx, a.UserID, r)
	if err != nil {
		return nil, err
	}
	token, err := s.Media.Token(r.MediaRoom, a.UserID, r.sources(a.UserID), lease, meta)
	if err != nil {
		return nil, err
	}
	// Persist only the trusted issuance context, never a caller-supplied Actor
	// field or an origin/source-IP inference. Background checks have no ingress.
	a.EntryZone = identity.EntryZoneFromContext(ctx)
	encrypted, err := s.Cipher.seal(a)
	if err != nil {
		return nil, err
	}
	grant := mediaGrant{RoomID: r.RoomID, MediaRoom: r.MediaRoom, UserID: a.UserID, Ciphertext: encrypted, ExpiresAt: s.now().Add(7 * 24 * time.Hour).Unix()}
	if err = s.Store.Save(ctx, document("media_grant:"+lease, "media_grant", a.UserID, grant), 0); err != nil && !errors.Is(err, ErrConflict) {
		return nil, err
	}
	if _, err = mutate[Room](ctx, s.Store, "room:"+r.RoomID, func(x *Room) error {
		if x.ended() || !contains(x.Joined, a.UserID) {
			return forbidden("room membership ended")
		}
		if x.CurrentLease == nil {
			x.CurrentLease = map[string]string{}
		}
		x.CurrentLease[a.UserID] = lease
		return nil
	}); err != nil {
		return nil, err
	}
	liveURL := s.Media.GatewayURL()
	if a.EntryOrigin != "" {
		liveURL = strings.Replace(a.EntryOrigin, "https://", "wss://", 1) + "/livekit"
	}
	return map[string]any{"token": token, "roomID": r.RoomID, "liveURL": liveURL, "meetingInfo": r.MeetingInfo}, nil
}
func (s *Server) mediaGateway(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("access_token")
	if token == "" {
		token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	if token == "" {
		writeError(w, unauthorized("media token required"))
		return
	}
	mediaRoom, user, lease, err := s.Media.ValidateToken(token)
	if err != nil {
		writeError(w, err)
		return
	}
	grant, _, err := load[mediaGrant](r.Context(), s.Store, "media_grant:"+lease)
	if err != nil || grant.MediaRoom != mediaRoom || grant.UserID != user {
		writeError(w, unauthorized("unregistered media token"))
		return
	}
	if grant.Revoked || s.now().Unix() > grant.ExpiresAt {
		writeError(w, unauthorized("media token expired or revoked"))
		return
	}
	room, _, err := load[Room](r.Context(), s.Store, "room:"+grant.RoomID)
	if err != nil || room.ended() || room.MediaRoom != grant.MediaRoom || !contains(room.Joined, grant.UserID) || room.CurrentLease[grant.UserID] != lease {
		writeError(w, forbidden("room membership ended"))
		return
	}
	actor, err := s.Cipher.open(grant.Ciphertext)
	if err != nil {
		writeError(w, err)
		return
	}
	if identity.Required() {
		currentIP, e := identity.ClientIP(r)
		if e != nil {
			writeError(w, forbidden("client address unavailable"))
			return
		}
		actor.IP = currentIP
	}
	req, _ := http.NewRequestWithContext(r.Context(), "POST", "http://internal/session", nil)
	req.Header.Set("token", actor.IMToken)
	req.Header.Set("chatToken", actor.ChatToken)
	req.Header.Set("X-Real-IP", actor.IP)
	if err = s.authenticateMediaActor(r.Context(), actor, req); err != nil {
		writeError(w, err)
		return
	}
	if room.Kind == "call" && room.Invitation.SessionType == 3 {
		members, e := s.IM.GroupMembers(r.Context(), room.Invitation.GroupID)
		if e != nil {
			writeError(w, e)
			return
		}
		allowed := false
		for _, u := range members {
			if u.UserID == grant.UserID {
				allowed = true
			}
		}
		if !allowed {
			writeError(w, forbidden("group membership ended"))
			return
		}
	}
	// Only signaling endpoints may be proxied. In particular no Twirp room-admin API.
	p := strings.TrimPrefix(r.URL.Path, "/livekit")
	if p != "/rtc" && p != "/rtc/validate" {
		writeError(w, missing("unsupported media endpoint"))
		return
	}
	// A previously issued join JWT may carry permissions from before a host
	// changed policy. Refresh the upstream grant from authoritative room state.
	meta, err := s.participantMetadata(r.Context(), grant.UserID, room)
	if err != nil {
		writeError(w, err)
		return
	}
	upToken, err := s.Media.Token(room.MediaRoom, grant.UserID, room.sources(grant.UserID), lease, meta)
	if err != nil {
		writeError(w, err)
		return
	}
	query := r.URL.Query()
	query.Set("access_token", upToken)
	r.URL.RawQuery = query.Encode()
	r.Header.Set("Authorization", "Bearer "+upToken)
	r.URL.Path = p
	r.URL.RawPath = ""
	if identity.Required() && isSignalUpgrade(r) {
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		r = r.WithContext(ctx)
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					checkCtx, done := context.WithTimeout(ctx, 3*time.Second)
					authErr := s.authenticateMediaActor(checkCtx, actor, req)
					current, _, stateErr := load[Room](checkCtx, s.Store, "room:"+grant.RoomID)
					done()
					if authErr == nil && stateErr == nil && !current.ended() && current.CurrentLease[grant.UserID] == lease {
						continue
					}
					// A delayed monitor may only remove its own current lease.
					// Closing this captured socket is safe even if a newer lease won.
					cancel()
					if authErr != nil && stateErr == nil && current.CurrentLease[grant.UserID] == lease {
						removeCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
						_ = s.Media.Remove(removeCtx, grant.MediaRoom, grant.UserID)
						stop()
					}
					cancel()
					return
				}
			}
		}()
	}
	s.Media.Proxy(w, r)
}
func (s *Server) participantMetadata(ctx context.Context, user string, r Room) (string, error) {
	u, err := s.user(ctx, user)
	if err != nil {
		return "", err
	}
	metadata := map[string]any{"userInfo": u}
	if r.Invitation.GroupID != "" {
		metadata["groupInfo"] = Group{GroupID: r.Invitation.GroupID}
		if resolver, ok := s.IM.(GroupResolver); ok {
			if groups, e := resolver.Groups(ctx, []string{r.Invitation.GroupID}); e == nil && len(groups) == 1 {
				metadata["groupInfo"] = groups[0]
			}
		}
		if members, e := s.IM.GroupMembers(ctx, r.Invitation.GroupID); e == nil {
			for _, member := range members {
				if member.UserID == user {
					metadata["groupMemberInfo"] = member
				}
			}
		}
	}
	return string(mustJSON(metadata)), nil
}
