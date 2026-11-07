package identity

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

func TrustedProxy(remote string) bool {
	host, _, e := net.SplitHostPort(remote)
	if e != nil {
		host = remote
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	for _, raw := range strings.Split(os.Getenv("TRUSTED_ENTRY_PROXY_CIDRS"), ",") {
		_, sub, e := net.ParseCIDR(strings.TrimSpace(raw))
		if e == nil && sub.Contains(ip) {
			return true
		}
	}
	return false
}
func ClientIP(r *http.Request) (string, error) {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		host = r.RemoteAddr
	}
	peer := net.ParseIP(host)
	if peer == nil {
		return "", ErrSession
	}
	if TrustedProxy(r.RemoteAddr) {
		v := r.Header.Values("X-GoSuxin-Client-IP")
		if len(v) == 0 {
			v = r.Header.Values("X-Real-IP")
		}
		if len(v) != 1 {
			return "", ErrSession
		}
		ip := net.ParseIP(v[0])
		if ip == nil {
			return "", ErrSession
		}
		return ip.String(), nil
	}
	return peer.String(), nil
}
func EntryOrigin(r *http.Request) string {
	if !TrustedProxy(r.RemoteAddr) {
		return ""
	}
	key := ""
	fallback := ""
	switch singleHeader(r, "X-Puty-Entry") {
	case "lan":
		key = "PUBLIC_LAN_ORIGIN"
		fallback = "https://192.168.10.11:9885"
	case "internet":
		key = "PUBLIC_INTERNET_ORIGIN"
		fallback = "https://im.szputy.com:9885"
	default:
		return ""
	}
	if v := os.Getenv(key); v != "" {
		return strings.TrimRight(v, "/")
	}
	return fallback
}

var accessHTTP = &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

var ErrAccessDenied = errors.New("HRM access denied")

type EffectivePolicy struct {
	Zone          string `json:"zone"`
	LogicalClient string `json:"logicalClient"`
	Login         bool   `json:"login"`
	Preview       bool   `json:"preview"`
	Save          bool   `json:"save"`
	Sources       struct {
		Login string `json:"login"`
		Save  string `json:"save"`
	} `json:"sources"`
	SecurityVersion int64  `json:"securityVersion"`
	PolicyRevision  int64  `json:"policyRevision"`
	NetworkRevision int64  `json:"networkRevision"`
	CurrentIP       string `json:"currentIP"`
}

type AccessDecision struct {
	Allowed         bool            `json:"allowed"`
	Zone            string          `json:"zone"`
	LogicalClient   string          `json:"logicalClient"`
	LoginAllowed    bool            `json:"loginAllowed"`
	SaveAllowed     bool            `json:"saveAllowed"`
	EffectivePolicy EffectivePolicy `json:"effectivePolicy"`
	SecurityVersion int64           `json:"securityVersion"`
	PolicyRevision  int64           `json:"policyRevision"`
	NetworkRevision int64           `json:"networkRevision"`
}

func CheckAccess(ctx context.Context, user string, platform int, ip, action, resource string) error {
	_, err := CheckLogicalAccess(ctx, user, platform, "", ip, action, resource)
	return err
}

func CheckSessionAccess(ctx context.Context, v Session, ip, action, resource string) error {
	_, err := CheckSessionDecision(ctx, v, ip, action, resource)
	return err
}

func CheckSessionDecision(ctx context.Context, v Session, ip, action, resource string) (AccessDecision, error) {
	if !Required() {
		return AccessDecision{Allowed: true}, nil
	}
	client, err := SessionClient(v)
	if err != nil {
		return AccessDecision{}, ErrSession
	}
	decision, err := CheckLogicalAccess(ctx, v.UserID, v.Platform, client, ip, action, resource)
	if err != nil {
		return decision, err
	}
	if decision.SecurityVersion < 1 || decision.SecurityVersion != v.Epoch {
		return decision, ErrAccessDenied
	}
	return decision, nil
}

