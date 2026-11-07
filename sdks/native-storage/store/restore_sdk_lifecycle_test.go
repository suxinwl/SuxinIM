package nativestore

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Unlike fixture(), this uses the production working.db path. Broker pre-open
// and subsequent SDK open must share exactly this handle rather than replace it.
func sdkWorkingFixture(t *testing.T) (*Store, string, *sql.DB) {
	t.Helper()
	account := strings.ReplaceAll(t.Name(), "/", "_")
	root := t.TempDir()
	key := bytes.Repeat([]byte{0x71}, 32)
	if err := Configure(account, root, key, time.Now().Add(8*time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { Lock(account) })
	s, err := Lookup(account)
	if err != nil {
		t.Fatal(err)
	}
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				s.mu.Lock()
				if !s.closed && time.Now().Before(s.expires) {
					s.expires = time.Now().Add(8 * time.Second)
				}
				s.mu.Unlock()
			}
		}
	}()
	t.Cleanup(func() { close(stop); <-done })
	handle, err := s.OpenWorkingDatabase()
	if err != nil {
		t.Fatal(err)
	}
	return s, root, handle
}

func TestSDKWorkingDatabaseOwnershipAndRepeatedClose(t *testing.T) {
	s, root, handle := sdkWorkingFixture(t)
	for i := 0; i < 3; i++ {
		if err := CloseSDKWorkingDatabase(s.account, handle); err != nil {
			t.Fatal("SDK-only close rejected its own working database", err)
		}
		next, err := s.OpenWorkingDatabase()
		if err != nil || next != handle {
			t.Fatal("SDK reopen replaced the vault-owned connection", err)
		}
		if err = handle.Ping(); err != nil {
			t.Fatal("SDK-only close shut the vault database", err)
		}
	}
	if _, err := s.Open(filepath.Join(root, "different.db")); err == nil {
		t.Fatal("a live owner accepted a second database path")
	}
	if _, err := os.Stat(filepath.Join(root, "different.db")); !os.IsNotExist(err) {
		t.Fatal("rejected second path created an orphan database", err)
	}
	if err := CloseSDKWorkingDatabase("foreign-synthetic-owner", handle); !errors.Is(err, ErrLocked) {
		t.Fatal("foreign owner was allowed to close a vault database", err)
	}
	if err := handle.Ping(); err != nil {
		t.Fatal("foreign-owner rejection closed the original database", err)
	}
	// Ownership survives lease expiry. Lookup() intentionally rejects that
	// owner, so using only Lookup to discover handle ownership would close it.
	s.mu.Lock()
	s.expires = time.Now().Add(-time.Second)
	s.mu.Unlock()
	foreignErr := CloseSDKWorkingDatabase("foreign-synthetic-owner", handle)
	s.mu.Lock()
	s.expires = time.Now().Add(8 * time.Second)
	s.mu.Unlock()
	if !errors.Is(foreignErr, ErrLocked) {
		t.Fatal("expired foreign vault handle was mistaken for an ordinary SDK database", foreignErr)
	}
	if err := handle.Ping(); err != nil {
		t.Fatal("foreign close destroyed the expired owner's database", err)
	}
	ordinary, err := sql.Open(s.driver, filepath.Join(t.TempDir(), "unowned.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ordinary.Close() })
	if err = ordinary.Ping(); err != nil {
		t.Fatal(err)
	}
	if err = CloseSDKWorkingDatabase(s.account, ordinary); err != nil {
		t.Fatal("ordinary SDK connection could not close", err)
	}
	if ordinary.Ping() == nil {
		t.Fatal("ordinary non-vault connection was retained")
	}
	Lock(s.account)
	if handle.Ping() == nil {
		t.Fatal("NativeLock retained its owned SQLCipher connection")
	}
	s.mu.Lock()
	zeroed := bytes.Equal(s.master, make([]byte, len(s.master)))
	closed := s.closed
	s.mu.Unlock()
	if !closed || !zeroed {
		t.Fatal("NativeLock did not close and zero the native owner")
	}
}

func TestConcurrentSDKOpenUsesOneHandleAndNativeLockClosesIt(t *testing.T) {
	s, _, handle := sdkWorkingFixture(t)
	results := make(chan *sql.DB, 16)
	failures := make(chan error, 16)
	var group sync.WaitGroup
	for i := 0; i < 16; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			db, err := s.OpenWorkingDatabase()
			results <- db
			failures <- err
		}()
	}
	group.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	for db := range results {
		if db != handle {
			t.Fatal("concurrent pre-open/SDK opens leaked a replacement handle")
		}
	}
	Lock(s.account)
	if handle.Ping() == nil {
		t.Fatal("NativeLock left an opened native handle alive")
	}
	if _, err := s.OpenWorkingDatabase(); !errors.Is(err, ErrLocked) {
		t.Fatal("closed owner reopened its database", err)
	}
}

