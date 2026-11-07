package adminuser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/suxinwl/GoSuxin/api/admin/user"
	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	"github.com/suxinwl/GoSuxin/framework/crypto/gmd5"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/os/gcfg"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/internal/imbridge"
	"github.com/suxinwl/GoSuxin/utility/auth"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"github.com/suxinwl/GoSuxin/utility/tools/cryptojs"
)

func TestHRMPasswordLoginPublicPolicyRunsAfterCredentialsBeforeToken(t *testing.T) {
	t.Setenv("GOSUXIN_HRM_PUBLIC_ADMINS_ONLY", "true")
	t.Setenv("GOSUXIN_IM_ENABLED", "true")
	previousDB, previousGroup := gdb.GetAllConfig(), gdb.GetDefaultGroup()
	if err := gdb.SetConfig(gdb.Config{"default": {{Type: "sqlite", Name: filepath.Join(t.TempDir(), "hrm-login.db"), Prefix: "gf_"}}}); err != nil {
		t.Fatal(err)
	}
	gdb.SetDefaultGroup("default")
	db := g.DB()
	ctx := context.Background()
	t.Cleanup(func() { _ = db.Close(ctx); _ = gdb.SetConfig(previousDB); gdb.SetDefaultGroup(previousGroup) })
	app := g.Cfg("app")
	previousApp := app.GetAdapter()
	setCaptcha := func(enabled bool) {
		adapter, err := gcfg.NewAdapterContent(fmt.Sprintf(`{"app":{"loginCaptcha":%t}}`, enabled))
		if err != nil {
			t.Fatal(err)
		}
		app.SetAdapter(adapter)
	}
	setCaptcha(false)
	t.Cleanup(func() { app.SetAdapter(previousApp) })
	secret, encrypt, dynamic := auth.SecretKey, auth.EncryptKey, auth.DynamicToken
	timeout, refresh, multi, cache := auth.TokenTimeout, auth.MaxRefresh, auth.MultiLogin, auth.TokenCacheKey
	auth.SecretKey, auth.EncryptKey, auth.DynamicToken = gvar.New("hrm-login-test-signing-secret"), gvar.New("0123456789abcdef0123456789abcdef"), gvar.New("")
	auth.TokenTimeout, auth.MaxRefresh, auth.MultiLogin, auth.TokenCacheKey = gvar.New(600), gvar.New(0), gvar.New(false), gvar.New("hrm-login-test-"+guid.S())
	t.Cleanup(func() {
		_ = auth.RemoveToken(ctx, "1")
		_ = auth.RemoveToken(ctx, "2")
		auth.SecretKey, auth.EncryptKey, auth.DynamicToken = secret, encrypt, dynamic
		auth.TokenTimeout, auth.MaxRefresh, auth.MultiLogin, auth.TokenCacheKey = timeout, refresh, multi, cache
	})
	for _, statement := range []string{
		`CREATE TABLE gf_admin(id INTEGER PRIMARY KEY,username TEXT,password TEXT,salt TEXT,status INTEGER,deletetime DATETIME,login_attempts INTEGER DEFAULT 0,lock_time DATETIME,loginip TEXT,logintime DATETIME)`,
		`CREATE TABLE gf_login_log(id INTEGER PRIMARY KEY,uid INTEGER,username TEXT,ip TEXT,address TEXT,des TEXT,os TEXT,browser TEXT,error_msg TEXT,status INTEGER,createtime DATETIME)`,
		`CREATE TABLE gf_auth_role(id INTEGER PRIMARY KEY,rules TEXT,btns TEXT,status INTEGER)`,
		`CREATE TABLE gf_auth_role_access(uid INTEGER,role_id INTEGER)`,
		`CREATE TABLE gf_auth_rule(id INTEGER PRIMARY KEY,permission TEXT,status INTEGER,type INTEGER)`,
		`CREATE TABLE biz_openim_access_network(singleton INTEGER PRIMARY KEY,revision BIGINT,cidrs_json TEXT,updated_at BIGINT,updated_by BIGINT)`,
		`CREATE TABLE biz_openim_epoch(uid BIGINT PRIMARY KEY,epoch BIGINT NOT NULL)`,
		`INSERT INTO biz_openim_access_network VALUES(1,1,'["192.168.10.0/24"]',0,0)`,
		`INSERT INTO gf_auth_role VALUES(1,'*','',0)`,
		`INSERT INTO gf_auth_role_access VALUES(2,1)`,
	} {
		if _, err := db.Exec(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	password := gmd5.MustEncryptString("Synthetic-Login-Test-Passphrase")
	for uid, account := range map[int]string{1: "ordinary", 2: "manager"} {
		if _, err := db.Exec(ctx, `INSERT INTO gf_admin(id,username,password,salt,status) VALUES(?,?,?,?,0)`, uid, account, gmd5.MustEncryptString(password+"fixture-salt"), "fixture-salt"); err != nil {
			t.Fatal(err)
		}
	}
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.BindHandler("POST:/admin/user/login", func(r *ghttp.Request) {
		r.Response.WriteJson(NewAdminUser().Login(r.Context(), &user.LoginReq{Username: r.Get("username").String(), Password: r.Get("password").String()}))
	})
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	login := func(account, input, ip string) (gf.R, *http.Response) {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"username": account, "password": input})
		req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:%d/admin/user/login", server.GetListenedPort()), strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-GoSuxin-Client-IP", ip)
		req.Header.Set("X-Forwarded-For", "192.168.10.9")
		req.Header.Set("X-Forwarded-Proto", "https")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var result gf.R
		if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result, response
	}
	lan, _ := login("ordinary", password, "192.168.10.9")
	if !lan.Ok() {
		t.Fatal("existing LAN employee password login stopped working")
	}
	opaque, err := cryptojs.AesDecrypt(lan.Data.(string))
	if err != nil {
		t.Fatal(err)
	}
	before, _, err := auth.GetTokenData(ctx, opaque)
	if err != nil {
		t.Fatal(err)
	}
	denied, response := login("ordinary", password, "203.0.113.9")
	if denied.Ok() || denied.Message != imbridge.ErrHRMPublicAdminRequired.Error() || len(response.Cookies()) != 0 {
		t.Fatal("ordinary employee public login was not rejected before cookie issuance")
	}
	after, _, err := auth.GetTokenData(ctx, opaque)
	if err != nil || after.UuId != before.UuId || after.JwtToken != before.JwtToken {
		t.Fatal("rejected public login replaced the original LAN session")
	}
	wrong, _ := login("ordinary", "incorrect-password-md5", "203.0.113.9")
	if wrong.Ok() || strings.Contains(wrong.Message, "公网") {
		t.Fatal("source policy ran before password validation")
	}
	setCaptcha(true)
	for i := 0; i < 7; i++ {
		invalid, _ := login("manager", strings.Repeat("0", 32), "203.0.113.9")
		if invalid.Ok() {
			t.Fatal("missing captcha accepted")
		}
	}
	var failures int
	master, masterErr := db.Master()
	if masterErr != nil {
		t.Fatal(masterErr)
	}
	if err = master.QueryRowContext(ctx, `SELECT COUNT(*) FROM gf_account_failure WHERE uid=2`).Scan(&failures); err != nil || failures != 0 {
		t.Fatal("captcha errors counted as password failures")
	}
	missingCaptcha, _ := login("manager", password, "203.0.113.9")
	if missingCaptcha.Ok() || strings.Contains(missingCaptcha.Message, "公网") {
		t.Fatal("administrator bypassed the existing captcha")
	}
	setCaptcha(false)
	manager, response := login("manager", password, "203.0.113.9")
	if !manager.Ok() {
		t.Fatal("authorized public administrator password login failed")
	}
	found := false
	for _, cookie := range response.Cookies() {
		if cookie.Name == "GoSuxinHRMAsset" {
			found = cookie.HttpOnly && cookie.Secure && cookie.Domain == "" && cookie.Path == "/resource/uploads/" && cookie.SameSite == http.SameSiteLaxMode
		}
	}
	if !found {
		t.Fatal("public HRM login did not issue a scoped secure asset cookie")
	}
	// The production policy allows ordinary HRM staff on both entrances. Asset
	// authentication must stay enabled when the optional admin-only flag is off.
	t.Setenv("GOSUXIN_HRM_PUBLIC_ADMINS_ONLY", "false")
	ordinary, response := login("ordinary", password, "203.0.113.9")
	if !ordinary.Ok() {
		t.Fatal("ordinary HRM public login failed with the optional restriction disabled")
	}
	found = false
	for _, cookie := range response.Cookies() {
		if cookie.Name == "GoSuxinHRMAsset" {
			found = cookie.HttpOnly && cookie.Secure && cookie.Path == "/resource/uploads/"
		}
	}
	if !found {
		t.Fatal("ordinary login omitted the resource cookie with admin-only policy disabled")
	}
	currentOpaque, err := cryptojs.AesDecrypt(ordinary.Data.(string))
	if err != nil {
		t.Fatal(err)
	}
	current, _, err := auth.GetTokenData(ctx, currentOpaque)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := auth.JwtParseToken(current.JwtToken)
	if err != nil {
		t.Fatal(err)
	}
	security, _ := claims.Data.(map[string]interface{})
	if security["hrmAssetCredential"] == nil || security["hrmAssetEpoch"] == nil {
		t.Fatal("ordinary password login omitted its immutable resource security state")
	}
}
