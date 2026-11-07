package runtimeplugin

import "net/http"

// Workers receive the verified administrator identity through the private
// host channel. Browser credentials must not escape to installed plugins.
func stripHostCredentials(_, _ string, request *http.Request) {
	request.Header.Del("Authorization")
	request.Header.Del("Cookie")
}
