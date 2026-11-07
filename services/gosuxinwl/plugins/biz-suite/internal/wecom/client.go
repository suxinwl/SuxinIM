package wecom

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const apiBase = "https://qyapi.weixin.qq.com/cgi-bin"

type cachedToken struct {
	Value string
	Until time.Time
}

// Client routes directory access and application access through separate
// credentials. Tokens are cached by credential and never returned to the UI.
type Client struct {
	HTTP                                            *http.Client
	CorpID, AgentID, BaseURL                        string
	ContactSecret, AppSecret                        string
	Secret                                          string   // Deprecated: application credential alias only.
	LegacyCredentialVariables                       []string // Names only; obsolete credential values are not retained.
	mu                                              sync.Mutex
	tokens                                          map[string]cachedToken
	limiter                                         *rate.Limiter
	endpointLimiters                                map[string]*rate.Limiter
	runMu                                           sync.Mutex
	running                                         bool
	settings                                        Settings
	envCorpID, envAgentID                           string
	envContactSecret, envAppSecret, envLegacySecret string
	savedContactSecret, savedAppSecret              bool
}

type apiError struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}
type APIError struct {
	Code     int
	Endpoint string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("wecom %s returned error code %d", e.Endpoint, e.Code)
}

func NewFromEnv() *Client {
	c := &Client{HTTP: &http.Client{Timeout: 20 * time.Second}, CorpID: os.Getenv("WECOM_CORP_ID"), AgentID: os.Getenv("WECOM_AGENT_ID"), Secret: os.Getenv("WECOM_SECRET"), ContactSecret: os.Getenv("WECOM_CONTACT_SECRET"), AppSecret: os.Getenv("WECOM_APP_SECRET"), BaseURL: apiBase}
	for _, key := range []string{"WECOM_CHECKIN_SECRET", "WECOM_APPROVAL_SECRET"} {
		if os.Getenv(key) != "" {
			c.LegacyCredentialVariables = append(c.LegacyCredentialVariables, key)
		}
	}
	return c
}
func (c *Client) secret(scope string) string {
	switch scope {
	case "contact":
		return c.ContactSecret
	case "", "app", "checkin", "approval", "leave", "out", "trip", "overtime", "unmapped":
		if c.AppSecret != "" {
			return c.AppSecret
		}
		return c.Secret
	default:
		return ""
	}
}
func (c *Client) Configured() bool {
	if c == nil {
		return false
	}
	c.runMu.Lock()
	defer c.runMu.Unlock()
	return c.configured()
}
func (c *Client) configured() bool {
	return c != nil && c.CorpID != "" && c.secret("contact") != "" && c.secret("app") != ""
}
func (c *Client) AccessToken(ctx context.Context) (string, error) { return c.accessToken(ctx, "") }
func (c *Client) accessToken(ctx context.Context, scope string) (string, error) {
	if c == nil {
		return "", errors.New("enterprise WeChat client is not configured")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.CorpID == "" || c.secret(scope) == "" {
		return "", errors.New("enterprise WeChat credentials are not configured for this sync type")
	}
	secret := c.secret(scope)
	if t := c.tokens[secret]; t.Value != "" && time.Now().Before(t.Until) {
		return t.Value, nil
	}
	var out struct {
		apiError
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	// Token refresh shares the limiter but avoids acquiring c.mu a second time.
	if c.limiter == nil {
		c.limiter = rate.NewLimiter(rate.Limit(8), 1)
	}
	if err := c.request(ctx, http.MethodGet, "/gettoken", map[string]string{"corpid": c.CorpID, "corpsecret": secret}, nil, &out, c.limiter); err != nil {
		return "", err
	}
	if out.AccessToken == "" || out.ExpiresIn <= 0 {
		return "", errors.New("wecom returned an invalid token response")
	}
	if c.tokens == nil {
		c.tokens = map[string]cachedToken{}
	}
	lifetime := time.Duration(out.ExpiresIn) * time.Second
	margin := lifetime / 10
	if margin > 120*time.Second {
		margin = 120 * time.Second
	}
	c.tokens[secret] = cachedToken{out.AccessToken, time.Now().Add(lifetime - margin)}
	return out.AccessToken, nil
}
func (c *Client) api(ctx context.Context, scope, method, path string, query map[string]string, body, out any) error {
	for refresh := 0; refresh < 2; refresh++ {
		token, err := c.accessToken(ctx, scope)
		if err != nil {
			return err
		}
		q := map[string]string{"access_token": token}
		for k, v := range query {
			q[k] = v
		}
		c.mu.Lock()
		limiter := c.limiter
		// The diagnostic reads all rules, whose dedicated quota is 60/minute.
		// Unlike raw punches (600/minute), it must not use the 8/second bucket.
		if path == "/checkin/getcorpcheckinoption" || path == "/checkin/getcheckinschedulist" {
			if c.endpointLimiters == nil {
				c.endpointLimiters = map[string]*rate.Limiter{}
			}
			if c.endpointLimiters[path] == nil {
				c.endpointLimiters[path] = rate.NewLimiter(rate.Every(time.Second), 1)
			}
			limiter = c.endpointLimiters[path]
		}
		c.mu.Unlock()
		err = c.request(ctx, method, path, q, body, out, limiter)
		var ae *APIError
		if refresh == 0 && errors.As(err, &ae) && (ae.Code == 40014 || ae.Code == 42001) {
			c.mu.Lock()
			if c.tokens[c.secret(scope)].Value == token {
				delete(c.tokens, c.secret(scope))
			}
			c.mu.Unlock()
			continue
		}
		return err
	}
	return errors.New("wecom token refresh failed")
}
func waitContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *Client) request(ctx context.Context, method, path string, query map[string]string, body, out any, limiter *rate.Limiter) error {
	if os.Getenv("GOSUXIN_REHEARSAL") == "true" {
		return errors.New("演练模式禁止调用企业微信上游接口")
	}
	base := c.BaseURL
	if base == "" {
		base = apiBase
	}
	endpoint, err := url.Parse(strings.TrimRight(base, "/") + path)
	if err != nil {
		return errors.New("invalid wecom API base URL")
	}
	q := endpoint.Query()
	for k, v := range query {
		q.Set(k, v)
	}
	endpoint.RawQuery = q.Encode()
	payload, err := json.Marshal(body)
	if err != nil {
		return errors.New("cannot encode wecom API request")
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	// Never allow an HTTP redirect to carry credentials to another origin.
	local := *client
	local.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if attempt > 0 {
			if err := waitContext(ctx, time.Duration(1<<uint(attempt-1))*250*time.Millisecond); err != nil {
				return err
			}
		}
		if limiter != nil {
			if err := limiter.Wait(ctx); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return errors.New("wecom request deadline is too short")
			}
		}
		var reader io.Reader
		if method != http.MethodGet {
			reader = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
		if err != nil {
			return errors.New("cannot construct wecom API request")
		}
		if reader != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := local.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			last = errors.New("wecom transport request failed")
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
		_ = resp.Body.Close()
		if readErr != nil {
			last = errors.New("wecom response read failed")
			continue
		}
		if len(raw) > 8<<20 {
			return errors.New("wecom response exceeds size limit")
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			last = fmt.Errorf("wecom %s returned HTTP %d", path, resp.StatusCode)
			if resp.StatusCode == 429 || resp.StatusCode >= 500 {
				continue
			}
			return last
		}
		var envelope struct {
			ErrCode *int `json:"errcode"`
		}
		if json.Unmarshal(raw, &envelope) != nil || envelope.ErrCode == nil {
			return errors.New("invalid wecom API response")
		}
		if *envelope.ErrCode != 0 {
			last = &APIError{*envelope.ErrCode, path}
			if *envelope.ErrCode == -1 || *envelope.ErrCode == 45009 || *envelope.ErrCode == 45011 {
				continue
			}
			return last
		}
		if err = json.Unmarshal(raw, out); err != nil {
			return errors.New("invalid wecom API response data")
		}
		return nil
	}
	return last
}
