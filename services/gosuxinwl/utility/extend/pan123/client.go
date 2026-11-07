package pan123

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
)

func allowedPanURL(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" && u.Port() != "443" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, suffix := range []string{"123pan.com", "123pan.cn", "123clouddisk.com", "123242.com", "123684.com", "123865.com", "123957.com"} {
		if h == suffix || strings.HasSuffix(h, "."+suffix) {
			return true
		}
	}
	return false
}

// CDN URLs come exclusively from the authenticated official direct-link API.
// Custom CDN domains are supported without relaxing API/upload destination checks.
func validPanCDNURL(raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if net.ParseIP(host) != nil || !strings.Contains(host, ".") || strings.HasSuffix(host, ".") {
		return false
	}
	for _, suffix := range []string{".localhost", ".local", ".internal", ".localdomain"} {
		if strings.HasSuffix(host, suffix) {
			return false
		}
	}
	return host != "" // DNS resolution is pinned and private addresses rejected by panHTTP's transport.
}
func panCDNClient(base *http.Client) *http.Client {
	client := *base
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) == 0 || len(via) > 4 || !validPanCDNURL(req.URL.String()) {
			return errors.New("不允许的 CDN 重定向")
		}
		if !strings.EqualFold(req.URL.Hostname(), via[0].URL.Hostname()) && !allowedPanURL(req.URL.String()) {
			return errors.New("CDN 重定向到未授权域名")
		}
		return nil
	}
	return &client
}

