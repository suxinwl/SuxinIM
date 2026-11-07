package auth

import (
	"github.com/suxinwl/GoSuxin/framework/os/gcfg"
	"github.com/suxinwl/GoSuxin/framework/os/gctx"
)

// 常量
const (
	JwtTokenOK      int = 200 //token有效
	JwtTokenInvalid int = 401 //无效的token
	JwtTokenExpired int = 403 //过期的token
)

// 从配置中获取配置数据
var (
	ctx              = gctx.New()
	TokenTimeout, _  = gcfg.Instance("app").Get(ctx, "app.TokenTimeout")  // 基础有效秒数；与 MaxRefresh 相加为完整会话有效期（13800 + 600 = 4 小时）
	MaxRefresh, _    = gcfg.Instance("app").Get(ctx, "app.MaxRefresh")    // 刷新token时间 默认10分钟（秒）：60 * 10(MaxRefresh值为0时,token将不会自动刷新)
	SecretKey, _     = gcfg.Instance("app").Get(ctx, "app.SecretKey")     //JWT-Token加密key 32位
	MultiLogin, _    = gcfg.Instance("app").Get(ctx, "app.MultiLogin")    // 是否允许多点登录
	TokenCacheKey, _ = gcfg.Instance("app").Get(ctx, "app.TokenCacheKey") // 缓存key前缀（防止与其他业务冲突）
	EncryptKey, _    = gcfg.Instance("app").Get(ctx, "app.EncryptKey")    // token返回值的AES算法加密字符 32位
	DynamicToken, _  = gcfg.Instance("app").Get(ctx, "app.DynamicToken")  // 开启动态token模块
)
