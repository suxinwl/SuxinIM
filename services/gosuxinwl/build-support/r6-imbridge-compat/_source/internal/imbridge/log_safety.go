package imbridge

import "net/http"

// SSO uses JSON bodies exclusively. Remove URL/referrer query credentials
// before framework access/error logging, without reading the request body.
func sanitizeSSOLogURL(r *http.Request) {
	r.URL.RawQuery = ""
	r.URL.Fragment = ""
	r.RequestURI = r.URL.EscapedPath()
	r.Header.Del("Referer")
}
