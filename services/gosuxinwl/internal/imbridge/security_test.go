package imbridge

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/internal/accountsecurity"
	"github.com/suxinwl/GoSuxin/internal/model/entity"
	"github.com/suxinwl/GoSuxin/internal/service"
)

type passwordTestAuthority struct {
	service.IAdminUser
	store *SQLStore
}

func (a *passwordTestAuthority) AuthenticatePassword(ctx context.Context, username, password, captchaID, captcha string) (*entity.Admin, error) {
	var uid int64
	if e := a.store.DB.QueryRowContext(ctx, `SELECT id FROM gf_admin WHERE username=?`, username).Scan(&uid); e != nil {
		return nil, e
	}
	epoch, e := a.store.security().AuthenticateVersion(ctx, uid, password)
	if e != nil {
		return nil, e
	}
	return &entity.Admin{Id: uint(uid), Username: username, AuthenticatedEpoch: epoch}, nil
}
func md5Test(v string) string { h := md5.Sum([]byte(v)); return hex.EncodeToString(h[:]) }

func TestPublicForcedPasswordChangeIssuesOnlyScopedSingleUseChallenge(t *testing.T) {
	ctx, store, remote := fixture(t)
	initial := md5Test("synthetic-initial-password")
	temporary := md5Test("synthetic-reset-password")
	personal := md5Test("synthetic-personal-password")
	_, e := store.DB.ExecContext(ctx, `UPDATE gf_admin SET password=?,salt='fixture' WHERE id=1`, md5Test(initial+"fixture"))
	must(t, e)
	provision(t, ctx, store, remote)
	var previous service.IAdminUser
	func() { defer func() { _ = recover() }(); previous = service.AdminUser() }()
	service.RegisterAdminUser(&passwordTestAuthority{store: store})
	t.Cleanup(func() { service.RegisterAdminUser(previous) })
	svc := &Service{Store: store, Remote: remote, Cache: newFakeCache()}
	old, e := svc.Login(ctx, 1, "windows")
	must(t, e)
	must(t, store.security().ChangePassword(ctx, 1, 99, "", temporary, "", true))
	if _, e = svc.Validate(ctx, old.SessionToken); e == nil {
		t.Fatal("old IM session survived reset")
	}
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	bindPublic(server, svc)
	must(t, server.Start())
	t.Cleanup(func() { _ = server.Shutdown() })
	post := func(path string, input any) (int, map[string]any) {
		t.Helper()
		body, _ := json.Marshal(input)
		r, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/common/im-sso%s", server.GetListenedPort(), path), strings.NewReader(string(body)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-GoSuxin-Client-IP", "192.168.10.42")
		res, e := http.DefaultClient.Do(r)
		must(t, e)
		defer res.Body.Close()
		var out map[string]any
		must(t, json.NewDecoder(res.Body).Decode(&out))
		return res.StatusCode, out
	}
	status, response := post("/login", map[string]any{"account": "custom", "password": temporary, "platform": "windows", "policyClient": "pc"})
	if status != 428 {
		t.Fatal("reset login did not require change")
	}
	challenge := response["data"].(map[string]any)
	if challenge["reasonCode"] != "PASSWORD_CHANGE_REQUIRED" || challenge["requiresPasswordChange"] != true {
		t.Fatal("challenge contract differs")
	}
	for _, field := range []string{"imToken", "chatToken", "sessionToken", "attachmentToken", "userID"} {
		if _, present := challenge[field]; present {
			t.Fatal("challenge included full-session field")
		}
	}
	ticket := challenge["changeTicket"].(string)
	status, _ = post("/password/status", map[string]any{"changeTicket": ticket})
	if status != 200 {
		t.Fatal("challenge status failed")
	}
	status, response = post("/password/change", map[string]any{"changeTicket": ticket, "newPassword": personal})
	if status != 200 || response["data"].(map[string]any)["requiresLogin"] != true {
		t.Fatal("forced change failed")
	}
	status, _ = post("/password/change", map[string]any{"changeTicket": ticket, "newPassword": temporary})
	if status != 403 {
		t.Fatal("ticket replay accepted")
	}
	must(t, store.Reconcile(ctx))
	must(t, store.Drain(ctx, remote))
	status, response = post("/login", map[string]any{"account": "custom", "password": personal, "platform": "windows", "policyClient": "pc"})
	if status != 200 {
		t.Fatal("fresh login after change failed")
	}
	data := response["data"].(map[string]any)
	policy := data["effectivePolicy"].(map[string]any)
	if data["policyClient"] != "pc" || policy["logicalClient"] != "pc" || policy["login"] != true || policy["preview"] != true || policy["currentIP"] != "192.168.10.42" {
		t.Fatal("login effective policy incomplete")
	}
}

