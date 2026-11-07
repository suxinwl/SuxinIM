package auth

import (
	"fmt"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
	"github.com/suxinwl/GoSuxin/utility/tools/cryptojs"
)

// RequestTimingDiagnostic reports only transport presence and clock difference.
// Never include credentials, decoded tokens, cookies or request bodies in logs.
func RequestTimingDiagnostic(r *ghttp.Request) (result string) {
	value := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), BearerPrefix))
	result = fmt.Sprintf("authorization_present=%t", value != "")
	defer func() { _ = recover() }()
	decoded, err := cryptojs.AesDecrypt(value)
	if err != nil {
		return result
	}
	parts := strings.Split(decoded, "#")
	if len(parts) == 2 && parts[0] != "" {
		result += fmt.Sprintf(" dynamic_token_age_seconds=%d", time.Now().Unix()-gconv.Int64(parts[1]))
	}
	return result
}
