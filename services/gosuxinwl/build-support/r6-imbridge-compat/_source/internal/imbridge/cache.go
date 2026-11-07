package imbridge

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/redis/go-redis/v9"
	"time"
)

const cachePrefix = "gosuxin:im:"

type RedisCache struct{ r *redis.Client }

func NewRedisCache(rawURL string) (*RedisCache, error) {
	o, e := redis.ParseURL(rawURL)
	if e != nil {
		return nil, errors.New("invalid IM Redis configuration")
	}
	o.DialTimeout = 3 * time.Second
	o.ReadTimeout = 3 * time.Second
	o.WriteTimeout = 3 * time.Second
	return &RedisCache{redis.NewClient(o)}, nil
}
func (c *RedisCache) Close() error                   { return c.r.Close() }
func (c *RedisCache) Ping(ctx context.Context) error { return c.r.Ping(ctx).Err() }
func (c *RedisCache) Put(ctx context.Context, k string, v any, ttl time.Duration) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	return c.r.Set(ctx, cachePrefix+k, b, ttl).Err()
}
func (c *RedisCache) Get(ctx context.Context, k string, v any) error {
	b, e := c.r.Get(ctx, cachePrefix+k).Bytes()
	if e != nil {
		return ErrExpired
	}
	return json.Unmarshal(b, v)
}
func (c *RedisCache) Take(ctx context.Context, k string, v any) error {
	b, e := c.r.GetDel(ctx, cachePrefix+k).Bytes()
	if e != nil {
		return ErrExpired
	}
	return json.Unmarshal(b, v)
}
func (c *RedisCache) Delete(ctx context.Context, k string) error {
	return c.r.Del(ctx, cachePrefix+k).Err()
}
func (c *RedisCache) ReplacePlatform(ctx context.Context, s Session, hash string, ttl time.Duration) (string, error) {
	b, e := json.Marshal(s)
	if e != nil {
		return "", e
	}
	return c.r.Eval(ctx, `local old=redis.call('GET',KEYS[1]); redis.call('SET',KEYS[1],ARGV[1],'PX',ARGV[3]); redis.call('SET',KEYS[2],ARGV[2],'PX',ARGV[3]); return old or ''`, []string{cachePrefix + platformKey(s), cachePrefix + "sid:" + hash}, hash, string(b), ttl.Milliseconds()).Text()
}
func (c *RedisCache) CheckPlatform(ctx context.Context, s Session, hash string) (bool, error) {
	v, e := c.r.Get(ctx, cachePrefix+platformKey(s)).Result()
	if e == redis.Nil {
		return false, nil
	}
	return v == hash, e
}
func (c *RedisCache) RefreshPlatform(ctx context.Context, s Session, hash string, ttl time.Duration) (bool, error) {
	b, e := json.Marshal(s)
	if e != nil {
		return false, e
	}
	v, e := c.r.Eval(ctx, `if redis.call('GET',KEYS[1]) ~= ARGV[1] then return 0 end; redis.call('SET',KEYS[1],ARGV[1],'PX',ARGV[3]); redis.call('SET',KEYS[2],ARGV[2],'PX',ARGV[3]); return 1`, []string{cachePrefix + platformKey(s), cachePrefix + "sid:" + hash}, hash, string(b), ttl.Milliseconds()).Int()
	return v == 1, e
}
