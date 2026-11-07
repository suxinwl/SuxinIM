// Package identity is the shared, fail-closed HRM session authority. Native IM
// and Chat signatures remain necessary, but never replace this live registry.
package identity

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrSession = errors.New("HRM session is expired, revoked or unavailable")

func Required() bool        { return strings.EqualFold(os.Getenv("HRM_SSO_REQUIRED"), "true") }
func Hash(s string) string  { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func Managed(s string) bool { return strings.HasPrefix(s, "emp_") || s == "hrm-im-system" }
func Platform(p int) bool   { return p == 1 || p == 2 || p == 3 || p == 5 }
func Sign(key, timestamp, nonce, method, path string, body []byte) string {
	h := sha256.Sum256(body)
	m := hmac.New(sha256.New, []byte(key))
	fmt.Fprintf(m, "%s\n%s\n%s\n%s\n%s", timestamp, nonce, method, path, hex.EncodeToString(h[:]))
	return hex.EncodeToString(m.Sum(nil))
}

type Pair struct {
	IMHash    string `json:"imHash"`
	ChatHash  string `json:"chatHash"`
	ExpiresAt int64  `json:"expiresAt"`
}
type Session struct {
	UserID         string `json:"userID"`
	SessionID      string `json:"sessionID"`
	Platform       int    `json:"platform"`
	Epoch          int64  `json:"epoch"`
	ExpiresAt      int64  `json:"expiresAt"`
	Revoked        bool   `json:"revoked"`
	Pairs          []Pair `json:"pairs"`
	PolicyPlatform int    `json:"policyPlatform,omitempty"`
	LogicalClient  string `json:"logicalClient,omitempty"`
}
type Projection struct {
	UserID         string `json:"userID"`
	Account        string `json:"account"`
	Nickname       string `json:"nickname"`
	FaceURL        string `json:"faceURL"`
	DepartmentID   string `json:"departmentID"`
	DepartmentName string `json:"departmentName"`
	Enabled        bool   `json:"enabled"`
	Version        int64  `json:"version"`
}
type Binding struct {
	SessionID string `json:"sessionID"`
	Kind      string `json:"kind"`
	ExpiresAt int64  `json:"expiresAt"`
}
type Store struct {
	Redis redis.UniversalClient
	Now   func() time.Time
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
func sessionKey(id string) string          { return "HRM38:SESSION:" + Hash(id) }
func userKey(id string) string             { return "HRM38:USER:" + id }
func bindingKey(token string) string       { return "HRM38:BIND:" + Hash(token) }
func currentKey(user string, p int) string { return "HRM38:CURRENT:" + user + ":" + strconv.Itoa(p) }
func sessionsKey(user string) string       { return "HRM38:SESSIONS:" + user }
func epochKey(user string) string          { return "HRM38:EPOCH:" + user }

var global struct {
	sync.Mutex
	url   string
	store *Store
}

func Global() (*Store, error) {
	global.Lock()
	defer global.Unlock()
	url := os.Getenv("HRM_REDIS_URL")
	if url == "" {
		return nil, ErrSession
	}
	if global.store != nil && global.url == url {
		return global.store, nil
	}
	options, err := redis.ParseURL(url)
	if err != nil {
		return nil, ErrSession
	}
	options.DialTimeout = 2 * time.Second
	options.ReadTimeout = 2 * time.Second
	options.WriteTimeout = 2 * time.Second
	options.MaxRetries = 0
	global.store = &Store{Redis: redis.NewClient(options)}
	global.url = url
	return global.store, nil
}
func (s *Store) Read(ctx context.Context, key string, out any) error {
	b, e := s.Redis.Get(ctx, key).Bytes()
	if e != nil {
		return fmt.Errorf("%w: %w", ErrSession, e)
	}
	if json.Unmarshal(b, out) != nil {
		return ErrSession
	}
	return nil
}
func (s *Store) Write(ctx context.Context, key string, v any, ttl time.Duration) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	return s.Redis.Set(ctx, key, b, ttl).Err()
}
func (s *Store) Lock(ctx context.Context, user string) (func(), error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return nil, e
	}
	v := hex.EncodeToString(b)
	key := "HRM38:LOCK:" + Hash(user)
	for i := 0; i < 150; i++ {
		ok, e := s.Redis.SetNX(ctx, key, v, 30*time.Second).Result()
		if e != nil {
			return nil, ErrSession
		}
		if ok {
			return func() {
				_ = s.Redis.Eval(context.Background(), `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0`, []string{key}, v).Err()
			}, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return nil, errors.New("HRM identity is busy")
}
func (s *Store) Projection(ctx context.Context, user string) (Projection, error) {
	var p Projection
	e := s.Read(ctx, userKey(user), &p)
	return p, e
}
func (s *Store) PutProjection(ctx context.Context, p Projection) error {
	return s.Write(ctx, userKey(p.UserID), p, 0)
}
func (s *Store) Session(ctx context.Context, id string) (Session, error) {
	var v Session
	e := s.Read(ctx, sessionKey(id), &v)
	return v, e
}
func (s *Store) Epoch(ctx context.Context, user string) (int64, error) {
	v, e := s.Redis.Get(ctx, epochKey(user)).Int64()
	if e == redis.Nil {
		return 0, nil
	}
	if e != nil {
		return 0, ErrSession
	}
	return v, nil
}
func (s *Store) Live(ctx context.Context, id string) (Session, error) {
	v, e := s.Session(ctx, id)
	if e != nil {
		return v, ErrSession
	}
	v.LogicalClient, e = SessionClient(v)
	if e != nil {
		return v, ErrSession
	}
	p, e := s.Projection(ctx, v.UserID)
	if e != nil || !p.Enabled || v.Revoked || v.ExpiresAt <= s.now().Unix() {
		return v, ErrSession
	}
	epoch, e := s.Epoch(ctx, v.UserID)
	if e != nil || v.Epoch < epoch {
		return v, ErrSession
	}
	current, e := s.Redis.Get(ctx, currentKey(v.UserID, v.Platform)).Result()
	if e != nil || current != id {
		return v, ErrSession
	}
	return v, nil
}
func (s *Store) Validate(ctx context.Context, token, user, kind string, platform int) (Session, error) {
	var b Binding
	if token == "" || s.Read(ctx, bindingKey(token), &b) != nil || b.Kind != kind || b.ExpiresAt <= s.now().Unix() {
		return Session{}, ErrSession
	}
	v, e := s.Live(ctx, b.SessionID)
	if e != nil || v.UserID != user || (platform != 0 && v.Platform != platform) {
		return Session{}, ErrSession
	}
	return v, nil
}
func Validate(ctx context.Context, token, user, kind string, platform int) (Session, error) {
	s, e := Global()
	if e != nil {
		return Session{}, e
	}
	return s.Validate(ctx, token, user, kind, platform)
}
func (s *Store) Pair(ctx context.Context, chat, im, user string, platform int) (Session, error) {
	a, e := s.Validate(ctx, chat, user, "chat", platform)
	if e != nil {
		return a, e
	}
	b, e := s.Validate(ctx, im, user, "im", platform)
	if e != nil || a.SessionID != b.SessionID || a.Epoch != b.Epoch || a.Platform != b.Platform {
		return Session{}, ErrSession
	}
	return a, nil
}

// Exchange is called while holding the user's lock. An old SID cannot be
// resurrected. Refresh retains previous generations until their own expiry.
func (s *Store) Exchange(ctx context.Context, v Session, chat, im string) ([]string, error) {
	client, err := SessionClient(v)
	if err != nil {
		return nil, ErrSession
	}
	v.LogicalClient = client
	if !Platform(v.Platform) || len(v.SessionID) < 16 || v.ExpiresAt <= s.now().Unix() || v.ExpiresAt > s.now().Add(4*time.Hour).Unix()+5 {
		return nil, ErrSession
	}
	p, e := s.Projection(ctx, v.UserID)
	if e != nil || !p.Enabled {
		return nil, ErrSession
	}
	ep, e := s.Epoch(ctx, v.UserID)
	if e != nil || v.Epoch < ep {
		return nil, ErrSession
	}
	var replaced []string
	old, e := s.Session(ctx, v.SessionID)
	if e != nil && !errors.Is(e, redis.Nil) {
		return nil, e
	}
	if e == nil {
		oldClient, clientErr := SessionClient(old)
		if old.Revoked || old.UserID != v.UserID || old.Platform != v.Platform || old.Epoch != v.Epoch || v.ExpiresAt < old.ExpiresAt || clientErr != nil || oldClient != v.LogicalClient {
			return nil, ErrSession
		}
		for _, pair := range old.Pairs {
			if pair.ExpiresAt > s.now().Unix() {
				v.Pairs = append(v.Pairs, pair)
			}
		}
	}
	current, e := s.Redis.Get(ctx, currentKey(v.UserID, v.Platform)).Result()
	if e != nil && e != redis.Nil {
		return nil, ErrSession
	}
	if current != "" && current != v.SessionID {
		prior, e := s.Session(ctx, current)
		if e != nil && !errors.Is(e, redis.Nil) {
			return nil, e
		}
		if e == nil {
			prior.Revoked = true
			if e = s.saveSession(ctx, prior); e != nil {
				return nil, e
			}
			replaced = append(replaced, current)
		}
	}
	v.Pairs = append(v.Pairs, Pair{IMHash: Hash(im), ChatHash: Hash(chat), ExpiresAt: v.ExpiresAt})
	ttl := time.Until(time.Unix(v.ExpiresAt, 0))
	if s.Now != nil {
		ttl = time.Duration(v.ExpiresAt-s.now().Unix()) * time.Second
	}
	if e = s.saveSession(ctx, v); e != nil {
		return nil, e
	}
	pipe := s.Redis.TxPipeline()
	pipe.Set(ctx, currentKey(v.UserID, v.Platform), v.SessionID, ttl)
	pipe.SAdd(ctx, sessionsKey(v.UserID), v.SessionID)
	pipe.Expire(ctx, sessionsKey(v.UserID), 4*time.Hour)
	cb, _ := json.Marshal(Binding{SessionID: v.SessionID, Kind: "chat", ExpiresAt: v.ExpiresAt})
	ib, _ := json.Marshal(Binding{SessionID: v.SessionID, Kind: "im", ExpiresAt: v.ExpiresAt})
	pipe.Set(ctx, bindingKey(chat), cb, ttl)
	pipe.Set(ctx, bindingKey(im), ib, ttl)
	_, e = pipe.Exec(ctx)
	return replaced, e
}
func (s *Store) saveSession(ctx context.Context, v Session) error {
	ttl := time.Duration(v.ExpiresAt-s.now().Unix()) * time.Second
	if ttl < 4*time.Hour {
		ttl = 4 * time.Hour
	}
	return s.Write(ctx, sessionKey(v.SessionID), v, ttl)
}

// Revoke must run under the user lock. A persisted epoch barrier only targets
// older generations; delayed jobs never disconnect a later successful login.
func (s *Store) Revoke(ctx context.Context, user, sid string, platform int, all bool, epoch int64) ([]string, error) {
	if sid != "" {
		v, err := s.Session(ctx, sid)
		if errors.Is(err, redis.Nil) {
			// Revocation can arrive before an in-flight exchange. Persist a
			// tombstone so that delayed exchange cannot resurrect this SID.
			v = Session{UserID: user, SessionID: sid, ExpiresAt: s.now().Add(4 * time.Hour).Unix()}
		} else if err != nil {
			return nil, err
		}
		if v.UserID != user {
			return nil, ErrSession
		}
		v.Revoked = true
		if err = s.saveSession(ctx, v); err != nil {
			return nil, err
		}
		// HRM includes the platform as metadata on an exact-SID logout.
		// Delayed retries must never expand that target to a replacement SID
		// on the same platform. Platform-wide and epoch-wide operations omit SID.
		return []string{sid}, nil
	}
	if all && epoch > 0 {
		_, e := s.Redis.Eval(ctx, `local old=tonumber(redis.call('GET',KEYS[1]) or '0');local n=tonumber(ARGV[1]);if n>old then redis.call('SET',KEYS[1],n) end;return 1`, []string{epochKey(user)}, epoch).Result()
		if e != nil {
			return nil, e
		}
	}
	ids, e := s.Redis.SMembers(ctx, sessionsKey(user)).Result()
	if e != nil {
		return nil, e
	}
	var revoked []string
	for _, id := range ids {
		v, e := s.Session(ctx, id)
		if errors.Is(e, redis.Nil) {
			if e = s.Redis.SRem(ctx, sessionsKey(user), id).Err(); e != nil {
				return nil, e
			}
			continue
		}
		if e != nil || v.UserID != user {
			return nil, ErrSession
		}
		target := sid != "" && id == sid || platform != 0 && v.Platform == platform || all && (epoch == 0 || v.Epoch < epoch)
		if !target {
			continue
		}
		v.Revoked = true
		if e = s.saveSession(ctx, v); e != nil {
			return nil, e
		}
		revoked = append(revoked, id)
	}
	return revoked, nil
}
