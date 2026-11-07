package accountsecurity

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

const testPassword = "0123456789abcdef0123456789abcdef"
const nextPassword = "abcdef0123456789abcdef0123456789"

func fixture(t *testing.T) (context.Context, *Store) {
	t.Helper()
	ctx := context.Background()
	db, e := gdb.New(gdb.ConfigNode{Type: "sqlite", Name: filepath.Join(t.TempDir(), "security.db")})
	if e != nil {
		t.Fatal(e)
	}
	raw, e := db.Master()
	if e != nil {
		t.Fatal(e)
	}
	raw.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close(ctx) })
	for _, q := range []string{
		`CREATE TABLE gf_admin(id INTEGER PRIMARY KEY,username TEXT,password TEXT,salt TEXT,status INTEGER DEFAULT 0,deletetime DATETIME,lock_time DATETIME,login_attempts INTEGER DEFAULT 0,pwd_reset_time DATETIME,name TEXT,mobile TEXT,tel TEXT,email TEXT,avatar TEXT,remark TEXT,dept_id INTEGER,account_id INTEGER,createtime DATETIME)`,
		`CREATE TABLE gf_auth_role(id BIGINT PRIMARY KEY,pid BIGINT DEFAULT 0,business_id BIGINT DEFAULT 0,rules TEXT,status INTEGER DEFAULT 0)`,
		`CREATE TABLE gf_auth_role_access(uid BIGINT,role_id BIGINT)`,
		`CREATE TABLE biz_openim_employee(uid BIGINT PRIMARY KEY,openim_user_id TEXT,employee_id TEXT)`,
		`CREATE TABLE biz_hrm_employee(id TEXT PRIMARY KEY,user_id BIGINT)`,
		`CREATE TABLE biz_openim_employee_access(employee_id TEXT PRIMARY KEY,allow_lan INTEGER,allow_pc INTEGER,allow_mobile INTEGER,allow_internet INTEGER,allow_internet_download INTEGER)`,
		`INSERT INTO gf_auth_role(id,rules) VALUES(1,'*'),(2,''),(3,'')`,
		`INSERT INTO gf_auth_role_access VALUES(1,1)`,
		`INSERT INTO biz_openim_employee VALUES(1,'emp_synthetic','synthetic')`,
		`INSERT INTO biz_hrm_employee VALUES('synthetic',1)`,
	} {
		if _, e = raw.ExecContext(ctx, q); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = raw.ExecContext(ctx, `INSERT INTO gf_admin(id,username,password,salt) VALUES(1,'synthetic',?,'fixture')`, passwordHash(testPassword, "fixture")); e != nil {
		t.Fatal(e)
	}
	s, e := New(raw, "gf_")
	if e != nil {
		t.Fatal(e)
	}
	s.IMEnabled = true
	s.Now = func() time.Time { return time.Unix(1900000000, 0) }
	if e = s.Ensure(ctx); e != nil {
		t.Fatal(e)
	}
	return ctx, s
}
func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func count(t *testing.T, s *Store, q string, args ...any) int {
	t.Helper()
	var n int
	must(t, s.DB.QueryRow(q, args...).Scan(&n))
	return n
}

