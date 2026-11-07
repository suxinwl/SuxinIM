package imbridge

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Desktop loads file:// resources with webSecurity enabled. Its opaque null
// origin may send password/sid JSON, but never receives credentialed CORS.
// Host cookies remain available only to the configured web origin.
func ssoCORS(w http.ResponseWriter, r *http.Request, publicOrigin string) bool {
	origin := r.Header.Get("Origin")
	if publicOrigin == "" {
		publicOrigin = "https://kf.szputy.com:9880"
	}
	publicOrigin = strings.TrimRight(publicOrigin, "/")
	w.Header().Add("Vary", "Origin")
	if origin != "" {
		loopbackSameOrigin := false
		if parsed, err := url.Parse(origin); err == nil && parsed.User == nil && parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == "" {
			ip := net.ParseIP(parsed.Hostname())
			scheme := "http"
			if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
				scheme = "https"
			}
			loopbackSameOrigin = ip != nil && ip.IsLoopback() && parsed.Scheme == scheme && parsed.Host == r.Host
		}
		if origin != "null" && origin != publicOrigin && !loopbackSameOrigin {
			envelope(w, http.StatusForbidden, nil, errors.New("通讯请求来源无效"))
			return false
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		if origin != "null" {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
	}
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		w.Header().Set("Access-Control-Max-Age", "600")
		if origin == "null" && r.Header.Get("Access-Control-Request-Private-Network") == "true" {
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		}
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	return true
}
