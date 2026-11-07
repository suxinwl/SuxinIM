package admininstall

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestEmptyBootstrapHasOneAdministratorAndNoSampleBusinessRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	config := map[string]string{"type": "sqlite", "path": path}
	if err := BootstrapEmpty(context.Background(), config, "../../../devsource/developer/install", "sysadmin", "a-strong-initial-password"); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"gf_auth_dept", "gf_dictionary_group", "gf_dictionary_data", "gf_home_quickop"} {
		var count int
		if err = db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("sample rows survived %s: %d %v", table, count, err)
		}
	}
	var count int
	if err = db.QueryRow("SELECT COUNT(*) FROM gf_admin WHERE username='sysadmin' AND dept_id=0 AND password<>''").Scan(&count); err != nil || count != 1 {
		t.Fatal("administrator bootstrap failed", err)
	}
	if err = BootstrapEmpty(context.Background(), config, "../../../devsource/developer/install", "another", "a-strong-initial-password"); err == nil {
		t.Fatal("bootstrap overwrote a non-empty database")
	}
}
