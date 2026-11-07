package gf

import (
	"github.com/suxinwl/GoSuxin/utility/tools/tcache"
	"time"
)

// 缓存-保存数据
func SetCache(key, code interface{}, duration time.Duration) (err error) {
	err = tcache.SetCache(key, code, duration)
	return
}

// 把验证码保存在本地，用GetVerifyCode获取key对应缓存
func SetVerifyCode(key, code string) (err error) {
	err = SetCache(key, code, time.Second*60)
	return
}

// 获取本地保存的验证码，使用SetVerifyCode保存可以对应数据
func GetVerifyCode(key string) (code string, err error) {
	val, err := tcache.GetCache(key)
	if err == nil {
		code = val.String()
	}
	return
}
