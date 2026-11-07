package imbridge

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func setAvatarProjection(t *testing.T, store *SQLStore, face string) {
	t.Helper()
	var raw string
	must(t, store.DB.QueryRow(`SELECT projection_json FROM biz_openim_employee WHERE employee_id='a'`).Scan(&raw))
	var p Projection
	must(t, json.Unmarshal([]byte(raw), &p))
	p.Profile.FaceURL, p.FaceURL = face, face
	b, e := json.Marshal(p)
	must(t, e)
	_, e = store.DB.Exec(`UPDATE biz_openim_employee SET projection_json=? WHERE employee_id='a'`, string(b))
	must(t, e)
}

func TestDirectoryAvatarExactAllowlistRejectsUnknownForeignAndTraversal(t *testing.T) {
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	setAvatarProjection(t, store, "https://hr.szputy.com:9880/resource/uploads/avatars/employee.png?v=1")
	for _, tc := range []struct {
		path    string
		allowed bool
	}{
		{"/resource/uploads/avatars/employee.png?v=1", true},
		{"/resource/uploads/avatars/employee.png", false},
		{"/resource/uploads/avatars/employee.png?v=2", false},
		{"/resource/uploads/private/payroll.xlsx", false},
		{"/resource/uploads/avatars/../private/payroll.xlsx", false},
		{"/resource/uploads/%2e%2e/private.txt", false},
		{"/resource/uploads/avatar%5c..%5cprivate.txt", false},
		{"https://hr.szputy.com:9880/resource/uploads/avatars/employee.png?v=1", false},
		{"//outside.example/resource/uploads/avatars/employee.png?v=1", false},
	} {
		allowed, e := store.IsDirectoryAvatarResource(ctx, tc.path)
		must(t, e)
		if allowed != tc.allowed {
			t.Errorf("path %s allowed %v wanted %v", tc.path, allowed, tc.allowed)
		}
	}
	setAvatarProjection(t, store, "https://foreign.example/resource/uploads/avatars/employee.png?v=1")
	if allowed, e := store.IsDirectoryAvatarResource(ctx, "/resource/uploads/avatars/employee.png?v=1"); e != nil || allowed {
		t.Fatal("foreign origin projection whitelisted local file")
	}
}

func TestPublicAvatarNeedsLiveCommunicationGrantWithoutPublicAttachmentDownload(t *testing.T) {
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	setAvatarProjection(t, store, "https://hr.szputy.com:9880/resource/uploads/avatar.png")
	svc := &Service{Store: store, Remote: remote, Cache: newFakeCache()}
	_, e := svc.SaveEmployeeAccess(ctx, EmployeeAccess{EmployeeID: "a", AllowLAN: true, AllowPC: true, AllowInternet: true}, 0, 99)
	must(t, e)
	key := strings.Repeat("a", 48)
	handler := svc.internalAccessHandler(key, &accessHMACVerifier{})
	for index, tc := range []struct {
		action, path string
		status       int
	}{{"avatar", "/resource/uploads/avatar.png", 200}, {"avatar", "/resource/uploads/private.sql", 403}, {"preview", "", 200}, {"save", "", 403}} {
		body, _ := json.Marshal(AccessCheckRequest{UserID: UserID("a"), Platform: 3, ClientIP: "203.0.113.8", Action: tc.action, ResourcePath: tc.path})
		nonce := strings.Repeat(string(rune('a'+index)), 64)
		r := signedAccessRequest(t, key, body, nonce, time.Now())
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != tc.status {
			t.Errorf("action %s status %d wanted %d", tc.action, w.Code, tc.status)
		}
	}
}