var panHTTP = &http.Client{Timeout: 120 * time.Second, Transport: &http.Transport{Proxy: nil, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, e := net.SplitHostPort(address)
	if e != nil {
		return nil, e
	}
	ips, e := net.DefaultResolver.LookupIPAddr(ctx, host)
	if e != nil {
		return nil, e
	}
	for _, ip := range ips {
		if !ip.IP.IsGlobalUnicast() || ip.IP.IsPrivate() || ip.IP.IsLoopback() || ip.IP.IsLinkLocalUnicast() {
			return nil, errors.New("云盘地址解析到非公共网络")
		}
	}
	var last error = errors.New("云盘域名无可用地址")
	for _, ip := range ips {
		conn, err := (&net.Dialer{Timeout: 20 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}}, CheckRedirect: func(req *http.Request, via []*http.Request) error {
	if len(via) > 4 || !allowedPanURL(req.URL.String()) {
		return errors.New("不允许的云盘重定向")
	}
	return nil
}}

type panClient struct {
	p           Config
	http        *http.Client
	mu          sync.Mutex
	token       string
	expires     time.Time
	rateMu      sync.Mutex
	nextRequest time.Time
}

var panClients sync.Map
var panTokenLocks sync.Map

func clientFor(p Config) *panClient {
	configBytes, _ := json.Marshal(p)
	key := fmt.Sprintf("%x", sha256.Sum256(configBytes))
	c, _ := panClients.LoadOrStore(key, &panClient{p: p, http: panHTTP})
	return c.(*panClient)
}
func pause(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
func (c *panClient) accessToken(ctx context.Context, invalid string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && c.token != invalid && time.Until(c.expires) > time.Minute {
		return c.token, nil
	}
	digest := sha256.Sum256([]byte(c.p.ClientID + ":" + c.p.ClientSecret))
	cachePath := "runtime/pan123/token-" + hex.EncodeToString(digest[:12]) + ".json"
	lock, _ := panTokenLocks.LoadOrStore(cachePath, &sync.Mutex{})
	accountLock := lock.(*sync.Mutex)
	accountLock.Lock()
	defer accountLock.Unlock()
	var cached struct {
		Token   string
		Expires time.Time
	}
	{
		if b, e := os.ReadFile(cachePath); e == nil && json.Unmarshal(b, &cached) == nil && cached.Token != invalid && time.Until(cached.Expires) > time.Minute {
			c.token = cached.Token
			c.expires = cached.Expires
			return c.token, nil
		}
	}
	data, e := c.request(ctx, http.MethodPost, "https://open-api.123pan.com/api/v1/access_token", g.Map{"clientID": c.p.ClientID, "clientSecret": c.p.ClientSecret}, false)
	if e != nil {
		return "", e
	}
	c.token = gconv.String(data["accessToken"])
	c.expires = gconv.Time(data["expiredAt"])
	if c.token == "" || !c.expires.After(time.Now()) {
		return "", errors.New("云盘返回无效访问凭证")
	}
	b, _ := json.Marshal(struct {
		Token   string
		Expires time.Time
	}{c.token, c.expires})
	if e = os.MkdirAll("runtime/pan123", 0700); e != nil {
		return "", e
	}
	if e = writePrivateFile(cachePath, b); e != nil {
		return "", e
	}
	return c.token, nil
}

type panAPIError struct {
	code        int
	description string
}

func (e *panAPIError) Error() string { return e.description }

func (c *panClient) request(ctx context.Context, method, address string, data any, auth bool) (g.Map, error) {
	payload, e := json.Marshal(data)
	if e != nil {
		return nil, e
	}
	body := func() io.ReadCloser {
		if method == http.MethodGet {
			return http.NoBody
		}
		return io.NopCloser(bytes.NewReader(payload))
	}
	return c.send(ctx, method, address, "application/json", body, auth, false)
}
func (c *panClient) send(ctx context.Context, method, address, contentType string, body func() io.ReadCloser, auth, emptyOK bool) (g.Map, error) {
	if !allowedPanURL(address) {
		return nil, errors.New("云盘返回了不支持的地址")
	}
	token := ""
	var err error
	if auth {
		token, err = c.accessToken(ctx, "")
		if err != nil {
			return nil, err
		}
	}
	for attempt := 0; attempt < 4; attempt++ {
		c.rateMu.Lock()
		delay := time.Until(c.nextRequest)
		if delay < 0 {
			delay = 0
		}
		c.nextRequest = time.Now().Add(delay + 100*time.Millisecond)
		c.rateMu.Unlock()
		if e := pause(ctx, delay); e != nil {
			return nil, e
		}
		req, e := http.NewRequestWithContext(ctx, method, address, body())
		if e != nil {
			return nil, e
		}
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Platform", "open_platform")
		if auth {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, e := c.http.Do(req)
		if e != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if attempt < 3 && (method == http.MethodGet || strings.HasSuffix(address, "/slice") || strings.HasSuffix(address, "/upload_complete")) {
				if err = pause(ctx, time.Duration(1<<attempt)*time.Second); err != nil {
					return nil, err
				}
				continue
			}
			return nil, errors.New("连接 123 云盘失败，有限重试已耗尽")
		}
		b, e := io.ReadAll(io.LimitReader(res.Body, 2<<20))
		res.Body.Close()
		if e != nil {
			return nil, errors.New("读取云盘响应失败")
		}
		var out struct {
			Code    *int   `json:"code"`
			Data    g.Map  `json:"data"`
			Trace   string `json:"x-traceID"`
			Message string `json:"message"`
		}
		parsed := json.Unmarshal(b, &out) == nil && out.Code != nil
		code := -1
		if parsed {
			code = *out.Code
		}
		if res.StatusCode == 200 && emptyOK && len(bytes.TrimSpace(b)) == 0 {
			return g.Map{}, nil
		}
		if auth && (res.StatusCode == 401 || parsed && code == 401) && attempt < 3 {
			token, err = c.accessToken(ctx, token)
			if err != nil {
				return nil, err
			}
			continue
		}
		safeRetry := method == http.MethodGet || strings.HasSuffix(address, "/slice") || strings.HasSuffix(address, "/upload_complete")
		if (res.StatusCode == 429 || res.StatusCode >= 500 && safeRetry || parsed && code == 429) && attempt < 3 {
			if err = pause(ctx, time.Duration(1<<attempt)*time.Second); err != nil {
				return nil, err
			}
			continue
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 || !parsed || code != 0 {
			message := out.Message
			for _, secret := range []string{token, c.p.ClientSecret, c.p.CDNKey} {
				if secret != "" {
					message = strings.ReplaceAll(message, secret, "[已脱敏]")
				}
			}
			endpoint, _ := url.Parse(address)
			return nil, &panAPIError{code: code, description: fmt.Sprintf("123 云盘请求失败（接口 %s，HTTP %d，代码 %d，原因 %s，追踪 %s）", endpoint.Path, res.StatusCode, code, message, out.Trace)}
		}
		return out.Data, nil
	}
	return nil, errors.New("123 云盘请求重试已耗尽")
}
func fileChecksum(path string) (string, int64, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", 0, e
	}
	defer f.Close()
	h := md5.New()
	n, e := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, e
}
func panNumber(m g.Map, keys ...string) int64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return gconv.Int64(v)
		}
	}
	return 0
}
func (c *panClient) detail(ctx context.Context, id int64) (g.Map, error) {
	return c.request(ctx, "GET", fmt.Sprintf("https://open-api.123pan.com/api/v1/file/detail?fileID=%d", id), nil, true)
}
func (c *panClient) upload(ctx context.Context, path, filename string, parent int64) (resultID int64, resultErr error) {
	var id int64
	defer func() {
		if resultErr != nil && id > 0 {
			cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if e := trash(cleanup, c, id); e != nil {
				resultErr = fmt.Errorf("%w；云端文件 %d 清理失败：%v", resultErr, id, e)
			}
		}
	}()
	etag, size, e := fileChecksum(path)
	if e != nil {
		return 0, e
	}
	d, e := c.request(ctx, "POST", "https://open-api.123pan.com/upload/v2/file/create", g.Map{"parentFileID": parent, "filename": filename, "etag": etag, "size": size, "containDir": true, "duplicate": 1}, true)
	if e != nil {
		return 0, e
	}
	id = panNumber(d, "fileID", "fileId")
	if !gconv.Bool(d["reuse"]) {
		pre := gconv.String(d["preuploadID"])
		chunk := gconv.Int64(d["sliceSize"])
		servers := gconv.Strings(d["servers"])
		if pre == "" || chunk <= 0 || len(servers) == 0 {
			return 0, errors.New("云盘未返回有效上传任务")
		}
		host := strings.TrimRight(servers[0], "/")
		// Older create responses advertise HTTP; always use TLS for the official host.
		if u, err := url.Parse(host); err == nil && u.Scheme == "http" && u.User == nil && u.Port() == "" {
			u.Scheme = "https"
			host = u.String()
		}
		if !allowedPanURL(host) {
			return 0, errors.New("上传域名不在许可范围内")
		}
		f, e := os.Open(path)
		if e != nil {
			return 0, e
		}
		defer f.Close()
		for offset, no := int64(0), 1; offset < size; no++ {
			n := min(chunk, size-offset)
			h := md5.New()
			if _, e = io.Copy(h, io.NewSectionReader(f, offset, n)); e != nil {
				return 0, e
			}
			var prefix bytes.Buffer
			w := multipart.NewWriter(&prefix)
			_ = w.WriteField("preuploadID", pre)
			_ = w.WriteField("sliceNo", strconv.Itoa(no))
			_ = w.WriteField("sliceMD5", hex.EncodeToString(h.Sum(nil)))
			if _, e = w.CreateFormFile("slice", "part"); e != nil {
				return 0, e
			}
			head := append([]byte(nil), prefix.Bytes()...)
			prefix.Reset()
			if e = w.Close(); e != nil {
				return 0, e
			}
			tail := append([]byte(nil), prefix.Bytes()...)
			factory := func() io.ReadCloser {
				return io.NopCloser(io.MultiReader(bytes.NewReader(head), io.NewSectionReader(f, offset, n), bytes.NewReader(tail)))
			}
			if _, e = c.send(ctx, "POST", host+"/upload/v2/file/slice", w.FormDataContentType(), factory, true, true); e != nil {
				return 0, e
			}
			offset += n
		}
		completeCtx, cancelComplete := context.WithTimeout(ctx, 2*time.Minute)
		defer cancelComplete()
		for {
			d, e = c.request(completeCtx, "POST", "https://open-api.123pan.com/upload/v2/file/upload_complete", g.Map{"preuploadID": pre}, true)
			if e != nil {
				var apiErr *panAPIError
				// 20103 means server-side verification is still in progress.
				if errors.As(e, &apiErr) && apiErr.code == 20103 {
					if e = pause(completeCtx, time.Second); e != nil {
						return 0, e
					}
					continue
				}
				return 0, e
			}
			if gconv.Bool(d["completed"]) {
				id = panNumber(d, "fileID", "fileId")
				break
			}
			if e = pause(completeCtx, time.Second); e != nil {
				return 0, e
			}
		}
	}
	if id <= 0 {
		return 0, errors.New("云盘未确认文件 ID")
	}
	info, e := c.detail(ctx, id)
	if e != nil {
		return 0, e
	}
	if panNumber(info, "status") > 100 || panNumber(info, "trashed") != 0 {
		return 0, errors.New("云端文件不可用")
	}
	if panNumber(info, "size") != size || !strings.EqualFold(gconv.String(info["etag"]), etag) {
		return 0, errors.New("云端文件大小或 MD5 校验失败")
	}
	return id, nil
}

