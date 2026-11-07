package tcache

import (
	"time"

	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/database/gredis"
	"github.com/suxinwl/GoSuxin/framework/errors/gerror"
	"github.com/suxinwl/GoSuxin/framework/os/gcache"
	"github.com/suxinwl/GoSuxin/framework/os/gcfg"
	"github.com/suxinwl/GoSuxin/framework/os/gctx"
)

var (
	ctx   = gctx.New()
	cache = gcache.New()
)

// 如果存在redis 缓存使用redis
func init() {
	// 创建redis缓存适配器并将其设置为缓存对象。
	confAddress, isExi := gcfg.Instance().Get(ctx, "redis.cache.address")
	if isExi == nil && confAddress != nil && !confAddress.IsEmpty() {
		confdb, _ := gcfg.Instance().Get(ctx, "redis.cache.db")
		redis, err := gredis.New(&gredis.Config{
			Address: confAddress.String(),
			Db:      confdb.Int(),
		})
		if err != nil {
			return
		}
		_, sizerr := redis.DBSize(ctx)
		if sizerr == nil {
			cache.SetAdapter(gcache.NewAdapterRedis(redis))
		}
	}

}

// 缓存中是否存在指定键名
func Contains(key string) bool {
	ok, _ := cache.Contains(ctx, key)
	return ok
}

// 设置缓存
func SetCache(key, value interface{}, duration time.Duration) error {
	err := cache.Set(ctx, key, value, duration)
	if err != nil {
		return gerror.New("redis链接失败,重启后端服务器或者检查Redis并启动redis服务。")
	}
	return err
}

// 获取缓存值
func GetCache(key string) (*gvar.Var, error) {
	result, err := cache.Get(ctx, key)
	if err != nil {
		return nil, gerror.New("redis链接失败,重启后端服务器或者检查Redis并启动redis服务。")
	}
	return result, err
}

// 删除
func RemoveCache(key string) (err error) {
	_, err = cache.Remove(ctx, key)
	return
}

// 关闭缓存对象，让GC回收资源
func CloseCache() (err error) {
	err = cache.Close(ctx)
	return
}
