package imbridge

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
)

func fixture(t *testing.T) (context.Context, *SQLStore, *fakeRemote) {
	t.Helper()
	t.Setenv("GOSUXIN_IM_AUTO_DEPARTMENT_GROUPS", "true")
	ctx := WithClientIP(context.Background(), "192.168.10.10")
	db, e := gdb.New(gdb.ConfigNode{Type: "sqlite", Name: filepath.Join(t.TempDir(), "imbridge.db"), Prefix: "gf_"})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = db.Close(ctx) })
	sqlDB, e := db.Master()
	if e != nil {
		t.Fatal(e)
	}
	s, e := NewSQLStore(sqlDB, "gf_")
	if e != nil {
		t.Fatal(e)
	}
	queries := []string{
		`CREATE TABLE gf_admin(id INTEGER PRIMARY KEY,account_id INTEGER,dept_id INTEGER,username TEXT,password TEXT,salt TEXT,name TEXT,nickname TEXT,avatar TEXT,email TEXT,mobile TEXT,tel TEXT,status INTEGER,remark TEXT,createtime DATETIME,updatetime DATETIME,deletetime DATETIME,login_attempts INTEGER DEFAULT 0,lock_time DATETIME,pwd_reset_time DATETIME)`,
		`CREATE TABLE gf_auth_dept(id INTEGER PRIMARY KEY,name TEXT,pid INTEGER DEFAULT 0,weigh INTEGER DEFAULT 0,status INTEGER,business_id INTEGER DEFAULT 0,deletetime DATETIME)`,
		`CREATE TABLE gf_auth_role(id INTEGER PRIMARY KEY,business_id INTEGER,account_id INTEGER,pid INTEGER,name TEXT,rules TEXT,menu TEXT,btns TEXT,status INTEGER,data_access INTEGER,remark TEXT,weigh INTEGER,createtime DATETIME)`,
		`CREATE TABLE gf_auth_role_access(uid INTEGER,role_id INTEGER)`,
		`CREATE TABLE gf_auth_rule(id INTEGER PRIMARY KEY,pid INTEGER,type INTEGER,routename TEXT,permission TEXT,status INTEGER)`,
		`CREATE TABLE biz_hrm_employee(id TEXT PRIMARY KEY,employee_no TEXT,user_id INTEGER,dept_id INTEGER,name TEXT,status TEXT,position TEXT,mobile TEXT DEFAULT '')`,
		`INSERT INTO gf_auth_rule(id,pid,type,routename,permission,status) VALUES(1,0,1,'runtime_plugin_biz-suite','',0),(2,1,1,'biz_suite_im','',0),(3,1,2,'','runtime.plugin.biz-suite.admin',0),(4,1,2,'','biz-suite.im.read',0),(5,1,2,'','biz-suite.im.write',0)`,
		`INSERT INTO gf_auth_dept(id,name,status) VALUES(1,'直属部门',0),(2,'另一部门',0)`,
		`INSERT INTO gf_admin(id,username,password,salt,name,status,dept_id) VALUES(1,'custom','preserved-password','original-salt','甲',1,1),(2,'conflicting','existing-password','salt','乙',0,1)`,
		`INSERT INTO biz_hrm_employee(id,employee_no,user_id,dept_id,name,status,position) VALUES('a','0001',1,1,'甲','active','职员'),('b',NULL,NULL,1,'无工号','active','职员'),('c','0003',NULL,2,'新员工','active','职员'),('d','conflicting',NULL,2,'冲突','active','职员')`,
	}
	for _, q := range queries {
		if _, e = s.DB.ExecContext(ctx, q); e != nil {
			t.Fatal(e)
		}
	}
	if e = s.Ensure(ctx); e != nil {
		t.Fatal(e)
	}
	return ctx, s, &fakeRemote{users: map[string]Projection{}, departments: map[string]Department{}}
}
func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func provision(t *testing.T, ctx context.Context, s *SQLStore, r *fakeRemote) {
	t.Helper()
	must(t, s.ConfirmProvision(ctx))
	must(t, s.Reconcile(ctx))
	must(t, s.Drain(ctx, r))
}

type fakeRemote struct {
	mu          sync.Mutex
	users       map[string]Projection
	departments map[string]Department
	revokes     []Revoke
	fail        bool
	exchanges   int
}

