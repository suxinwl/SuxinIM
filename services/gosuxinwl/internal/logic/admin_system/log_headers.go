package adminsystem

import (
	"net/http"
	"net/url"
)

// Clone first: logging must not remove the credentials needed by the handler.
func redactedLogHeaders(headers http.Header) http.Header {
	clean := headers.Clone()
	for _, name := range []string{"Authorization", "X-Original-Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie", "Apiverify", "X-Auth-Token", "X-Suxin-Plugin-Secret", "X-Suxin-Plugin-User", "X-Devtools-Token"} {
		clean.Del(name)
	}
	if value := clean.Get("Referer"); value != "" {
		if parsed, err := url.Parse(value); err == nil {
			parsed.RawQuery = ""
			parsed.Fragment = ""
			parsed.User = nil
			clean.Set("Referer", parsed.String())
		} else {
			clean.Del("Referer")
		}
	}
	return clean
}
