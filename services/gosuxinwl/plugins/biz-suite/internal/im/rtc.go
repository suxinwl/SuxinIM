package im

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

func rtcSettings() (string, string, string, bool) {
	endpoint, key, secret := os.Getenv("LIVEKIT_URL"), os.Getenv("LIVEKIT_API_KEY"), os.Getenv("LIVEKIT_API_SECRET")
	u, err := url.Parse(endpoint)
	return endpoint, key, secret, err == nil && u.Host != "" && (u.Scheme == "wss" || u.Scheme == "ws") && key != "" && len(secret) >= 32
}
func rtcConfig(r *ghttp.Request) {
	_, _, _, enabled := rtcSettings()
	reply(r, g.Map{"enabled": enabled, "configured": enabled, "provider": "livekit"}, nil)
}
func rtcToken(r *ghttp.Request) {
	endpoint, key, secret, enabled := rtcSettings()
	if !enabled {
		r.Response.WriteStatus(503)
		r.Response.WriteJson(gf.Failed().SetMsg("RTC unavailable: configure LIVEKIT_URL, LIVEKIT_API_KEY and LIVEKIT_API_SECRET"))
		return
	}
	var in struct {
		ConversationType string `json:"conversation_type"`
		ConversationID   string `json:"conversation_id"`
	}
	if !parse(r, &in) {
		return
	}
	user := core.UID(r.Context())
	if err := communicationAccount(r.Context(), user, true); err != nil {
		reply(r, nil, err)
		return
	}
	users, err := participants(r.Context(), user, in.ConversationType, in.ConversationID)
	if err != nil {
		reply(r, nil, err)
		return
	}
	stream, _ := streamID(in.ConversationType, in.ConversationID, user)
	hash := sha256.Sum256([]byte(stream))
	room := "biz-" + hex.EncodeToString(hash[:16])
	now := time.Now()
	claims := jwt.MapClaims{"iss": key, "sub": strconv.FormatInt(user, 10), "iat": now.Unix(), "nbf": now.Add(-5 * time.Second).Unix(), "exp": now.Add(5 * time.Minute).Unix(), "video": g.Map{"roomJoin": true, "room": room, "canPublish": true, "canSubscribe": true, "canPublishData": true}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		reply(r, nil, err)
		return
	}
	callID := guid.S()
	_, err = model(r.Context(), "biz_im_rtc_call").Data(g.Map{"id": callID, "creator_id": user, "stream_id": stream, "room_name": room, "status": "token_issued", "created_at": core.Now()}).Insert()
	if err != nil {
		reply(r, nil, err)
		return
	}
	liveHub.publish(liveRecipients(r.Context(), users), g.Map{"type": "rtc", "data": g.Map{"call_id": callID, "stream_id": stream, "creator_id": user, "room": room}})
	reply(r, g.Map{"url": endpoint, "token": token, "room": room, "call_id": callID, "expires_in": 300}, nil)
}
