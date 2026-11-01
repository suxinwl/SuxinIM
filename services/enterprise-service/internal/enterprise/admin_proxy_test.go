package enterprise

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminProxyKeepsManagementCredentialOnServer(t *testing.T) {
	var observed string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed = r.Header.Get("token")
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/account/login" {
			io.WriteString(w, `{"errCode":0,"data":{"adminToken":"chat-admin-session","imToken":"must-never-reach-browser"}}`)
			return
		}
		if r.URL.Path == "/auth/get_admin_token" {
			io.WriteString(w, `{"errCode":0,"data":{"token":"private-management-token","expireTimeSeconds":86400}}`)
			return
		}
		io.WriteString(w, `{"errCode":0,"data":{"ok":true}}`)
	}))
	defer upstream.Close()
	s := &Server{Auth: &testAuth{}, Upstream: &Upstream{IMBase: upstream.URL, ChatAdminBase: upstream.URL, HTTP: upstream.Client(), IMAdminID: "private-admin", IMSecret: "private-secret"}}
	request := func(path, user string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
		req.Header.Set("chatToken", user)
		rr := httptest.NewRecorder()
		if !s.adminRoutes(rr, req, path) {
			t.Fatal("management route not handled")
		}
		return rr
	}
	if rr := request("/admin/login", ""); rr.Code != 200 || strings.Contains(rr.Body.String(), "imToken") || strings.Contains(rr.Body.String(), "must-never") {
		t.Fatalf("login leaked private token: %d", rr.Code)
	}
	observed = ""
	if rr := request("/admin/im/user/get_users_info", "alice"); rr.Code != 403 || observed != "" {
		t.Fatalf("ordinary account reached management upstream: %d", rr.Code)
	}
	if rr := request("/admin/im/auth/get_user_token", "admin"); rr.Code != 403 {
		t.Fatalf("browser may mint client tokens: %d", rr.Code)
	}
	if rr := request("/admin/im/user/get_users_info", "admin"); rr.Code != 200 || observed != "private-management-token" {
		t.Fatalf("server credential not used: %d", rr.Code)
	}
	if rr := request("/admin/im/auth/force_logout", "admin"); rr.Code != 200 {
		t.Fatalf("admin cannot revoke client sessions: %d", rr.Code)
	}
	if _, err := s.Upstream.AdminToken(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestAdminProfileDoesNotExposePasswordHash(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/account/info" {
			t.Errorf("unexpected profile route %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"errCode":0,"data":{"userID":"admin","nickname":"profile preserved","password":"credential-hash","nested":{"password":"second-hash"}}}`)
	}))
	defer upstream.Close()
	s := &Server{Auth: &testAuth{}, Upstream: &Upstream{ChatAdminBase: upstream.URL, HTTP: upstream.Client()}}
	for _, path := range []string{"/admin/chat/account/info", "/admin/chat/account/./info"} {
		r := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
		r.Header.Set("chatToken", "admin")
		w := httptest.NewRecorder()
		if !s.adminRoutes(w, r, path) || w.Code != 200 || strings.Contains(w.Body.String(), "password") || strings.Contains(w.Body.String(), "hash") || !strings.Contains(w.Body.String(), "profile preserved") {
			t.Fatalf("profile response failed credential redaction: status %d", w.Code)
		}
	}
}

func TestAdminLoginAliasesAlwaysStripIMCredentials(t *testing.T) {
	hits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/account/login" {
			t.Errorf("login alias reached an unnormalized path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"errCode":0,"data":{"adminToken":"browser-chat-session","imToken":"leak-main","nested":{"IM_Admin_Token":"leak-nested","OpenIMToken":"leak-openim"}}}`)
	}))
	defer upstream.Close()
	s := &Server{Auth: &testAuth{}, Upstream: &Upstream{ChatAdminBase: upstream.URL, HTTP: upstream.Client()}}
	aliases := []string{
		"/admin/login", "/admin/login/", "/admin//login",
		"/admin/chat/account/login", "/admin/chat/account/./login", "/admin//chat//account//login/",
		"/./admin/chat/account/login", "/admin/chat/account/%2e/login", "/admin/chat/account%2flogin/",
	}
	for _, alias := range aliases {
		t.Run(alias, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/enterprise/v1"+alias, strings.NewReader(`{}`))
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != 200 || strings.Contains(w.Body.String(), "leak-") || !strings.Contains(w.Body.String(), "browser-chat-session") {
				t.Fatalf("login alias leaked credentials or lost Chat login: %d %s", w.Code, w.Body.String())
			}
		})
	}
	if hits != len(aliases) {
		t.Fatalf("expected only canonical login calls: %d", hits)
	}
}