func (r *fakeRemote) Exchange(_ context.Context, _ string, s Session) (Tokens, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return Tokens{}, errors.New("unavailable")
	}
	r.exchanges++
	return Tokens{IMToken: "im-token-" + s.ID, ChatToken: "chat-token-" + s.ID, AttachmentToken: "attachment-token-" + s.ID}, nil
}
func (r *fakeRemote) Revoke(_ context.Context, v Revoke) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return errors.New("unavailable")
	}
	r.revokes = append(r.revokes, v)
	return nil
}
func (r *fakeRemote) User(_ context.Context, v Projection) error {
	if r.fail {
		return errors.New("unavailable")
	}
	r.users[v.UserID] = v
	return nil
}
func (r *fakeRemote) Department(_ context.Context, v Department) error {
	if r.fail {
		return errors.New("unavailable")
	}
	r.departments[v.DepartmentID] = v
	return nil
}
func (r *fakeRemote) Manage(context.Context, string, any) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}

func TestProvisioningPreservesPasswordsAndHonorsPending(t *testing.T) {
	ctx, s, r := fixture(t)
	must(t, s.Reconcile(ctx))
	var status int
	must(t, s.DB.QueryRowContext(ctx, `SELECT status FROM gf_admin WHERE id=1`).Scan(&status))
	if status != 1 {
		t.Fatal("unconfirmed provisioning enabled a production account")
	}
	if len(r.users) != 0 {
		t.Fatal("unexpected remote user")
	}
	provision(t, ctx, s, r)
	var password, salt string
	must(t, s.DB.QueryRowContext(ctx, `SELECT status,password,salt FROM gf_admin WHERE id=1`).Scan(&status, &password, &salt))
	if status != 0 || password != "preserved-password" || salt != "original-salt" {
		t.Fatal("initial provisioning changed credentials")
	}
	if len(r.users) != 2 {
		t.Fatalf("wanted two eligible users, got %d", len(r.users))
	}
	for _, id := range []string{"b", "d"} {
		var enabled int
		must(t, s.DB.QueryRowContext(ctx, `SELECT enabled FROM biz_openim_employee WHERE employee_id=?`, id).Scan(&enabled))
		if enabled != 0 {
			t.Fatalf("pending %s was enabled", id)
		}
	}
	p, e := s.Eligible(ctx, 1)
	must(t, e)
	if p.UserID != UserID("a") || p.Profile.FaceURL != "https://hr.szputy.com:9880/resource/static/brand/default-avatar.png?v=puty2" {
		t.Fatal("bad stable identity/avatar")
	}
	_, e = s.DB.ExecContext(ctx, `UPDATE gf_admin SET status=1 WHERE id=1`)
	must(t, e)
	must(t, s.Reconcile(ctx))
	must(t, s.Drain(ctx, r))
	must(t, s.Reconcile(ctx))
	must(t, s.DB.QueryRowContext(ctx, `SELECT status FROM gf_admin WHERE id=1`).Scan(&status))
	if status != 1 {
		t.Fatal("scanner re-enabled a manually disabled account")
	}
	if _, e = s.Eligible(ctx, 1); !errors.Is(e, ErrDenied) {
		t.Fatal("disabled identity remained eligible")
	}
	if len(r.revokes) == 0 || !r.revokes[len(r.revokes)-1].All {
		t.Fatal("disable did not revoke all sessions")
	}
}
func TestStableIdentityDepartmentProjectionAndRetries(t *testing.T) {
	ctx, s, r := fixture(t)
	provision(t, ctx, s, r)
	var leader2 int64
	must(t, s.DB.QueryRowContext(ctx, `SELECT user_id FROM biz_hrm_employee WHERE id='c'`).Scan(&leader2))
	_, e := s.DB.ExecContext(ctx, `UPDATE gf_auth_dept SET im_group_enabled=1,leader_uid=CASE WHEN id=1 THEN 1 ELSE ? END`, leader2)
	must(t, e)
	must(t, s.Reconcile(ctx))
	must(t, s.Drain(ctx, r))
	_, e = s.DB.ExecContext(ctx, `UPDATE biz_hrm_employee SET employee_no='0099',dept_id=2 WHERE id='a'`)
	must(t, e)
	must(t, s.Reconcile(ctx))
	r.fail = true
	must(t, s.Drain(ctx, r))
	var desired, applied int64
	must(t, s.DB.QueryRowContext(ctx, `SELECT desired_version,applied_version FROM biz_openim_employee WHERE employee_id='a'`).Scan(&desired, &applied))
	if desired == applied {
		t.Fatal("failed projection was marked applied")
	}
	r.fail = false
	must(t, s.Retry(ctx))
	must(t, s.Drain(ctx, r))
	must(t, s.Reconcile(ctx))
	must(t, s.Drain(ctx, r))
	if r.users[UserID("a")].Account != "0099" {
		t.Fatal("work number change lost stable identity")
	}
	if r.departments["1"].Frozen || len(r.departments["1"].MemberUserIDs) != 1 || r.departments["1"].LeaderUserID != UserID("a") {
		t.Fatal("explicit department owner must remain a member without freezing history")
	}
	if len(r.departments["2"].MemberUserIDs) != 2 {
		t.Fatal("wrong direct department membership")
	}
	// A ready projection is sent again after bridge state loss, at the same version.
	r.users = map[string]Projection{}
	r.departments = map[string]Department{}
	must(t, s.Reconcile(ctx))
	must(t, s.Drain(ctx, r))
	if len(r.users) != 2 || len(r.departments) != 2 {
		t.Fatal("heartbeat did not repair bridge state loss")
	}
}