type directEntry struct {
	URL   string
	Until time.Time
}

var directLinks sync.Map

func (c *panClient) direct(ctx context.Context, id int64) (string, error) {
	key := fmt.Sprintf("%s:%d:%d", c.p.ClientID, c.p.UID, id)
	if v, ok := directLinks.Load(key); ok {
		entry := v.(directEntry)
		if time.Now().Before(entry.Until) {
			return entry.URL, nil
		}
	}
	d, e := c.request(ctx, "GET", fmt.Sprintf("https://open-api.123pan.com/api/v1/direct-link/url?fileID=%d", id), nil, true)
	if e != nil {
		return "", e
	}
	raw := gconv.String(d["url"])
	if !validPanCDNURL(raw) {
		return "", errors.New("官方接口返回的 CDN 直链无效：需要 HTTPS 公网域名")
	}
	u, _ := url.Parse(raw)
	u.RawQuery = ""
	u.Fragment = ""
	raw = u.String()
	directLinks.Store(key, directEntry{raw, time.Now().Add(10 * time.Minute)})
	return raw, nil
}

// Both modes accept only vendor-resolved, validated HTTPS CDN addresses.
func profileCDNURL(raw string, p Config, expires time.Time) (string, error) {
	if p.URLAuthEnabled() {
		return signPanURL(raw, p.UID, p.CDNKey, expires)
	}
	if !validPanCDNURL(raw) {
		return "", errors.New("云盘直链必须是受信任的 HTTPS 地址")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

// Official 123 URL authentication: URI-timestamp-rand-uid-PrivateKey.
func signPanURL(raw string, uid int64, key string, expires time.Time) (string, error) {
	if !validPanCDNURL(raw) || uid <= 0 || key == "" {
		return "", errors.New("CDN 鉴权配置不完整")
	}
	u, e := url.Parse(raw)
	if e != nil {
		return "", e
	}
	random, e := newKey()
	if e != nil {
		return "", e
	}
	fields := fmt.Sprintf("%d-%s-%d", expires.Unix(), random, uid)
	hash := md5.Sum([]byte(u.EscapedPath() + "-" + fields + "-" + key))
	q := u.Query()
	q.Set("auth_key", fields+"-"+hex.EncodeToString(hash[:]))
	u.RawQuery = q.Encode()
	return u.String(), nil
}