func CheckLogicalAccess(ctx context.Context, user string, platform int, client, ip, action, resource string) (AccessDecision, error) {
	var decision AccessDecision
	if !Required() {
		decision.Allowed = true
		return decision, nil
	}
	if net.ParseIP(ip) == nil || !Platform(platform) {
		return decision, ErrSession
	}
	client, e := LogicalClient(platform, 0, client)
	if e != nil {
		return decision, ErrSession
	}
	endpoint := os.Getenv("HRM_ACCESS_CHECK_URL")
	u, e := url.Parse(endpoint)
	if e != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "/internal/im-access/check" && u.Path != "/internal/hrm/access/check") {
		return decision, ErrSession
	}
	key := os.Getenv("HRM_BRIDGE_HMAC_KEY")
	if len(key) < 32 {
		return decision, ErrSession
	}
	body, _ := json.Marshal(map[string]any{"userID": user, "platform": platform, "logicalClient": client, "clientIP": ip, "entryZone": EntryZoneFromContext(ctx), "action": action, "resourcePath": resource})
	random := make([]byte, 32)
	if _, e = rand.Read(random); e != nil {
		return decision, e
	}
	nonce := hex.EncodeToString(random)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	r, e := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if e != nil {
		return decision, ErrSession
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-HRM-Timestamp", timestamp)
	r.Header.Set("X-HRM-Nonce", nonce)
	r.Header.Set("X-HRM-Signature", Sign(key, timestamp, nonce, "POST", u.Path, body))
	response, e := accessHTTP.Do(r)
	if e != nil {
		return decision, ErrSession
	}
	defer response.Body.Close()
	var result struct {
		ErrCode int            `json:"errCode"`
		Data    AccessDecision `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&result) != nil {
		return decision, ErrSession
	}
	if response.StatusCode == http.StatusForbidden && (result.ErrCode == http.StatusForbidden || result.ErrCode == 1002) || response.StatusCode == http.StatusOK && result.ErrCode == 0 && !result.Data.Allowed {
		return result.Data, ErrAccessDenied
	}
	if response.StatusCode != http.StatusOK || result.ErrCode != 0 {
		return decision, ErrSession
	}
	return result.Data, nil
}

type Attachment struct {
	SessionID     string `json:"sessionID"`
	UserID        string `json:"userID"`
	Epoch         int64  `json:"epoch"`
	Platform      int    `json:"platform"`
	SourceHost    string `json:"sourceHost"`
	Path          string `json:"path"`
	Write         bool   `json:"write"`
	ExpiresAt     int64  `json:"expiresAt"`
	Kind          string `json:"kind,omitempty"`
	Purpose       string `json:"purpose,omitempty"`
	LogicalClient string `json:"logicalClient,omitempty"`
}

func (s *Store) Attachment(ctx context.Context, token string) (Attachment, error) {
	var a Attachment
	e := s.Read(ctx, "HRM38:ATTACH:"+Hash(token), &a)
	if e != nil || a.ExpiresAt <= s.now().Unix() {
		return a, ErrSession
	}
	v, e := s.Live(ctx, a.SessionID)
	if e != nil || v.UserID != a.UserID || v.Epoch != a.Epoch || v.Platform != a.Platform {
		return a, ErrSession
	}
	if a.LogicalClient != "" && a.LogicalClient != v.LogicalClient {
		return a, ErrSession
	}
	a.LogicalClient = v.LogicalClient
	if a.Purpose == "" {
		a.Purpose = "preview"
		if a.Write {
			a.Purpose = "upload"
		}
	}
	return a, nil
}
func (s *Store) WrapAttachment(ctx context.Context, v Session, origin, raw string, write bool) (string, error) {
	purpose := "preview"
	if write {
		purpose = "upload"
	}
	return s.WrapAttachmentPurpose(ctx, v, origin, raw, write, purpose)
}
func (s *Store) WrapAttachmentPurpose(ctx context.Context, v Session, origin, raw string, write bool, purpose string) (string, error) {
	client, e := SessionClient(v)
	if e != nil || write && purpose != "upload" || !write && purpose != "preview" && purpose != "save" {
		return "", ErrSession
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || (!SafePath(u.Path, "/openim38/") && !(write && u.Path == "/openim38")) {
		return "", ErrSession
	}
	lan := os.Getenv("PUBLIC_LAN_ORIGIN")
	if lan == "" {
		lan = "https://192.168.10.11:9885"
	}
	wan := os.Getenv("PUBLIC_INTERNET_ORIGIN")
	if wan == "" {
		wan = "https://im.szputy.com:9885"
	}
	source := "https://" + u.Host
	if source != lan && source != wan || origin != lan && origin != wan {
		return "", ErrSession
	}
	random := make([]byte, 32)
	if _, e = rand.Read(random); e != nil {
		return "", e
	}
	token := hex.EncodeToString(random)
	a := Attachment{SessionID: v.SessionID, UserID: v.UserID, Epoch: v.Epoch, Platform: v.Platform, LogicalClient: client, Purpose: purpose, SourceHost: u.Host, Path: u.EscapedPath(), Write: write, ExpiresAt: v.ExpiresAt}
	if e = s.Write(ctx, "HRM38:ATTACH:"+Hash(token), a, time.Duration(v.ExpiresAt-s.now().Unix())*time.Second); e != nil {
		return "", e
	}
	return origin + "/attachments/" + token + u.EscapedPath() + func() string {
		if u.RawQuery != "" {
			return "?" + u.RawQuery
		}
		return ""
	}(), nil
}
func (s *Store) WrapAvatar(ctx context.Context, v Session, origin, raw string) (string, error) {
	return s.WrapAvatarPurpose(ctx, v, origin, raw, "preview")
}
func (s *Store) WrapAvatarPurpose(ctx context.Context, v Session, origin, raw, purpose string) (string, error) {
	client, e := SessionClient(v)
	if e != nil || purpose != "preview" && purpose != "save" {
		return "", ErrSession
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || !SafePath(u.Path, "/resource/uploads/") {
		return "", ErrSession
	}
	lan := os.Getenv("PUBLIC_HRM_LAN_ORIGIN")
	if lan == "" {
		lan = "https://192.168.10.11:9880"
	}
	wan := os.Getenv("PUBLIC_HRM_INTERNET_ORIGIN")
	if wan == "" {
		wan = "https://hr.szputy.com:9880"
	}
	if "https://"+u.Host != lan && "https://"+u.Host != wan {
		return "", ErrSession
	}
	random := make([]byte, 32)
	if _, e = rand.Read(random); e != nil {
		return "", e
	}
	token := hex.EncodeToString(random)
	a := Attachment{SessionID: v.SessionID, UserID: v.UserID, Epoch: v.Epoch, Platform: v.Platform, LogicalClient: client, Purpose: purpose, SourceHost: u.Host, Path: u.EscapedPath(), ExpiresAt: v.ExpiresAt, Kind: "avatar"}
	if e = s.Write(ctx, "HRM38:ATTACH:"+Hash(token), a, time.Duration(v.ExpiresAt-s.now().Unix())*time.Second); e != nil {
		return "", e
	}
	return origin + "/attachments/" + token + u.EscapedPath(), nil
}
func SafePath(path, prefix string) bool {
	if !strings.HasPrefix(path, prefix) || strings.ContainsAny(path, "\\\x00%") {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	return true
}
