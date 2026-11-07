// Package ai supplies tenant-scoped AI configuration and revocable developer tokens.
package ai

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Endpoint     string `json:"endpoint"`
	Model        string `json:"model"`
	APIKey       string `json:"apiKey,omitempty"`
	AllowPrivate bool   `json:"allowPrivate"`
	SealedKey    string `json:"sealedKey,omitempty"`
}
type Token struct {
	ID        string    `json:"id"`
	UID       int64     `json:"uid"`
	Name      string    `json:"name"`
	Scopes    []string  `json:"scopes"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	Revoked   bool      `json:"revoked"`
}
type Service struct {
	root     string
	key      []byte
	mu       sync.Mutex
	requests chan struct{}
}

var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var validScopes = map[string]bool{"codegen:read": true, "excel:read": true, "ai:generate": true}

func NewService(root string) (*Service, error) {
	if root == "" {
		return nil, errors.New("AI 数据目录为空")
	}
	for _, d := range []string{root, filepath.Join(root, "config"), filepath.Join(root, "tokens")} {
		if err := os.MkdirAll(d, 0700); err != nil {
			return nil, err
		}
	}
	s := &Service{root: root, requests: make(chan struct{}, 2)}
	p := filepath.Join(root, "master.key")
	k, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		k = make([]byte, 32)
		if _, err = rand.Read(k); err != nil {
			return nil, err
		}
		f, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			k, err = os.ReadFile(p)
		} else {
			_, err = f.Write(k)
			ce := f.Close()
			if err == nil {
				err = ce
			}
		}
	}
	if err != nil {
		return nil, err
	}
	if len(k) != 32 {
		return nil, errors.New("AI 配置加密密钥无效")
	}
	s.key = k
	return s, nil
}
func (s *Service) configPath(uid int64) (string, error) {
	if uid < 1 {
		return "", errors.New("管理员身份无效")
	}
	return filepath.Join(s.root, "config", fmt.Sprintf("%d.json", uid)), nil
}
func (s *Service) readConfig(uid int64) (Config, error) {
	p, e := s.configPath(uid)
	if e != nil {
		return Config{}, e
	}
	b, e := os.ReadFile(p)
	if os.IsNotExist(e) {
		return Config{}, nil
	}
	if e != nil {
		return Config{}, e
	}
	var c Config
	e = json.Unmarshal(b, &c)
	return c, e
}
func (s *Service) PublicConfig(uid int64) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, e := s.readConfig(uid)
	return map[string]any{"endpoint": c.Endpoint, "model": c.Model, "allowPrivate": c.AllowPrivate, "keyConfigured": c.SealedKey != ""}, e
}
func validateEndpoint(value string, allowPrivate bool) (*url.URL, error) {
	u, e := url.Parse(strings.TrimSpace(value))
	if e != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("请输入完整的 AI API 基地址，不包含密码、查询参数或片段")
	}
	if u.Scheme != "https" && !(allowPrivate && u.Scheme == "http") {
		return nil, errors.New("AI 接口必须使用 HTTPS；本机服务需显式允许内网接口")
	}
	if !allowPrivate && (u.Hostname() == "localhost" || isPrivate(net.ParseIP(u.Hostname()))) {
		return nil, errors.New("内网接口需要显式启用允许内网选项")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u, nil
}
func isPrivate(ip net.IP) bool {
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast())
}
func (s *Service) SaveConfig(uid int64, c Config, clearKey bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, e := s.configPath(uid)
	if e != nil {
		return e
	}
	u, e := validateEndpoint(c.Endpoint, c.AllowPrivate)
	if e != nil {
		return e
	}
	c.Endpoint = u.String()
	c.Model = strings.TrimSpace(c.Model)
	if c.Model == "" || len(c.Model) > 120 {
		return errors.New("模型名称不能为空且不能超过 120 字符")
	}
	old, e := s.readConfig(uid)
	if e != nil {
		return e
	}
	c.SealedKey = old.SealedKey
	if clearKey {
		c.SealedKey = ""
	}
	if c.APIKey != "" {
		if len(c.APIKey) > 8192 {
			return errors.New("API Key 太长")
		}
		c.SealedKey, e = s.seal(c.APIKey)
		if e != nil {
			return e
		}
	}
	c.APIKey = ""
	return atomicJSON(p, c)
}
func (s *Service) seal(value string) (string, error) {
	block, e := aes.NewCipher(s.key)
	if e != nil {
		return "", e
	}
	g, e := cipher.NewGCM(block)
	if e != nil {
		return "", e
	}
	n := make([]byte, g.NonceSize())
	if _, e = rand.Read(n); e != nil {
		return "", e
	}
	return base64.RawStdEncoding.EncodeToString(g.Seal(n, n, []byte(value), nil)), nil
}
func (s *Service) unseal(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	b, e := base64.RawStdEncoding.DecodeString(value)
	if e != nil {
		return "", e
	}
	block, _ := aes.NewCipher(s.key)
	g, _ := cipher.NewGCM(block)
	if len(b) < g.NonceSize() {
		return "", errors.New("API Key 数据损坏")
	}
	plain, e := g.Open(nil, b[:g.NonceSize()], b[g.NonceSize():], nil)
	return string(plain), e
}
func atomicJSON(p string, value any) error {
	b, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(p), ".save-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(0600); e != nil {
		f.Close()
		return e
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), p)
}
func (s *Service) CreateToken(uid int64, name string, scopes []string, days int) (Token, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if uid < 1 {
		return Token{}, "", errors.New("管理员身份无效")
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return Token{}, "", errors.New("令牌名称不能为空且不能超过 100 字符")
	}
	if days < 1 || days > 30 {
		return Token{}, "", errors.New("令牌有效期必须为 1 至 30 天")
	}
	if len(scopes) == 0 {
		scopes = []string{"codegen:read", "excel:read"}
	}
	seen := map[string]bool{}
	for _, v := range scopes {
		if !validScopes[v] || seen[v] {
			return Token{}, "", errors.New("令牌权限无效或重复")
		}
		seen[v] = true
	}
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return Token{}, "", e
	}
	raw := "sxdev_" + base64.RawURLEncoding.EncodeToString(b)
	hash := sha256.Sum256([]byte(raw))
	t := Token{ID: hex.EncodeToString(hash[:]), UID: uid, Name: name, Scopes: scopes, CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour)}
	return t, raw, atomicJSON(filepath.Join(s.root, "tokens", t.ID+".json"), t)
}
func (s *Service) ValidateToken(raw string) (Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !strings.HasPrefix(raw, "sxdev_") || len(raw) > 100 {
		return Token{}, errors.New("开发令牌无效")
	}
	hash := sha256.Sum256([]byte(raw))
	id := hex.EncodeToString(hash[:])
	b, e := os.ReadFile(filepath.Join(s.root, "tokens", id+".json"))
	if e != nil {
		return Token{}, errors.New("开发令牌无效")
	}
	var t Token
	if json.Unmarshal(b, &t) != nil || t.ID != id || t.UID < 1 || t.Revoked || !time.Now().Before(t.ExpiresAt) {
		return Token{}, errors.New("开发令牌失效或已撤销")
	}
	return t, nil
}
func (s *Service) Tokens(uid int64) ([]Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if uid < 1 {
		return nil, errors.New("管理员身份无效")
	}
	files, e := os.ReadDir(filepath.Join(s.root, "tokens"))
	if e != nil {
		return nil, e
	}
	result := []Token{}
	for _, f := range files {
		if f.IsDir() || !hashPattern.MatchString(strings.TrimSuffix(f.Name(), ".json")) {
			continue
		}
		b, e := os.ReadFile(filepath.Join(s.root, "tokens", f.Name()))
		if e != nil {
			return nil, e
		}
		var t Token
		if e = json.Unmarshal(b, &t); e != nil {
			return nil, e
		}
		if t.UID == uid {
			result = append(result, t)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}
func (s *Service) Revoke(uid int64, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !hashPattern.MatchString(id) {
		return errors.New("令牌 ID 无效")
	}
	p := filepath.Join(s.root, "tokens", id+".json")
	b, e := os.ReadFile(p)
	if e != nil {
		return errors.New("令牌不存在")
	}
	var t Token
	if e = json.Unmarshal(b, &t); e != nil {
		return e
	}
	if t.UID != uid {
		return errors.New("令牌不存在")
	}
	t.Revoked = true
	return atomicJSON(p, t)
}
func (t Token) Allows(scope string) bool {
	for _, s := range t.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

var Instructions = map[string]string{
	"schema":  "你是 GoSuxin 开发助手。把需求转为字段设计，只返回 JSON：{module,table,title,fields:[{name,label,type,required,length}]}。标识符使用小写下划线，type 仅 string/text/int/float/bool/datetime。不要包含业务记录、凭据或 DROP SQL。",
	"review":  "你是 GoSuxin 代码审查助手。用中文分析输入的设计或代码，重点检查权限、类型、路径、数据兼容和测试。不要执行代码，不要求提供密码。",
	"explain": "你是 GoSuxin 开发助手。用中文解释需求与实现建议，遵循 GoSuxin 分层、Vue 3/Arco 界面和独立运行插件方式。",
}

func (s *Service) Generate(ctx context.Context, uid int64, mode, prompt string) (map[string]any, error) {
	instruction, ok := Instructions[mode]
	if !ok {
		return nil, errors.New("提示词模式无效")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || len(prompt) > 32768 {
		return nil, errors.New("需求或代码不能为空且不能超过 32KB")
	}
	select {
	case s.requests <- struct{}{}:
		defer func() { <-s.requests }()
	default:
		return nil, errors.New("AI 正在处理其他请求，请稍后再试")
	}
	s.mu.Lock()
	c, e := s.readConfig(uid)
	s.mu.Unlock()
	if e != nil {
		return nil, e
	}
	if c.Endpoint == "" || c.Model == "" {
		return nil, errors.New("请先配置 AI API 基地址和模型")
	}
	u, e := validateEndpoint(c.Endpoint, c.AllowPrivate)
	if e != nil {
		return nil, e
	}
	key, e := s.unseal(c.SealedKey)
	if e != nil {
		return nil, errors.New("AI API Key 读取失败")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/chat/completions"
	payload := map[string]any{"model": c.Model, "stream": false, "messages": []map[string]string{{"role": "system", "content": instruction}, {"role": "user", "content": prompt}}}
	b, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(b))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	transport := &http.Transport{Proxy: nil, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 60 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
		if e != nil {
			return nil, e
		}
		if len(ips) == 0 {
			return nil, errors.New("AI 主机没有地址")
		}
		for _, v := range ips {
			if !c.AllowPrivate && isPrivate(v.IP) {
				return nil, errors.New("AI 接口解析为内网地址，需显式允许")
			}
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("AI 接口不允许重定向") }}
	resp, e := client.Do(req)
	if e != nil {
		return nil, errors.New("AI 接口连接失败或请求超时，请检查接口配置")
	}
	defer resp.Body.Close()
	body, e := io.ReadAll(io.LimitReader(resp.Body, 2<<20+1))
	if e != nil || len(body) > 2<<20 {
		return nil, errors.New("AI 返回内容过大或读取失败")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("AI 接口返回 HTTP %d", resp.StatusCode)
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Model string          `json:"model"`
		Usage json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(body, &result) != nil || len(result.Choices) == 0 || result.Choices[0].Message.Content == "" {
		return nil, errors.New("AI 返回格式无效，需要兼容 Chat Completions 的接口")
	}
	return map[string]any{"text": result.Choices[0].Message.Content, "model": result.Model, "usage": result.Usage}, nil
}
