// ======================================================
// MaxRefresh：处理携带token的请求时当前时间大于超时时间并小于缓存刷新时间时token将自动刷新即重置token存活时间
// ======================================================
package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/suxinwl/GoSuxin/framework/crypto/gaes"
	"github.com/suxinwl/GoSuxin/framework/crypto/gmd5"
	"github.com/suxinwl/GoSuxin/framework/encoding/gbase64"
	"github.com/suxinwl/GoSuxin/framework/errors/gerror"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/grand"
)

// TokenData Token 数据
type TokenData struct {
	JwtToken string `json:"jwtToken"`
	UuId     string `json:"uuId"`
}

// token存活时间 (存活时间 = 超时时间 + 缓存刷新时间)
func makeExpiresTime() time.Time {
	return time.Now().Add(time.Second * time.Duration(TokenTimeout.Int64()+MaxRefresh.Int64()))
}

// GenerateToken 应用层传入加密内容返回token
func GenerateToken(ctx context.Context, key string, data interface{}) (keys string, err error) {
	if len(key) <= 0 {
		err = gerror.New("The key value cannot be empty")
		return
	}
	var (
		uuid   string
		tData  *TokenData
		tokens string
	)
	// 支持多端重复登录，返回新token
	if MultiLogin.Bool() {
		tData, err = getCache(TokenCacheKey.String() + key)
		if err != nil {
			return
		}
		if tData != nil {
			// Reuse a live session only when the newly authenticated security
			// claims are unchanged. A changed password/epoch must rotate UUID.
			claims, parseErr := JwtParseToken(tData.JwtToken)
			if parseErr == nil {
				previous, previousErr := json.Marshal(claims.Data)
				current, currentErr := json.Marshal(data)
				if previousErr == nil && currentErr == nil && bytes.Equal(previous, current) && doRefresh(key, tData) {
					keys, _, err = EncryptToken(ctx, key, tData.UuId)
					return
				}
			}
		}
	}
	tokens, err = createToken(CustomClaims{
		data,
		jwt.RegisteredClaims{
			NotBefore: jwt.NewNumericDate(time.Unix(time.Now().Unix()-10, 0)), // 生效开始时间
			ExpiresAt: jwt.NewNumericDate(makeExpiresTime()),                  // 失效截止时间
		},
	})
	if err != nil {
		return
	}
	keys, uuid, err = EncryptToken(ctx, key)
	if err != nil {
		return
	}
	err = setCache(TokenCacheKey.String()+key, TokenData{
		JwtToken: tokens,
		UuId:     uuid,
	})
	return
}

// RemoveToken 删除token
func RemoveToken(ctx context.Context, key string) (err error) {
	err = removeCache(TokenCacheKey.String() + key)
	return
}

// 获取jwt-token中的数据
func GetTokenData(ctx context.Context, token string) (tData *TokenData, key string, err error) {
	var uuid string
	key, uuid, err = DecryptToken(ctx, token)
	if err != nil {
		return
	}
	tData, err = getCache(TokenCacheKey.String() + key)
	// fmt.Println("过期tData", tData, err)
	if tData == nil || tData.UuId != uuid {
		err = errors.New("token is invalid")
	}
	return
}

// 解析token (验证格式并验证过期)
func ParseToken(r *ghttp.Request) (*CustomClaims, error) {
	token, key, err := GetJwtToken(r)
	if err != nil {
		return nil, err
	}
	claims, err := JwtParseToken(token.JwtToken)
	if err != nil {
		return nil, err
	}
	policy := requestPolicy.Load()
	if policy != nil && policy.check != nil {
		if err = policy.check(r, claims); err != nil {
			return nil, err
		}
	}
	if !IsEffective(key, token) {
		return nil, errors.New("token is invalid")
	}
	// doRefresh replaces JwtToken on success. Return the current expiry so
	// plugin cookies can follow the same session instead of expiring earlier.
	claims, err = JwtParseToken(token.JwtToken)
	if err == nil && policy != nil && policy.accepted != nil {
		policy.accepted(r, claims)
	}
	return claims, err
}

