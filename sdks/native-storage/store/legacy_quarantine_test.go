package nativestore

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUnknownOwnedFileQuarantineCanRecoverOnlyExactSentReference(t *testing.T) {
	s, _ := fixture(t)
	oldRoot, quarantine := t.TempDir(), t.TempDir()
	if err := s.ConfigureLegacyQuarantine(quarantine, bytes.Repeat([]byte{0x29}, 32)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLegacyRoots([]string{oldRoot}); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(oldRoot, "unclassified-pending.txt")
	payload := []byte("SYNTHETIC_UNCLASSIFIED_PENDING")
	if err := os.WriteFile(oldPath, payload, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.QuarantineLegacyOwnedFiles(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("unknown plaintext source retained after durable seal", err)
	}
	body, _ := json.Marshal(map[string]string{"filePath": oldPath, "fileName": "职员名单.txt"})
	if _, err := s.db.Exec(`INSERT INTO chat_logs_synthetic(client_msg_id,send_id,content_type,content,send_time)VALUES('legacy-own',?,105,?,?)`, s.account, string(body), time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err := s.MigrateLegacyMedia(); err != nil {
		t.Fatal(err)
	}
	var encoded string
	if err := s.db.QueryRow(`SELECT content FROM chat_logs_synthetic WHERE client_msg_id='legacy-own'`).Scan(&encoded); err != nil {
		t.Fatal(err)
	}
	var rewritten map[string]string
	if err := json.Unmarshal([]byte(encoded), &rewritten); err != nil {
		t.Fatal(err)
	}
	if rewritten["filePath"] == oldPath {
		t.Fatal("owned sent reference not restored from encrypted quarantine")
	}
	source, err := OpenOutgoing(rewritten["filePath"])
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	actual, err := io.ReadAll(source)
	if err != nil || !bytes.Equal(actual, payload) {
		t.Fatal("protected recovered source mismatch", err)
	}
	if entries, err := os.ReadDir(filepath.Join(quarantine, "objects", sourceDigest(oldPath))); err != nil || len(entries) != 2 {
		t.Fatal("unknown preservation copy was destroyed after claim", err)
	}
}

func TestOtherAccountLegacyWALQuarantineThenAuthorizedRAMMigration(t *testing.T) {
	s, _ := fixture(t)
	quarantine := t.TempDir()
	deviceKey := bytes.Repeat([]byte{0x39}, 32)
	if err := s.ConfigureLegacyQuarantine(quarantine, deviceKey); err != nil {
		t.Fatal(err)
	}
	original := t.TempDir()
	legacy := t.TempDir()
	other := "synthetic-legacy-other"
	originalPath := filepath.Join(original, "OpenIM_v3_"+other+".db")
	path := filepath.Join(legacy, filepath.Base(originalPath))
	driver := s.driver + "_test-plain"
	registerMigrationDriver(driver, s)
	plain, err := sql.Open(driver, originalPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = plain.Exec("PRAGMA journal_mode=WAL;PRAGMA wal_autocheckpoint=0;CREATE TABLE synthetic(body TEXT);INSERT INTO synthetic VALUES('SYNTHETIC_WAL_ONLY_CONTENT');"); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		data, e := os.ReadFile(originalPath + suffix)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(path+suffix, data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	_ = plain.Close()
	if err = s.SetLegacyRoots([]string{legacy}); err != nil {
		t.Fatal(err)
	}
	if err = s.QuarantineLegacyDatabases(); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, e := os.Stat(path + suffix); !os.IsNotExist(e) {
			t.Fatal("historical plaintext remained", suffix, e)
		}
	}
	if _, _, err = s.openQuarantinedSource(path, "database", s.account); !os.IsNotExist(err) {
		t.Fatal("other account snapshot exposed to current account", err)
	}
	nextRoot := t.TempDir()
	if err = Configure(other, nextRoot, bytes.Repeat([]byte{0x51}, 32), time.Now().Add(8*time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { Lock(other) })
	next, err := Lookup(other)
	if err != nil {
		t.Fatal(err)
	}
	if err = next.ConfigureLegacyQuarantine(quarantine, deviceKey); err != nil {
		t.Fatal(err)
	}
	if err = next.MigrateLegacy(path); err != nil {
		t.Fatal(err)
	}
	db, err := next.OpenWorkingDatabase()
	if err != nil {
		t.Fatal(err)
	}
	var body string
	if err = db.QueryRow("SELECT body FROM synthetic").Scan(&body); err != nil || body != "SYNTHETIC_WAL_ONLY_CONTENT" {
		t.Fatal("committed WAL message lost", err)
	}
	if err = filepath.WalkDir(nextRoot, func(path string, entry os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if entry.IsDir() {
			return nil
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		if bytes.Contains(data, []byte("SYNTHETIC_WAL_ONLY_CONTENT")) {
			t.Fatal("migration created plaintext file")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestQuarantinedSourceVersionsAndTamperFailClosed(t *testing.T) {
	s, _ := fixture(t)
	quarantine := t.TempDir()
	if err := s.ConfigureLegacyQuarantine(quarantine, bytes.Repeat([]byte{0x41}, 32)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "OpenIM_v3_other.db")
	if err := s.sealLegacySnapshot(path, "other", []byte("synthetic-version-one")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	if err := s.sealLegacySnapshot(path, "other", []byte("synthetic-version-two")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.openQuarantinedSource(path, "database", "other"); err == nil {
		t.Fatal("ambiguous historical versions guessed by timestamp")
	}
	entries, err := os.ReadDir(filepath.Join(quarantine, "objects", sourceDigest(path)))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		t.Fatal("earlier source version overwritten")
	}
	clean := filepath.Join(filepath.Dir(path), "OpenIM_v3_single.db")
	if err := s.sealLegacySnapshot(clean, "single", []byte("synthetic-single")); err != nil {
		t.Fatal(err)
	}
	reader, meta, err := s.openQuarantinedSource(clean, "database", "single")
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	target := filepath.Join(quarantine, "objects", sourceDigest(clean), meta.SHA256+".data")
	encrypted, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	encrypted[len(encrypted)-1] ^= 1
	if err = os.WriteFile(target, encrypted, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.openQuarantinedSource(clean, "database", "single"); err == nil {
		t.Fatal("tampered quarantine accepted")
	}
	Lock(s.account)
	if _, _, err = s.openQuarantinedSource(path, "database", "other"); err == nil || !strings.Contains(err.Error(), "authorization") {
		t.Fatal("locked quarantine accepted", err)
	}
}