type fakeCache struct {
	mu       sync.Mutex
	values   map[string][]byte
	platform map[string]string
}

func newFakeCache() *fakeCache {
	return &fakeCache{values: map[string][]byte{}, platform: map[string]string{}}
}
func (c *fakeCache) Put(_ context.Context, k string, v any, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, e := json.Marshal(v)
	c.values[k] = b
	return e
}
func (c *fakeCache) Get(_ context.Context, k string, v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.values[k]
	if !ok {
		return ErrExpired
	}
	return json.Unmarshal(b, v)
}
func (c *fakeCache) Take(_ context.Context, k string, v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.values[k]
	if !ok {
		return ErrExpired
	}
	delete(c.values, k)
	return json.Unmarshal(b, v)
}
func (c *fakeCache) Delete(_ context.Context, k string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.values, k)
	return nil
}
func (c *fakeCache) ReplacePlatform(_ context.Context, s Session, h string, _ time.Duration) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	old := c.platform[platformKey(s)]
	c.platform[platformKey(s)] = h
	b, e := json.Marshal(s)
	c.values["sid:"+h] = b
	return old, e
}
func (c *fakeCache) CheckPlatform(_ context.Context, s Session, h string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.platform[platformKey(s)] == h, nil
}
func (c *fakeCache) RefreshPlatform(_ context.Context, s Session, h string, _ time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.platform[platformKey(s)] != h {
		return false, nil
	}
	b, e := json.Marshal(s)
	c.values["sid:"+h] = b
	return true, e
}
func TestSessionsTicketsRefreshAndGlobalRevocation(t *testing.T) {
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	now := time.Now().Truncate(time.Second)
	cache := newFakeCache()
	svc := &Service{Store: store, Remote: remote, Cache: cache, Now: func() time.Time { return now }}
	_, e := svc.SaveEmployeeAccess(ctx, EmployeeAccess{EmployeeID: "a", AllowLAN: true, AllowPC: true, AllowMobile: true}, 0, 99)
	must(t, e)
	web, e := svc.Login(ctx, 1, "web")
	must(t, e)
	android, e := svc.Login(ctx, 1, "android")
	must(t, e)
	windows, e := svc.Login(ctx, 1, "windows")
	must(t, e)
	if web.ExpiresAt != now.Add(4*time.Hour).Unix() {
		t.Fatal("session is not 4 hours")
	}
	for _, result := range []LoginResult{web, android, windows} {
		_, e = svc.Validate(ctx, result.SessionToken)
		must(t, e)
	}
	ticket, e := svc.IssueTicket(ctx, 1)
	must(t, e)
	replacement, e := svc.Exchange(ctx, ticket, "web")
	must(t, e)
	if _, e = svc.Exchange(ctx, ticket, "web"); e == nil {
		t.Fatal("one-time ticket replay succeeded")
	}
	if _, e = svc.Validate(ctx, web.SessionToken); e == nil {
		t.Fatal("same-platform old session remained active")
	}
	if _, e = svc.Validate(ctx, android.SessionToken); e != nil {
		t.Fatal("web replacement revoked Android")
	}
	oldExpiry := replacement.ExpiresAt
	now = now.Add(time.Hour)
	fresh, e := svc.Refresh(ctx, replacement.SessionToken)
	must(t, e)
	if fresh.ExpiresAt != oldExpiry {
		t.Fatal("early refresh extended session")
	}
	now = time.Unix(oldExpiry, 0).Add(-9 * time.Minute)
	fresh, e = svc.Refresh(ctx, replacement.SessionToken)
	must(t, e)
	if fresh.ExpiresAt != now.Add(4*time.Hour).Unix() {
		t.Fatal("refresh window did not extend session")
	}
	// Independent host instance can recover Redis sid and SQL epoch after restart.
	restarted := &Service{Store: store, Remote: remote, Cache: cache, Now: svc.Now}
	_, e = restarted.Validate(ctx, fresh.SessionToken)
	must(t, e)
	must(t, restarted.Logout(ctx, android.SessionToken, false))
	if _, e = svc.Validate(ctx, windows.SessionToken); e != nil {
		t.Fatal("single platform logout revoked Windows")
	}
	must(t, restarted.RevokeUID(ctx, 1))
	if _, e = svc.Validate(ctx, fresh.SessionToken); e == nil {
		t.Fatal("global epoch did not invalidate Web")
	}
	if _, e = svc.Validate(ctx, windows.SessionToken); e == nil {
		t.Fatal("global epoch did not invalidate Windows")
	}
}
func TestPasswordChangeAndDeletedEmployeeDenyImmediately(t *testing.T) {
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	svc := &Service{Store: store, Remote: remote, Cache: newFakeCache()}
	login, e := svc.Login(ctx, 1, "web")
	must(t, e)
	_, e = store.DB.ExecContext(ctx, `UPDATE gf_admin SET password='new-password' WHERE id=1`)
	must(t, e)
	if _, e = svc.Validate(ctx, login.SessionToken); e == nil {
		t.Fatal("password change accepted stale fingerprint")
	}
	must(t, store.Reconcile(ctx))
	must(t, store.Drain(ctx, remote))
	_, e = store.DB.ExecContext(ctx, `DELETE FROM biz_hrm_employee WHERE id='a'`)
	must(t, e)
	must(t, store.Reconcile(ctx))
	must(t, store.Drain(ctx, remote))
	if remote.users[UserID("a")].Enabled {
		t.Fatal("deleted employee remained enabled")
	}
}

