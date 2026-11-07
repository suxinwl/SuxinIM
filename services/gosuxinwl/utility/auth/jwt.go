package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type CustomClaims struct {
	Data interface{} `json:"data" dc:"Custom data"`
	jwt.RegisteredClaims
}

// createToken 生成一个token
func createToken(claims CustomClaims) (string, error) {
	// 生成jwt格式的header、claims 部分
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(SecretKey.String()))
}

// 解析Token (只验证格式及验证过期)
func JwtParseToken(tokenString string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(SecretKey.String()), nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if token == nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, err
}

// 解析Token (只验证格式并不验证过期)
func JwtOnlyParseToken(tokenString string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(SecretKey.String()), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithoutClaimsValidation())
	if token == nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, err
}

// 更新token有效期
func JWTRefreshToken(tokenString string, extraAddSeconds int64) (string, error) {
	if customClaims, err := JwtParseToken(tokenString); err == nil {
		customClaims.ExpiresAt = jwt.NewNumericDate(time.Unix(extraAddSeconds, 0))
		return createToken(*customClaims)
	} else {
		return "", err
	}
}
