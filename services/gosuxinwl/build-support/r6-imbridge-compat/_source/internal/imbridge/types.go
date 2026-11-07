// Package imbridge is the HRM authority for OpenIM identities and sessions.
// It is opt-in: without GOSUXIN_IM_ENABLED=true it registers no routes, creates
// no tables and never changes existing employee accounts.
package imbridge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const SessionLifetime = 4 * time.Hour
const RefreshWindow = 10 * time.Minute
const TicketLifetime = 30 * time.Second

var ErrDenied = errors.New("账号、密码或通讯权限无效")
var ErrExpired = errors.New("通讯会话已失效，请重新登录")

type Config struct {
	Enabled                                 bool
	BridgeURL, HMACKey, RedisURL, PublicURL string
}

func ConfigFromEnv() Config {
	return Config{Enabled: os.Getenv("GOSUXIN_IM_ENABLED") == "true", BridgeURL: os.Getenv("GOSUXIN_IM_BRIDGE_URL"), HMACKey: os.Getenv("GOSUXIN_IM_HMAC_KEY"), RedisURL: os.Getenv("GOSUXIN_IM_REDIS_URL"), PublicURL: os.Getenv("GOSUXIN_IM_PUBLIC_URL")}
}
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if len(c.HMACKey) < 32 {
		return errors.New("GOSUXIN_IM_HMAC_KEY requires at least 32 characters")
	}
	if c.RedisURL == "" {
		return errors.New("GOSUXIN_IM_REDIS_URL is required")
	}
	return nil
}

type Profile struct {
	EmployeeID     string `json:"employeeID"`
	Account        string `json:"account"`
	Name           string `json:"name"`
	FaceURL        string `json:"faceURL"`
	DepartmentID   string `json:"departmentID"`
	DepartmentName string `json:"departmentName"`
	Position       string `json:"position"`
}
type Projection struct {
	UserID          string  `json:"userID"`
	Account         string  `json:"account"`
	Nickname        string  `json:"nickname"`
	FaceURL         string  `json:"faceURL"`
	DepartmentID    string  `json:"departmentID"`
	DepartmentName  string  `json:"departmentName"`
	Enabled         bool    `json:"enabled"`
	Version         int64   `json:"version"`
	EmployeeID      string  `json:"employeeID"`
	HostUID         int64   `json:"hostUID"`
	Reason          string  `json:"reason"`
	AuthFingerprint string  `json:"authFingerprint,omitempty"`
	Profile         Profile `json:"profile"`
}
type Department struct {
	DepartmentID  string   `json:"departmentID"`
	Name          string   `json:"name"`
	GroupID       string   `json:"groupID"`
	MemberUserIDs []string `json:"memberUserIDs"`
	Frozen        bool     `json:"frozen"`
	Version       int64    `json:"version"`
}
type Session struct {
	ID        string  `json:"id"`
	UserID    string  `json:"userID"`
	UID       int64   `json:"uid"`
	Platform  int     `json:"platform"`
	Epoch     int64   `json:"epoch"`
	ExpiresAt int64   `json:"expiresAt"`
	Profile   Profile `json:"profile"`
}
type Ticket struct {
	Session  Session `json:"session"`
	Audience string  `json:"audience"`
}
type LoginResult struct {
	UserID          string  `json:"userID"`
	IMToken         string  `json:"imToken"`
	ChatToken       string  `json:"chatToken"`
	AttachmentToken string  `json:"attachmentToken"`
	SessionToken    string  `json:"sessionToken"`
	ExpiresAt       int64   `json:"expiresAt"`
	Profile         Profile `json:"profile"`
}
type Tokens struct {
	IMToken         string `json:"imToken"`
	ChatToken       string `json:"chatToken"`
	AttachmentToken string `json:"attachmentToken"`
}
type Revoke struct {
	UserID    string `json:"userID"`
	SessionID string `json:"sessionID,omitempty"`
	Platform  int    `json:"platform,omitempty"`
	All       bool   `json:"all"`
	Epoch     int64  `json:"epoch"`
}
type Remote interface {
	Exchange(context.Context, string, Session) (Tokens, error)
	Revoke(context.Context, Revoke) error
	User(context.Context, Projection) error
	Department(context.Context, Department) error
	Manage(context.Context, string, any) (json.RawMessage, error)
}
type Cache interface {
	Put(context.Context, string, any, time.Duration) error
	Get(context.Context, string, any) error
	Take(context.Context, string, any) error
	Delete(context.Context, string) error
	// ReplacePlatform atomically replaces the platform pointer and persists the
	// new session. It returns the previous opaque session hash, never its token.
	ReplacePlatform(context.Context, Session, string, time.Duration) (string, error)
	CheckPlatform(context.Context, Session, string) (bool, error)
	RefreshPlatform(context.Context, Session, string, time.Duration) (bool, error)
}

func UserID(employeeID string) string {
	h := sha256.Sum256([]byte(employeeID))
	return "emp_" + hex.EncodeToString(h[:16])
}
func GroupID(dept string) string {
	h := sha256.Sum256([]byte("department:" + dept))
	return "dept_" + hex.EncodeToString(h[:16])
}
func token() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func digest(value string) string { v := sha256.Sum256([]byte(value)); return hex.EncodeToString(v[:]) }
func Platform(value string) (int, error) {
	switch value {
	case "android":
		return 2, nil
	case "windows":
		return 3, nil
	case "web":
		return 5, nil
	}
	return 0, errors.New("不支持的客户端平台")
}
func platformKey(s Session) string { return "platform:" + s.UserID + ":" + strconv.Itoa(s.Platform) }
func envelope(w http.ResponseWriter, status int, data any, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	code, msg := 0, ""
	if err != nil {
		code = status
		msg = err.Error()
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"code": code, "msg": msg, "data": data})
}
func decode(r *http.Request, v any) error {
	if r.Method != http.MethodPost {
		return errors.New("仅支持POST")
	}
	r.Body = http.MaxBytesReader(nil, r.Body, 64<<10)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return errors.New("请求格式无效")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("请求格式无效")
	}
	return nil
}
func fingerprint(p Projection) string {
	p.Version = 0
	b, _ := json.Marshal(p)
	return digest(string(b))
}
func errorText(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, ErrDenied) || errors.Is(err, ErrExpired) || errors.Is(err, ErrConflict) || errors.Is(err, ErrInvalid) {
		return err.Error()
	}
	return "通讯服务暂不可用，请稍后重试"
}
func intString(v int64) string { return strconv.FormatInt(v, 10) }
func safePrefix(p string) (string, error) {
	for _, c := range p {
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return "", fmt.Errorf("invalid database prefix")
		}
	}
	return p, nil
}
func normalize(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
func containsDirectory(p Projection, k string) bool {
	return strings.Contains(normalize(p.Account+" "+p.Nickname+" "+p.DepartmentName+" "+p.Profile.Position), normalize(k))
}
func faceURL(origin, value string) string {
	if value == "" {
		value = "/resource/static/brand/logo.png?v=suxin1"
	}
	base, e := url.Parse(origin)
	if e != nil {
		return ""
	}
	u, e := url.Parse(value)
	if e != nil {
		return ""
	}
	u = base.ResolveReference(u)
	if u.Scheme != "https" && u.Scheme != "http" {
		return ""
	}
	return u.String()
}