// ParseTokenReadOnly validates the current opaque UUID/cache and signed token.
// It never refreshes the session, runs response callbacks, or issues cookies.
// Callers must apply the resource's own authorization after this authentication.
func ParseTokenReadOnly(r *ghttp.Request) (*CustomClaims, error) {
	token, _, err := GetJwtToken(r)
	if err != nil {
		return nil, err
	}
	return JwtParseToken(token.JwtToken)
}

// 检查缓存的token是否有效且自动刷新缓存token(请求时返回最新token)
func IsEffective(key string, cacheToken *TokenData) bool {
	_, code := IsNotExpired(cacheToken.JwtToken)
	if JwtTokenOK == code {
		// 刷新缓存
		if IsRefresh(cacheToken.JwtToken) {
			return doRefresh(key, cacheToken)
		}
		return true
	}
	return false
}

// token是否处于刷新期
func IsRefresh(token string) bool {
	if MaxRefresh.Int64() == 0 {
		return false
	}
	if customClaims, err := JwtParseToken(token); err == nil {
		now := time.Now().Unix()
		if now < customClaims.ExpiresAt.Unix() && now > (customClaims.ExpiresAt.Unix()-MaxRefresh.Int64()) {
			return true
		}
	}
	return false
}

// 刷新token并存到缓存中
func doRefresh(key string, cacheToken *TokenData) bool {
	if cacheToken == nil {
		return false
	}
	// A prior request's snapshot must not restore a removed session or replace
	// a newer login. This is a defensive recheck, not a distributed cache CAS.
	current, err := getCache(TokenCacheKey.String() + key)
	if err != nil || current == nil || current.UuId != cacheToken.UuId {
		return false
	}
	newToken, err := TokenRefreshToken(current.JwtToken)
	if err != nil {
		return false
	}
	current.JwtToken = newToken
	if err = setCache(TokenCacheKey.String()+key, current); err != nil {
		return false
	}
	cacheToken.JwtToken = newToken
	return true
}

// 刷新token的缓存有效期
func TokenRefreshToken(oldToken string) (newToken string, err error) {
	if newToken, err = JWTRefreshToken(oldToken, makeExpiresTime().Unix()); err != nil {
		return
	}
	return
}

// 检查token是否过期 (过期时间 = 超时时间 + 缓存刷新时间)
func IsNotExpired(token string) (*CustomClaims, int) {
	if customClaims, err := JwtParseToken(token); err == nil {
		if time.Now().Unix()-customClaims.ExpiresAt.Unix() < 0 {
			// token有效
			return customClaims, JwtTokenOK
		} else {
			// 过期的token
			return customClaims, JwtTokenExpired
		}
	} else {
		// 无效的token
		return customClaims, JwtTokenInvalid
	}
}

// EncryptToken token加密方法
func EncryptToken(ctx context.Context, key string, randStr ...string) (encryptStr, uuid string, err error) {
	if key == "" {
		err = errors.New("encrypt key empty")
		return
	}
	// 生成随机串
	if len(randStr) > 0 {
		uuid = randStr[0]
	} else {
		uuid = gmd5.MustEncrypt(grand.Letters(12))
	}
	strdata := fmt.Sprintf("%v%v", key, uuid)
	token, err := gaes.Encrypt([]byte(strdata), []byte(EncryptKey.String()))
	if err != nil {
		err = errors.New("encrypt error")
		return
	}
	encryptStr = gbase64.EncodeToString(token)
	return
}

// DecryptToken token解密方法
func DecryptToken(ctx context.Context, token string) (DecryptStr, uuid string, err error) {
	if token == "" {
		err = errors.New("decrypt Token empty")
		return
	}
	token64, err := gbase64.Decode([]byte(token))
	if err != nil {
		err = errors.New("decode error")
		return
	}
	decryptToken, err := gaes.Decrypt(token64, []byte(EncryptKey.String()))
	if err != nil {
		err = errors.New("decrypt error")
		return
	}
	length := len(decryptToken)
	if length <= 32 {
		err = errors.New("decrypt token invalid")
		return
	}
	uuid = string(decryptToken[length-32:])
	DecryptStr = string(decryptToken[:length-32])
	return
}