func TestReverseDecisionContainsCurrentGenerationAndPreviewSaveSplit(t *testing.T) {
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	p := accountsecurity.DefaultMatrix("inherit")
	p["internet"]["pc"] = accountsecurity.Cell{Login: "allow", Save: "deny"}
	_, e := store.security().SavePolicy(ctx, accountsecurity.Policy{Subject: "person", SubjectID: 1, Matrix: p}, 99)
	must(t, e)
	for _, action := range []string{"connect", "upload", "download", "preview"} {
		d, e := store.CheckAccessLogical(ctx, 1, 3, "203.0.113.8", action, "pc")
		must(t, e)
		if !d.Allowed || !d.LoginAllowed || d.SaveAllowed || d.SecurityVersion != 2 || d.EffectivePolicy.SecurityVersion != 2 || d.EffectivePolicy.Save || !d.EffectivePolicy.Preview || d.EffectivePolicy.CurrentIP != "203.0.113.8" {
			t.Fatal("incomplete generation-bound preview decision")
		}
	}
	if _, e = store.CheckAccessLogical(ctx, 1, 3, "203.0.113.8", "save", "pc"); e != ErrDenied {
		t.Fatal("save policy ignored")
	}
	if _, e = store.CheckAccessLogical(ctx, 1, 3, "192.168.10.8", "connect", "android"); e != ErrDenied {
		t.Fatal("logical client mismatched SDK")
	}
}

func TestAuthenticationProofCannotAdoptNewGeneration(t *testing.T) {
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	epoch, e := store.Epoch(ctx, 1)
	must(t, e)
	must(t, store.security().Revoke(ctx, 1))
	svc := &Service{Store: store, Remote: remote, Cache: newFakeCache()}
	if _, e = svc.Login(context.WithValue(ctx, authenticatedVersionKey{}, epoch), 1, "windows"); e == nil {
		t.Fatal("old password proof adopted a new security generation")
	}
}

func TestReverseDependencyFailureIs503AndUnknownUserIs403(t *testing.T) {
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	svc := &Service{Store: store, Remote: remote, Cache: newFakeCache()}
	key := strings.Repeat("a", 48)
	handler := svc.internalAccessHandler(key, &accessHMACVerifier{})
	check := func(user, nonce string, want int) {
		t.Helper()
		body, _ := json.Marshal(AccessCheckRequest{UserID: user, Platform: 3, LogicalClient: "pc", ClientIP: "192.168.10.42", Action: "connect"})
		request := signedAccessRequest(t, key, body, strings.Repeat(nonce, 64), time.Now())
		response := httptest.NewRecorder()
		handler(response, request)
		if response.Code != want {
			t.Fatalf("reverse status %d wanted %d", response.Code, want)
		}
		if strings.Contains(response.Body.String(), "SELECT") || strings.Contains(response.Body.String(), "account_security") {
			t.Fatal("database error detail escaped")
		}
	}
	check("emp_missing", "1", 403)
	_, e := store.DB.ExecContext(ctx, `DROP TABLE gf_account_security`)
	must(t, e)
	check(UserID("a"), "2", 503)
	must(t, store.Security.Ensure(ctx))
	_, e = store.DB.ExecContext(ctx, `DROP TABLE biz_openim_employee`)
	must(t, e)
	check(UserID("a"), "3", 503)
}
