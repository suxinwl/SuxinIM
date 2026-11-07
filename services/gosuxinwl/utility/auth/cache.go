package auth

import (
	"github.com/suxinwl/GoSuxin/utility/tools/tcache"
	"time"

	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
)

func contains(key string) bool {
	return tcache.Contains(key)
}

// 设置缓存
func setCache(key string, value interface{}) error {
	return tcache.SetCache(key, value, time.Duration(TokenTimeout.Int64()+MaxRefresh.Int64())*time.Second)
}

// 获取缓存值
func getCache(key string) (tData *TokenData, err error) {
	var result *gvar.Var
	result, err = tcache.GetCache(key)
	if err != nil {
		return
	}
	if result.Val() != nil {
		err = gconv.Struct(result, &tData)
	}
	return
}

// 删除
func removeCache(key string) (err error) {
	err = tcache.RemoveCache(key)
	return
}

// 关闭缓存对象，让GC回收资源
func CloseCache() (err error) {
	err = tcache.CloseCache()
	return
}