func TestRollingWindowThresholdExpiryAndAtomicRevocation(t *testing.T) {
	ctx, s := fixture(t)
	wrong := "00000000000000000000000000000000"
	for i := 0; i < 2; i++ {
		if !errors.Is(s.Authenticate(ctx, 1, wrong), ErrDenied) {
			t.Fatal("failure accepted")
		}
	}
	s.Now = func() time.Time { return time.Unix(1900000601, 0) }
	for i := 0; i < 4; i++ {
		_ = s.Authenticate(ctx, 1, wrong)
	}
	v, e := s.State(ctx, 1)
	must(t, e)
	if v.Locked || v.RecentFailures != 4 {
		t.Fatal("old failures counted in rolling window")
	}
	_ = s.Authenticate(ctx, 1, wrong)
	v, e = s.State(ctx, 1)
	must(t, e)
	if !v.Locked || v.LockedUntil != 1900002401 || v.SecurityVersion != 2 {
		t.Fatal("fifth failure did not lock atomically")
	}
	if count(t, s, `SELECT COUNT(*) FROM biz_openim_outbox WHERE kind='revoke' AND status='pending'`) != 1 {
		t.Fatal("missing durable revoke")
	}
	if !errors.Is(s.Authenticate(ctx, 1, testPassword), ErrDenied) {
		t.Fatal("correct password bypassed lock")
	}
	s.Now = func() time.Time { return time.Unix(1900002402, 0) }
	must(t, s.Authenticate(ctx, 1, testPassword))
	v, e = s.State(ctx, 1)
	must(t, e)
	if v.Locked || v.RecentFailures != 0 {
		t.Fatal("lock did not expire")
	}
}
func TestConcurrentFailuresCannotSkipOrDuplicateLock(t *testing.T) {
	ctx, s := fixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Go(func() { _ = s.Authenticate(ctx, 1, "00000000000000000000000000000000") })
	}
	wg.Wait()
	v, e := s.State(ctx, 1)
	must(t, e)
	if !v.Locked || v.SecurityVersion != 2 || count(t, s, `SELECT COUNT(*) FROM biz_openim_outbox`) != 1 {
		t.Fatal("concurrent threshold was not atomic")
	}
	must(t, s.Unlock(ctx, 1, 1))
	v, e = s.State(ctx, 1)
	must(t, e)
	if v.Locked || v.RecentFailures != 0 || v.SecurityVersion != 2 {
		t.Fatal("unlock revived security generation")
	}
	must(t, s.Authenticate(ctx, 1, testPassword))
}
func TestResetForcedTicketSingleUseAndNoFullSession(t *testing.T) {
	ctx, s := fixture(t)
	must(t, s.ChangePassword(ctx, 1, 1, "", nextPassword, "", true))
	v, e := s.State(ctx, 1)
	must(t, e)
	if !v.MustChange || v.SecurityVersion != 2 {
		t.Fatal("reset not forced")
	}
	if !errors.Is(s.CheckActive(ctx, 1), ErrPasswordChangeRequired) {
		t.Fatal("reset received ordinary access")
	}
	must(t, s.Authenticate(ctx, 1, nextPassword))
	c, e := s.Challenge(ctx, 1)
	must(t, e)
	raw, _ := json.Marshal(c)
	for _, secret := range []string{"imToken", "chatToken", "sessionToken", testPassword, nextPassword} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("challenge contained credential or full session")
		}
	}
	if c.ExpiresAt != 1900000300 || len(c.ChangeTicket) != 64 {
		t.Fatal("wrong challenge lifetime")
	}
	_, e = s.TicketStatus(ctx, c.ChangeTicket)
	must(t, e)
	must(t, s.ChangePassword(ctx, 0, 0, "", testPassword, c.ChangeTicket, false))
	if _, e = s.TicketStatus(ctx, c.ChangeTicket); !errors.Is(e, ErrDenied) {
		t.Fatal("challenge replay")
	}
	if e = s.ChangePassword(ctx, 0, 0, "", nextPassword, c.ChangeTicket, false); !errors.Is(e, ErrDenied) {
		t.Fatal("change replay")
	}
	v, e = s.State(ctx, 1)
	must(t, e)
	if v.MustChange || v.SecurityVersion != 3 {
		t.Fatal("change did not revoke prior sessions")
	}
}
func TestTicketExpiresAndResetSupersedesOlderTicket(t *testing.T) {
	ctx, s := fixture(t)
	must(t, s.ChangePassword(ctx, 1, 1, "", nextPassword, "", true))
	c, e := s.Challenge(ctx, 1)
	must(t, e)
	s.Now = func() time.Time { return time.Unix(1900000301, 0) }
	if _, e = s.TicketStatus(ctx, c.ChangeTicket); !errors.Is(e, ErrDenied) {
		t.Fatal("expired ticket accepted")
	}
	c, e = s.Challenge(ctx, 1)
	must(t, e)
	must(t, s.ChangePassword(ctx, 1, 1, "", testPassword, "", true))
	if _, e = s.TicketStatus(ctx, c.ChangeTicket); !errors.Is(e, ErrDenied) {
		t.Fatal("reset did not supersede ticket")
	}
}
func TestPasswordAndEpochRollbackWhenOutboxCannotCommit(t *testing.T) {
	ctx, s := fixture(t)
	must(t, s.Authenticate(ctx, 1, testPassword))
	must(t, exec(s, `CREATE TRIGGER reject_revoke BEFORE INSERT ON biz_openim_outbox BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`))
	if e := s.ChangePassword(ctx, 1, 1, testPassword, nextPassword, "", false); e == nil {
		t.Fatal("partial password commit accepted")
	}
	must(t, s.Authenticate(ctx, 1, testPassword))
	v, e := s.State(ctx, 1)
	must(t, e)
	if v.SecurityVersion != 1 || v.MustChange {
		t.Fatal("security version changed on rollback")
	}
}
func exec(s *Store, q string, args ...any) error { _, e := s.DB.Exec(q, args...); return e }
func TestAllHRMRequestsRejectChangedPasswordLockedAndInactive(t *testing.T) {
	ctx, s := fixture(t)
	fp := Hash(passwordHash(testPassword, "fixture") + ":fixture")
	must(t, s.CheckSession(ctx, 1, 1, fp))
	must(t, s.ChangePassword(ctx, 1, 1, testPassword, nextPassword, "", false))
	if e := s.CheckSession(ctx, 1, 1, fp); !errors.Is(e, ErrDenied) {
		t.Fatal("old host session survived change")
	}
	var password, salt string
	must(t, s.DB.QueryRow(`SELECT password,salt FROM gf_admin WHERE id=1`).Scan(&password, &salt))
	must(t, s.CheckSession(ctx, 1, 2, Hash(password+":"+salt)))
	for i := 0; i < 5; i++ {
		_ = s.Authenticate(ctx, 1, testPassword)
	}
	if e := s.CheckSession(ctx, 1, 2, Hash(password+":"+salt)); !errors.Is(e, ErrDenied) {
		t.Fatal("old session survived lock")
	}
	must(t, s.Unlock(ctx, 1, 1))
	must(t, exec(s, `UPDATE gf_admin SET status=1 WHERE id=1`))
	if e := s.CheckSession(ctx, 1, 3, Hash(password+":"+salt)); !errors.Is(e, ErrDenied) {
		t.Fatal("disabled host session accepted")
	}
}
func TestRoleUnionPersonalPriorityAndAllTwentyCells(t *testing.T) {
	ctx, s := fixture(t)
	must(t, exec(s, `DELETE FROM gf_auth_role_access`))
	must(t, exec(s, `INSERT INTO gf_auth_role_access VALUES(1,2),(1,3)`))
	a := DefaultMatrix("deny")
	b := DefaultMatrix("deny")
	a["lan"]["android"] = Cell{"allow", "deny"}
	b["internet"]["pc"] = Cell{"allow", "allow"}
	_, e := s.SavePolicy(ctx, Policy{Subject: "role", SubjectID: 2, Matrix: a}, 1)
	must(t, e)
	_, e = s.SavePolicy(ctx, Policy{Subject: "role", SubjectID: 3, Matrix: b}, 1)
	must(t, e)
	for _, z := range Zones {
		for _, client := range Clients {
			v, e := s.Resolve(ctx, 1, z, client)
			must(t, e)
			wantLogin := z == "lan" && client == "android" || z == "internet" && client == "pc"
			wantSave := z == "internet" && client == "pc"
			if v.Login != wantLogin || v.Preview != wantLogin || v.Save != wantSave {
				t.Fatal("role union cell mismatch")
			}
		}
	}
	p := DefaultMatrix("inherit")
	p["lan"]["ios"] = Cell{"allow", "allow"}
	p["internet"]["pc"] = Cell{"deny", "allow"}
	saved, e := s.SavePolicy(ctx, Policy{Subject: "person", SubjectID: 1, Matrix: p}, 1)
	must(t, e)
	v, e := s.Resolve(ctx, 1, "lan", "ios")
	must(t, e)
	if !v.Login || !v.Save || v.Sources.Login != "person" {
		t.Fatal("personal allow did not override role deny")
	}
	v, e = s.Resolve(ctx, 1, "internet", "pc")
	must(t, e)
	if v.Login || v.Save || v.Preview {
		t.Fatal("personal deny did not override role allow")
	}
	saved.Revision--
	if _, e = s.SavePolicy(ctx, saved, 1); !errors.Is(e, ErrConflict) {
		t.Fatal("stale update accepted")
	}
	must(t, exec(s, `UPDATE gf_auth_role SET status=1 WHERE id=2`))
	v, e = s.Resolve(ctx, 1, "lan", "android")
	must(t, e)
	if v.Login {
		t.Fatal("disabled role still granted")
	}
}
func TestLegacyMigrationPreservesFlagsAndNeverOverwritesPersonalEdit(t *testing.T) {
	ctx, s := fixture(t)
	must(t, exec(s, `INSERT INTO biz_openim_employee_access VALUES('synthetic',1,1,0,1,0)`))
	must(t, s.MigrateLegacyPolicies(ctx))
	p, e := s.ReadPolicy(ctx, "person", 1)
	must(t, e)
	if p.Revision != 1 || p.Matrix["lan"]["pc"].Save != "allow" || p.Matrix["internet"]["pc"].Login != "allow" || p.Matrix["internet"]["pc"].Save != "deny" || p.Matrix["lan"]["web_mobile"].Login != "deny" {
		t.Fatal("legacy policy drift")
	}
	p.Matrix = DefaultMatrix("inherit")
	p, e = s.SavePolicy(ctx, p, 1)
	must(t, e)
	must(t, s.MigrateLegacyPolicies(ctx))
	after, e := s.ReadPolicy(ctx, "person", 1)
	must(t, e)
	if after.Revision != p.Revision || after.Matrix["lan"]["pc"].Save != "inherit" {
		t.Fatal("migration overwrote admin choice")
	}
}
func TestMalformedMatrixAndLogicalClientFailClosed(t *testing.T) {
	ctx, s := fixture(t)
	m := DefaultMatrix("inherit")
	if ValidateMatrix(m, false) == nil {
		t.Fatal("role inherit accepted")
	}
	m["lan"]["other"] = Cell{"allow", "allow"}
	if ValidateMatrix(m, true) == nil {
		t.Fatal("extra client accepted")
	}
	if _, e := s.Resolve(ctx, 1, "invalid", "pc"); !errors.Is(e, ErrDenied) {
		t.Fatal("invalid zone accepted")
	}
	if _, e := s.Resolve(ctx, 1, "lan", "windows"); !errors.Is(e, ErrDenied) {
		t.Fatal("SDK field accepted as logical client")
	}
}

