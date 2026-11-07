package imbridge

import (
	"errors"
	"testing"
)

func TestHRMBackofficePublicAccessUsesExactActiveManagementRoles(t *testing.T) {
	ctx, store, remote := fixture(t)
	provision(t, ctx, store, remote)
	for _, statement := range []string{
		`INSERT INTO gf_auth_rule(id,pid,type,routename,permission,status) VALUES(101,1,2,'','biz-suite.hrm.manage',0),(102,1,2,'','biz-suite.im.manage',0),(103,1,2,'','biz-suite.hrm.self',0),(104,1,2,'','biz-suite.hrm.department',0),(105,1,2,'','biz-suite.hrm.read',0),(106,1,2,'','biz-suite.hrm.write',0),(107,1,2,'','biz-suite.hrm.manage',1)`,
		`INSERT INTO gf_auth_role(id,rules,btns,status) VALUES(101,'*','',0),(102,'','101',0),(103,'','102',0),(104,'','103',0),(105,'','104',0),(106,'','105',0),(107,'','106',0),(108,'*','',1),(109,'','107',0)`,
	} {
		_, err := store.DB.ExecContext(ctx, statement)
		must(t, err)
	}
	public := WithClientIP(ctx, "203.0.113.9")
	for _, test := range []struct {
		name    string
		role    int
		allowed bool
	}{
		{"super administrator", 101, true},
		{"HRM manager", 102, true},
		{"IM manager", 103, false},
		{"employee self service", 104, false},
		{"department staff", 105, false},
		{"HRM reader", 106, false},
		{"HRM writer", 107, false},
		{"disabled super role", 108, false},
		{"disabled HRM permission", 109, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := store.DB.ExecContext(ctx, `DELETE FROM gf_auth_role_access WHERE uid=1`)
			must(t, err)
			_, err = store.DB.ExecContext(ctx, `INSERT INTO gf_auth_role_access(uid,role_id) VALUES(1,?)`, test.role)
			must(t, err)
			err = store.CheckHRMBackofficeAccess(public, 1)
			if test.allowed && err != nil {
				t.Fatalf("authorized public administrator was rejected: %v", err)
			}
			if !test.allowed && !errors.Is(err, ErrHRMPublicAdminRequired) {
				t.Fatalf("nonmanager was not rejected: %v", err)
			}
			must(t, store.CheckHRMBackofficeAccess(ctx, 1))
		})
	}
	_, err := store.DB.ExecContext(ctx, `UPDATE gf_auth_role_access SET role_id=101 WHERE uid=1`)
	must(t, err)
	for _, statement := range []string{`UPDATE gf_admin SET status=1 WHERE id=1`, `UPDATE gf_admin SET status=0,deletetime=CURRENT_TIMESTAMP WHERE id=1`} {
		_, err = store.DB.ExecContext(ctx, statement)
		must(t, err)
		if !errors.Is(store.CheckHRMBackofficeAccess(public, 1), ErrHRMPublicAdminRequired) {
			t.Fatal("disabled or deleted public administrator was accepted")
		}
	}
	for _, uid := range []int64{0, 999} {
		if !errors.Is(store.CheckHRMBackofficeAccess(public, uid), ErrHRMPublicAdminRequired) {
			t.Fatal("missing identity was accepted")
		}
	}
	if !errors.Is(store.CheckHRMBackofficeAccess(WithClientIP(ctx, ""), 1), ErrHRMPublicAdminRequired) {
		t.Fatal("unclassified source was accepted")
	}
}

func TestHRMBackofficeGuardPathsExcludeEmployeeIMAndInternalHMAC(t *testing.T) {
	for _, value := range []string{"/admin", "/admin/user/login", "//admin//user/./getMenu", "/plugins/biz-suite/admin", "/plugins/biz-suite/admin/hrm", "/plugins/biz-suite/api/admin/im/manager/openim/certificate", "/plugins/inventory/api/admin/items"} {
		if !hrmBackofficePath(value) {
			t.Errorf("backoffice path escaped guard: %s", value)
		}
	}
	for _, value := range []string{"/common/im-sso/login", "/common/im-sso/refresh", "/internal/im-access/check", "/im/", "/suxinweb/login", "/plugins/biz-suite/api/public/items", "/administrator"} {
		if hrmBackofficePath(value) {
			t.Errorf("nonbackoffice path was guarded: %s", value)
		}
	}
}
