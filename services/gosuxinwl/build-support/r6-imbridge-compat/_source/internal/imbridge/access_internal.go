package imbridge

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
)

type AccessCheckRequest struct {
	UserID       string `json:"userID"`
	Platform     int    `json:"platform"`
	ClientIP     string `json:"clientIP"`
	Action       string `json:"action"`
	ResourcePath string `json:"resourcePath,omitempty"`
}

type accessHMACVerifier struct {
	mu     sync.Mutex
	nonces map[string]int64
}

func isAccessLoopback(r *http.Request) bool {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	return err == nil && ip == "127.0.0.1"
}

func (v *accessHMACVerifier) verify(r *http.Request, body []byte, key string, now time.Time) bool {
	if !isAccessLoopback(r) || len(key) < 32 || r.Method != http.MethodPost || r.URL.Path != "/internal/im-access/check" || r.URL.RawQuery != "" {
		return false
	}
	ts, nonce, signature := r.Header.Get("X-HRM-Timestamp"), r.Header.Get("X-HRM-Nonce"), r.Header.Get("X-HRM-Signature")
	second, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || second < now.Unix()-60 || second > now.Unix()+60 || len(nonce) != 64 || len(signature) != 64 {
		return false
	}
	if _, err = hex.DecodeString(nonce); err != nil {
		return false
	}
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	sum := sha256.Sum256(body)
	canonical := ts + "\n" + nonce + "\nPOST\n/internal/im-access/check\n" + hex.EncodeToString(sum[:])
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(canonical))
	if !hmac.Equal(mac.Sum(nil), provided) {
		return false
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.nonces == nil {
		v.nonces = map[string]int64{}
	}
	for n, expiry := range v.nonces {
		if expiry < now.Unix() {
			delete(v.nonces, n)
		}
	}
	if _, exists := v.nonces[nonce]; exists || len(v.nonces) >= 50000 {
		return false
	}
	v.nonces[nonce] = second + 61
	return true
}

func accessReply(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if status == 200 {
		_ = json.NewEncoder(w).Encode(map[string]any{"errCode": 0, "data": data})
		return
	}
	message := "access denied"
	if status == 503 {
		message = "access authority unavailable"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"errCode": status, "errMsg": message})
}

func (s *Service) internalAccessHandler(key string, verifier *accessHMACVerifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isAccessLoopback(r) {
			accessReply(w, 403, nil)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, (64<<10)+1))
		if err != nil || len(body) > 64<<10 || !verifier.verify(r, body, key, s.now()) {
			accessReply(w, 401, nil)
			return
		}
		var in AccessCheckRequest
		d := json.NewDecoder(strings.NewReader(string(body)))
		d.DisallowUnknownFields()
		if d.Decode(&in) != nil || d.Decode(new(any)) != io.EOF || len(in.UserID) > 64 {
			accessReply(w, 403, nil)
			return
		}
		ip, err := netip.ParseAddr(in.ClientIP)
		if err != nil || ip.Zone() != "" {
			accessReply(w, 403, nil)
			return
		}
		var uid int64
		if err = s.Store.DB.QueryRowContext(r.Context(), `SELECT uid FROM biz_openim_employee WHERE openim_user_id=? AND uid>0`, in.UserID).Scan(&uid); err != nil {
			accessReply(w, 403, nil)
			return
		}
		action := in.Action
		if action == "avatar" {
			action = "connect"
		}
		decision, err := s.Store.CheckAccess(r.Context(), uid, in.Platform, ip.Unmap().String(), action)
		if err == nil && in.Action == "avatar" {
			known, avatarErr := s.Store.IsDirectoryAvatarResource(r.Context(), in.ResourcePath)
			if avatarErr != nil {
				err = avatarErr
			} else if !known {
				err = ErrDenied
			}
		}
		if err != nil {
			status := 503
			if errors.Is(err, ErrDenied) {
				status = 403
			}
			accessReply(w, status, nil)
			return
		}
		accessReply(w, 200, decision)
	}
}

func (s *Service) lanCheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || !isAccessLoopback(r) || len(r.Header.Values("X-GoSuxin-Client-IP")) != 1 {
		w.WriteHeader(403)
		return
	}
	n, err := s.Store.NetworkPolicy(requestAccessContext(r.Context(), r))
	if err != nil {
		w.WriteHeader(503)
		return
	}
	if n.CurrentZone != "lan" {
		w.WriteHeader(403)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func bindInternalAccess(server *ghttp.Server, s *Service) {
	check := s.internalAccessHandler(ConfigFromEnv().HMACKey, &accessHMACVerifier{})
	server.BindHandler("POST:/internal/im-access/check", func(r *ghttp.Request) { check(r.Response.BufferWriter, r.Request) })
	server.BindHandler("GET:/internal/im-access/lan-check", func(r *ghttp.Request) { s.lanCheckHandler(r.Response.BufferWriter, r.Request) })
}

func setAttachmentCookie(w http.ResponseWriter, login LoginResult, clear bool) {
	if !clear && login.AttachmentToken == "" {
		return
	}
	cookie := &http.Cookie{Name: "GoSuxinAttachment", Value: login.AttachmentToken, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteNoneMode, Expires: time.Unix(login.ExpiresAt, 0)}
	remaining := int(time.Until(cookie.Expires).Seconds())
	if clear || remaining <= 0 {
		cookie.Value = ""
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(1, 0)
	} else {
		cookie.MaxAge = remaining
	}
	http.SetCookie(w, cookie)
}