func TestNullableSourceFieldsAreProjectedWithoutBreakingWholeScan(t *testing.T) {
	ctx, store, remote := fixture(t)
	_, e := store.DB.ExecContext(ctx, `INSERT INTO biz_hrm_employee(id,employee_no,user_id,dept_id,name,status,position) VALUES('nullable-valid','0005',NULL,NULL,'空可选字段','active',NULL),('nullable-pending',NULL,NULL,NULL,NULL,NULL,NULL)`)
	must(t, e)
	provision(t, ctx, store, remote)
	if len(remote.users) != 3 {
		t.Fatalf("wanted3valid projected identities, got%d", len(remote.users))
	}
	p := remote.users[UserID("nullable-valid")]
	if p.DepartmentID != "0" || p.Profile.Position != "" {
		t.Fatal("nullable optional fields not normalized")
	}
	var enabled int
	must(t, store.DB.QueryRowContext(ctx, `SELECT enabled FROM biz_openim_employee WHERE employee_id='nullable-pending'`).Scan(&enabled))
	if enabled != 0 {
		t.Fatal("null required identity fields must remain pending")
	}
}
func TestDiagnosticsClassifyWithoutLoggingSecrets(t *testing.T) {
	value := Diagnostic(fault("source.employees.scan", errors.New("sql: Scan error converting NULL; password=should-never-print")))
	if !strings.Contains(value, "stage=source.employees.scan class=null_projection") || strings.Contains(value, "should-never-print") {
		t.Fatal("unsafe or unhelpful diagnostic", value)
	}
}
func TestConcurrentSamePlatformLoginLeavesOneUnrevokedCurrentSession(t *testing.T) {
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	svc := &Service{Store: store, Remote: remote, Cache: newFakeCache()}
	results := make(chan LoginResult, 2)
	failures := make(chan error, 2)
	var wait sync.WaitGroup
	for i := 0; i < 2; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			v, e := svc.Login(ctx, 1, "web")
			if e != nil {
				failures <- e
				return
			}
			results <- v
		}()
	}
	wait.Wait()
	close(results)
	close(failures)
	for e := range failures {
		t.Fatal(e)
	}
	valid := 0
	for v := range results {
		session, e := svc.Validate(ctx, v.SessionToken)
		if e == nil {
			valid++
			for _, revoked := range remote.revokes {
				if revoked.SessionID == session.ID {
					t.Fatal("current session was revoked by an older concurrent login")
				}
			}
		}
	}
	if valid != 1 {
		t.Fatalf("wanted1current platform session, got%d", valid)
	}
}