func TestAdminRuntimeConfigurationAndMalformedRoutesNeverReachUpstream(t *testing.T) {
	hits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"errCode":0,"data":{"ok":true}}`)
	}))
	defer upstream.Close()
	s := &Server{Auth: &testAuth{}, Upstream: &Upstream{IMBase: upstream.URL, ChatAdminBase: upstream.URL, ChatBase: upstream.URL, HTTP: upstream.Client()}}
	for _, service := range []string{"im", "chat", "business"} {
		for _, route := range []string{"/config", "/config/get_config", "//config/./get_config/", "/config/set_config", "/restart", "//restart/", "/restart/./now"} {
			r := httptest.NewRequest("POST", "/enterprise/v1/admin/"+service+route, strings.NewReader(`{"configName":"share.yml"}`))
			r.Header.Set("chatToken", "admin")
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != 403 {
				t.Fatalf("raw operational route accepted: %s%s HTTP %d", service, route, w.Code)
			}
		}
	}
	for _, path := range []string{"/admin/chat/account/../login", "/admin/chat/account/..hidden/login", "/admin/chat/config\\get_config", "/admin/chat/config/\x00", "/admin/chat/account/%2e/login"} {
		r := httptest.NewRequest("POST", "/enterprise/v1/admin/login", strings.NewReader(`{}`))
		// Literal percent escapes here represent double-encoded HTTP paths after
		// net/http has decoded URL.Path once; do not decode again on forwarding.
		r.URL.Path = "/enterprise/v1" + path
		r.Header.Set("chatToken", "admin")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatalf("malformed route accepted: %q HTTP %d", path, w.Code)
		}
	}
	if hits != 0 {
		t.Fatalf("blocked management requests reached upstream %d times", hits)
	}
	// Client-facing business configuration is distinct from runtime secrets.
	w := post(s, "admin", "/enterprise/v1/admin/chat/client_config/get", map[string]any{})
	if w.Code != 200 || hits != 1 {
		t.Fatalf("original client_config was blocked: %d", w.Code)
	}
}

func TestAdminOnlineResponsesRemoveCredentialsAndRestorePlatformNames(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/get_admin_token":
			io.WriteString(w, `{"errCode":0,"data":{"token":"server-management-only"}}`)
		case "/user/get_users_online_token_detail":
			io.WriteString(w, `{"errCode":0,"data":[{"userID":"alice","status":1,"singlePlatformToken":[{"platformID":3,"total":2,"token":["leak-one","leak-two"],"tokens":["leak-list"],"imToken":"leak-im","nested":{"chatToken":"leak-chat","refresh_token":"leak-refresh","tokenStates":{"leak-map-key":1}}},{"platformID":2,"total":1,"token":["leak-android"]}]},{"userID":"offline","status":0,"singlePlatformToken":[]}]}`)
		case "/user/get_users_online_status":
			io.WriteString(w, `{"errCode":0,"data":[{"userID":"alice","status":1,"detailPlatformStatus":[{"platformID":5,"token":"leak-web","connID":"conn-one","isBackground":false},{"platformID":"1","Token":"leak-ios","IM_Admin_Token":"leak-admin"}]}]}`)
		default:
			t.Errorf("unexpected upstream path: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	s := &Server{Auth: &testAuth{}, Upstream: &Upstream{IMBase: upstream.URL, HTTP: upstream.Client()}}
	for _, route := range []string{"get_users_online_token_detail", "get_users_online_status"} {
		w := post(s, "admin", "/enterprise/v1/admin/im/user/./"+route+"/", map[string]any{"userIDs": []string{"alice", "offline"}})
		if w.Code != 200 || strings.Contains(w.Body.String(), "leak-") || strings.Contains(w.Body.String(), "server-management-only") {
			t.Fatalf("online response leaked credentials: %d %s", w.Code, w.Body.String())
		}
		var envelope struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		field := "singlePlatformToken"
		want := []string{"Windows", "Android"}
		if route == "get_users_online_status" {
			field, want = "detailPlatformStatus", []string{"Web", "IOS"}
		}
		platforms := envelope.Data[0][field].([]any)
		for index, item := range platforms {
			platform := item.(map[string]any)
			if platform["platform"] != want[index] || platform["platformID"] == nil {
				t.Fatalf("official platform ID was not adapted: %+v", platform)
			}
		}
		if route == "get_users_online_token_detail" {
			if platforms[0].(map[string]any)["total"] != float64(2) || len(envelope.Data[1][field].([]any)) != 0 {
				t.Fatal("sanitization removed platform counts or offline structure")
			}
		}
	}
}
