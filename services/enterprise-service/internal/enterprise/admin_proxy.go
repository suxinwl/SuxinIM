package enterprise

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"openim38.local/identity"
	pathpkg "path"
	"strconv"
	"strings"
)

// adminRoutes keeps the IM management credential entirely behind the trusted
// backend. A normal Chat account is never sufficient to use these proxies.
func (s *Server) adminRoutes(w http.ResponseWriter, r *http.Request, path string) bool {
	const prefix = "/admin/"
	clean := pathpkg.Clean(path)
	if !strings.HasPrefix(path, prefix) && !strings.HasPrefix(clean, prefix) {
		return false
	}
	if identity.Required() {
		writeError(w, forbidden("use the HRM management gateway"))
		return true
	}
	// Resolve harmless aliases before classifying login or applying deny rules.
	// The HTTP server has already URL-decoded Path once. Literal percent escapes
	// are rejected so URL construction cannot perform a second, different decode.
	if strings.Contains(path, "..") || strings.ContainsAny(path, "\\\x00%") {
		writeError(w, bad("invalid management route"))
		return true
	}
	path = clean
	route := strings.TrimPrefix(path, prefix)
	login := route == "login" || route == "chat/account/login"
	if !login && !strings.HasPrefix(route, "im/") && !strings.HasPrefix(route, "chat/") && !strings.HasPrefix(route, "business/") {
		return false
	}
	if r.Method != "POST" && r.Method != "GET" {
		writeError(w, bad("unsupported management method"))
		return true
	}
	var service, suffix string
	if !login {
		parts := strings.SplitN(route, "/", 2)
		if len(parts) != 2 {
			writeError(w, bad("invalid management route"))
			return true
		}
		service, suffix = parts[0], "/"+parts[1]
		// Raw runtime configuration contains IM signing secrets, database and
		// LiveKit credentials. Keep operational control entirely server-side;
		// business /client_config remains available to the original admin UI.
		if suffix == "/config" || strings.HasPrefix(suffix, "/config/") || suffix == "/restart" || strings.HasPrefix(suffix, "/restart/") {
			writeError(w, forbidden("runtime configuration is managed on the server"))
			return true
		}
	}
	if login {
		if r.Method != "POST" {
			writeError(w, bad("login requires POST"))
			return true
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
	} else {
		if _, err := s.Auth.Authenticate(r.Context(), r, true); err != nil {
			writeError(w, err)
			return true
		}
	}
	var base, token string
	if login {
		base = s.Upstream.ChatAdminBase
		suffix = "/account/login"
	} else {
		switch service {
		case "im":
			// Issuing management/client tokens through a browser is unnecessary and
			// would bypass the server-only management boundary.
			if strings.HasPrefix(suffix, "/auth/") && suffix != "/auth/force_logout" {
				writeError(w, forbidden("token issuance is internal"))
				return true
			}
			base = s.Upstream.IMBase
			var err error
			token, err = s.Upstream.AdminToken(r.Context())
			if err != nil {
				writeError(w, err)
				return true
			}
		case "chat":
			base = s.Upstream.ChatAdminBase
			token = r.Header.Get("chatToken")
			if token == "" {
				token = r.Header.Get("token")
			}
		case "business":
			if suffix == "/user/rtc/get_token" || suffix == "/rtc/get_token_for_video_meeting" {
				writeError(w, forbidden("use invitation-scoped enterprise media access"))
				return true
			}
			base = s.Upstream.ChatBase
			token = r.Header.Get("chatToken")
			if token == "" {
				token = r.Header.Get("token")
			}
		default:
			writeError(w, missing("unknown management service"))
			return true
		}
	}
	upstreamURL, err := url.Parse(strings.TrimRight(base, "/") + suffix)
	if err != nil || upstreamURL.Host == "" {
		writeError(w, bad("management upstream is not configured"))
		return true
	}
	upstreamURL.RawQuery = r.URL.RawQuery
	request, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL.String(), r.Body)
	if err != nil {
		writeError(w, err)
		return true
	}
	request.ContentLength = r.ContentLength
	request.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	request.Header.Set("operationID", newID())
	// The isolated ingress overwrites this value with its fixed public API URL.
	if publicAPI := r.Header.Get("X-Request-Api"); publicAPI != "" {
		request.Header.Set("X-Request-Api", publicAPI)
	}
	if token != "" {
		request.Header.Set("token", token)
	}
	response, err := s.Upstream.HTTP.Do(request)
	if err != nil {
		writeError(w, err)
		return true
	}
	defer response.Body.Close()
	w.Header().Set("Cache-Control", "no-store")
	online := service == "im" && (suffix == "/user/get_users_online_status" || suffix == "/user/get_users_online_token_detail")
	profile := service == "chat" && suffix == "/account/info"
	if login || online || profile {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, (16<<20)+1))
		if readErr != nil || len(body) > 16<<20 {
			writeError(w, &apiError{503, 1006, "invalid management response"})
			return true
		}
		var envelope any
		if err = json.Unmarshal(body, &envelope); err != nil {
			writeError(w, err)
			return true
		}
		sanitizeManagementResponse(envelope, online || profile)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(response.StatusCode)
		_ = json.NewEncoder(w).Encode(envelope)
		return true
	}
	for _, header := range []string{"Content-Type", "Content-Disposition"} {
		if value := response.Header.Get(header); value != "" {
			w.Header().Set(header, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
	return true
}

// Fixed protocol v0.0.73-alpha.19 platform_id_to_name.go. The enterprise module
// intentionally does not import a second protocol version into its module graph.
var managementPlatformNames = map[int]string{1: "IOS", 2: "Android", 3: "Windows", 4: "OSX", 5: "Web", 6: "MiniWeb", 7: "Linux", 8: "APad", 9: "IPad", 10: "Admin", 11: "HarmonyOS", 12: "Bot"}

func sanitizeManagementResponse(value any, online bool) {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			sanitizeManagementResponse(item, online)
		}
	case map[string]any:
		for key, item := range v {
			normal := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
			remove := normal == "imtoken" || normal == "imadmintoken" || normal == "openimtoken" || normal == "openimadmintoken"
			if online {
				switch normal {
				case "token", "tokens", "tokenstates", "tokensmap", "chattoken", "admintoken", "accesstoken", "refreshtoken", "pushtoken", "fcmtoken", "credential", "credentials", "password", "secret":
					remove = true
				}
			}
			if remove {
				delete(v, key)
			} else {
				sanitizeManagementResponse(item, online)
			}
		}
		if online {
			var id int
			switch p := v["platformID"].(type) {
			case float64:
				if p == float64(int(p)) {
					id = int(p)
				}
			case string:
				id, _ = strconv.Atoi(p)
			}
			if id > 0 {
				if name, ok := managementPlatformNames[id]; ok {
					v["platform"] = name
				} else {
					v["platform"] = "Platform " + strconv.Itoa(id)
				}
			}
		}
	}
}