func TestSDKOnlyCloseThenRestoreKeepsHistoricalFutureCleanup(t *testing.T) {
	s, root, handle := sdkWorkingFixture(t)
	if _, err := handle.Exec(`CREATE TABLE chat_logs_synthetic(client_msg_id TEXT PRIMARY KEY,server_msg_id TEXT DEFAULT '',send_id TEXT DEFAULT '',recv_id TEXT DEFAULT '',sender_platform_id INTEGER DEFAULT 2,sender_nick_name TEXT DEFAULT '',sender_face_url TEXT DEFAULT '',session_type INTEGER DEFAULT 1,msg_from INTEGER DEFAULT 100,content_type INTEGER DEFAULT 101,content TEXT DEFAULT '',is_read INTEGER DEFAULT 1,status INTEGER DEFAULT 2,seq INTEGER DEFAULT 1,send_time INTEGER,create_time INTEGER DEFAULT 0,attached_info TEXT DEFAULT '',ex TEXT DEFAULT '',local_ex TEXT DEFAULT '')`); err != nil {
		t.Fatal(err)
	}
	if err := s.InstallConversation("chat_logs_synthetic", "synthetic"); err != nil {
		t.Fatal(err)
	}
	s.MarkViewsReady(true)
	cutoff := time.Now().Add(24 * time.Hour).UnixMilli()
	if _, err := handle.Exec(`INSERT INTO chat_logs_synthetic(client_msg_id,content,send_time) VALUES('old','SYNTHETIC_OLDER_BACKUP',?)`, cutoff-1); err != nil {
		t.Fatal(err)
	}
	backup, err := s.ExportBackup("synthetic lifecycle recovery password")
	if err != nil {
		t.Fatal(err)
	}
	// This is authenticated historical local state from a previous release,
	// not a new UI clear. Restore must retain its exact old upper bound.
	if _, err = handle.Exec(`INSERT INTO puty_local_tombstones(conversation,from_time,to_time) VALUES('synthetic',0,?)`, cutoff); err != nil {
		t.Fatal(err)
	}
	if err = CloseSDKWorkingDatabase(s.account, handle); err != nil {
		t.Fatal(err)
	}
	prepared, err := s.PrepareRestore(backup, "synthetic lifecycle recovery password")
	if err != nil {
		t.Fatal("restore could not read current privacy state after SDK-only logout", err)
	}
	defer prepared.Abort()
	key := prepared.MasterKey()
	defer clear(key)
	envelope := bytes.Repeat([]byte{0x42}, 64)
	if err = prepared.PrepareCommit(envelope); err != nil {
		t.Fatal("restore preparation failed after SDK-only close", err)
	}
	if err = prepared.Commit(envelope); err != nil {
		t.Fatal(err)
	}
	account := s.account
	Lock(account)
	if err = Configure(account, root, key, time.Now().Add(8*time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	restored, err := Lookup(account)
	if err != nil {
		t.Fatal(err)
	}
	db, err := restored.OpenWorkingDatabase()
	if err != nil {
		t.Fatal(err)
	}
	var upper int64
	if err = db.QueryRow(`SELECT to_time FROM puty_local_tombstones WHERE conversation='synthetic' AND from_time=0`).Scan(&upper); err != nil || upper != cutoff {
		t.Fatal("restore shortened a previously verified historical tombstone", err)
	}
	if !restored.Suppressed("synthetic", "old-resync", cutoff-1) {
		t.Fatal("older backup restored a historically cleared message")
	}
	if restored.Suppressed("synthetic", "later-new-id", cutoff+1) {
		t.Fatal("historical range expanded into a later new message")
	}
}