func TestUnconfiguredRoleDoesNotSilentlyWidenConfiguredUnion(t *testing.T) {
	ctx, s := fixture(t)
	must(t, exec(s, `DELETE FROM gf_auth_role_access`))
	must(t, exec(s, `INSERT INTO gf_auth_role_access VALUES(1,2),(1,3)`))
	v, e := s.Resolve(ctx, 1, "lan", "pc")
	must(t, e)
	if !v.Login || v.Sources.Login != "default" {
		t.Fatal("unconfigured compatibility default changed")
	}
	_, e = s.SavePolicy(ctx, Policy{Subject: "role", SubjectID: 2, Matrix: DefaultMatrix("deny")}, 1)
	must(t, e)
	v, e = s.Resolve(ctx, 1, "lan", "pc")
	must(t, e)
	if v.Login || v.Sources.Login != "roles" {
		t.Fatal("unconfigured role overrode configured restriction")
	}
	p, e := s.ReadPolicy(ctx, "role", 3)
	must(t, e)
	if p.Revision != 0 {
		t.Fatal("unconfigured editor state was hidden")
	}
}

func TestManagedTargetsRequireDescendantAndMatchingBusiness(t *testing.T) {
	ctx, s := fixture(t)
	for _, q := range []string{
		`INSERT INTO gf_admin(id,username,status) VALUES(2,'manager',0),(3,'child',0),(4,'foreign',0)`,
		`UPDATE gf_auth_role SET pid=1,business_id=1 WHERE id=2`,
		`UPDATE gf_auth_role SET pid=2,business_id=1 WHERE id=3`,
		`INSERT INTO gf_auth_role(id,pid,business_id,rules,status) VALUES(4,2,2,'',0)`,
		`INSERT INTO gf_auth_role_access VALUES(2,2),(3,3),(4,4)`,
	} {
		must(t, exec(s, q))
	}
	must(t, s.AssertManagedTarget(ctx, 2, 3, false))
	must(t, s.AssertManagedTarget(ctx, 2, 3, true))
	for _, id := range []int64{1, 2, 4, 999} {
		if e := s.AssertManagedTarget(ctx, 2, id, false); !errors.Is(e, ErrDenied) {
			t.Fatal("delegated account target escaped its scope")
		}
	}
	if e := s.AssertManagedTarget(ctx, 2, 4, true); !errors.Is(e, ErrDenied) {
		t.Fatal("cross-business role granted")
	}
	must(t, s.AssertManagedTarget(ctx, 1, 4, false))
}

func TestLegacyFutureLockSurvivesMigrationAndExplicitUnlock(t *testing.T) {
	ctx, s := fixture(t)
	until := s.now().Add(20 * time.Minute)
	must(t, exec(s, `UPDATE gf_admin SET lock_time=?,login_attempts=3 WHERE id=1`, until))
	must(t, s.MigrateLegacyLocks(ctx))
	v, e := s.State(ctx, 1)
	must(t, e)
	if !v.Locked || v.LockedUntil != until.Unix() {
		t.Fatal("legacy lock was lost")
	}
	if e = s.Authenticate(ctx, 1, testPassword); !errors.Is(e, ErrDenied) {
		t.Fatal("migrated lock bypassed")
	}
	must(t, s.Unlock(ctx, 1, 1))
	must(t, s.MigrateLegacyLocks(ctx))
	must(t, s.Authenticate(ctx, 1, testPassword))
}
