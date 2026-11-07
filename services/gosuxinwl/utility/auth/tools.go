package auth

import (
	"github.com/suxinwl/GoSuxin/utility/tools/cryptojs"
	"strconv"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
)

const (
	BearerPrefix = "Bearer "
)

func decodeDynamicToken(value string) (token string, ok bool) {
	return decodeDynamicTokenAt(value, time.Now())
}

func decodeDynamicTokenAt(value string, now time.Time) (token string, ok bool) {
	defer func() {
		if recover() != nil {
			token, ok = "", false
		}
	}()
	decoded, err := cryptojs.AesDecrypt(value)
	if err != nil {
		return "", false
	}
	parts := strings.Split(decoded, "#")
	if len(parts) != 2 || parts[0] == "" {
		return "", false
	}
	issuedAt, err := strconv.ParseInt(parts[1], 10, 64)
	// Keep the existing 30-second expiry; tolerate only a small forward clock
	// difference on remote browsers. The underlying session is still verified.
	if err != nil || issuedAt <= now.Unix()-30 || issuedAt > now.Unix()+5 {
		return "", false
	}
	return parts[0], true
}

// Configured API groups still require the short-lived encrypted wrapper.
// Plugin bootstrap also accepts it; the plugin cookie contains the base token.
// Credentials are deliberately not read from query parameters.
func GetRequestToken(r *ghttp.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	value = strings.TrimSpace(strings.TrimPrefix(value, BearerPrefix))
	if value == "" {
		return ""
	}
	group := strings.Split(strings.Trim(r.URL.Path, "/"), "/")[0]
	required := false
	for _, configured := range strings.Split(DynamicToken.String(), ",") {
		if strings.TrimSpace(configured) != "" && strings.TrimSpace(configured) == group {
			required = true
		}
	}
	if decoded, ok := decodeDynamicToken(value); ok {
		return decoded
	}
	if required {
		return ""
	}
	return value
}

// 获取jwttoken
func GetJwtToken(r *ghttp.Request) (tData *TokenData, key string, err error) {
	token := GetRequestToken(r)
	tData, key, err = GetTokenData(r.GetCtx(), token)
	return
}
