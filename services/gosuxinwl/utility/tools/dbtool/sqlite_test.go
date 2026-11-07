package dbtool

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestSQLiteDumpRoundTrip(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec("CREATE TABLE `gf_demo` (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT, payload BLOB, amount REAL, missing TEXT); CREATE INDEX `gf_demo_name` ON `gf_demo` (name);")
	if err != nil {
		t.Fatal(err)
	}
	name := "生产部 O'Reilly\\line\n下一行"
	if _, err = db.Exec("INSERT INTO gf_demo(name,payload,amount) VALUES (?,?,?)", name, []byte{0, 1, 255}, 1.25); err != nil {
		t.Fatal(err)
	}
	var dump bytes.Buffer
	if err = dumpSQLite(db, &dump, nil, "gf_", true, true); err != nil {
		t.Fatal(err)
	}
	restored, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "restored.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if _, err = restored.Exec(dump.String()); err != nil {
		t.Fatalf("restore: %v\n%s", err, dump.String())
	}
	var got string
	var blob []byte
	var amount float64
	var missing sql.NullString
	if err = restored.QueryRow("SELECT name,payload,amount,missing FROM demo").Scan(&got, &blob, &amount, &missing); err != nil {
		t.Fatal(err)
	}
	if got != name || !bytes.Equal(blob, []byte{0, 1, 255}) || amount != 1.25 || missing.Valid {
		t.Fatal("dump changed values")
	}
	var indexes int
	if err = restored.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND tbl_name='demo'").Scan(&indexes); err != nil || indexes != 1 {
		t.Fatalf("index lost: %d %v", indexes, err)
	}
}
