package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"os"
)

// Only worker processes accept a host-supplied identity over their private channel.
func TrustedPluginIdentity(r *ghttp.Request) bool {
	secret := os.Getenv("SUXIN_PLUGIN_SECRET")
	if len(secret) != 48 || os.Getenv("SUXIN_PLUGIN_NAME") == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Suxin-Plugin-Secret")), []byte(secret)) != 1 {
		return false
	}
	var user map[string]interface{}
	if json.Unmarshal([]byte(r.Header.Get("X-Suxin-Plugin-User")), &user) != nil || user["uid"] == nil {
		return false
	}
	r.SetCtxVar("user", user)
	r.SetCtxVar("uid", user["uid"])
	return true
}
