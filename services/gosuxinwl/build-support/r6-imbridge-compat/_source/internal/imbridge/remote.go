package imbridge

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type HTTPRemote struct {
	base, key string
	client    *http.Client
}

func NewHTTPRemote(base, key string) (*HTTPRemote, error) {
	if base == "" {
		base = "http://127.0.0.1:10010"
	}
	u, e := url.Parse(base)
	if e != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Scheme != "http" || !net.ParseIP(u.Hostname()).IsLoopback() {
		return nil, errors.New("IM bridge must use a loopback HTTP endpoint")
	}
	return &HTTPRemote{strings.TrimRight(base, "/"), key, &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (h *HTTPRemote) call(ctx context.Context, path string, in, out any) error {
	body, e := json.Marshal(in)
	if e != nil {
		return e
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce, e := token()
	if e != nil {
		return e
	}
	sum := sha256.Sum256(body)
	canonical := ts + "\n" + nonce + "\nPOST\n" + path + "\n" + hex.EncodeToString(sum[:])
	mac := hmac.New(sha256.New, []byte(h.key))
	mac.Write([]byte(canonical))
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, h.base+path, bytes.NewReader(body))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HRM-Timestamp", ts)
	req.Header.Set("X-HRM-Nonce", nonce)
	req.Header.Set("X-HRM-Signature", hex.EncodeToString(mac.Sum(nil)))
	res, e := h.client.Do(req)
	if e != nil {
		return errors.New("OpenIM bridge unavailable")
	}
	defer res.Body.Close()
	b, e := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if e != nil {
		return errors.New("OpenIM bridge response unavailable")
	}
	var reply struct {
		ErrCode int             `json:"errCode"`
		Data    json.RawMessage `json:"data"`
	}
	if res.StatusCode != 200 || json.Unmarshal(b, &reply) != nil || reply.ErrCode != 0 {
		return fmt.Errorf("OpenIM bridge rejected request (%d)", res.StatusCode)
	}
	if out != nil && len(reply.Data) > 0 {
		return json.Unmarshal(reply.Data, out)
	}
	return nil
}
func (h *HTTPRemote) Exchange(c context.Context, t string, s Session) (v Tokens, e error) {
	e = h.call(c, "/internal/hrm/exchange", map[string]any{"ticket": t, "userID": s.UserID, "sessionID": s.ID, "platform": s.Platform, "expiresAt": s.ExpiresAt, "epoch": s.Epoch}, &v)
	if e == nil && (v.IMToken == "" || v.ChatToken == "" || v.AttachmentToken == "") {
		e = errors.New("OpenIM bridge returned empty tokens")
	}
	return
}
func (h *HTTPRemote) Revoke(c context.Context, v Revoke) error {
	return h.call(c, "/internal/hrm/revoke", v, nil)
}
func (h *HTTPRemote) User(c context.Context, v Projection) error {
	return h.call(c, "/internal/hrm/users/reconcile", map[string]any{"userID": v.UserID, "account": v.Account, "nickname": v.Nickname, "faceURL": v.FaceURL, "departmentID": v.DepartmentID, "departmentName": v.DepartmentName, "enabled": v.Enabled, "version": v.Version}, nil)
}
func (h *HTTPRemote) Department(c context.Context, v Department) error {
	return h.call(c, "/internal/hrm/departments/reconcile", v, nil)
}
func (h *HTTPRemote) Manage(c context.Context, operation string, body any) (out json.RawMessage, e error) {
	switch operation {
	case "groups/search", "groups/members", "groups/mute", "groups/notice":
	default:
		return nil, errors.New("unsupported management operation")
	}
	e = h.call(c, "/internal/hrm/manage/"+operation, body, &out)
	return
}
