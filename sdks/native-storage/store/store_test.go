package nativestore

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) (*Store, string) {
	t.Helper()
	account := strings.ReplaceAll(t.Name(), "/", "_")
	root := t.TempDir()
	if err := Configure(account, root, bytes.Repeat([]byte{0x71}, 32), time.Now().Add(8*time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { Lock(account) })
	s, err := Lookup(account)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the production broker's periodic successful online renewal while
	// this synthetic test actor remains live. Never renew an expired/closed
	// store, so expiry and explicit-lock negative cases remain fail closed.
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				s.mu.Lock()
				if !s.closed && time.Now().Before(s.expires) {
					s.expires = time.Now().Add(8 * time.Second)
				}
				s.mu.Unlock()
			}
		}
	}()
	t.Cleanup(func() { close(stop); <-done })
	if _, err = s.Open(filepath.Join(root, "synthetic.db")); err != nil {
		t.Fatal(err)
	}
	_, err = s.db.Exec(`CREATE TABLE chat_logs_synthetic(client_msg_id TEXT PRIMARY KEY,server_msg_id TEXT DEFAULT '',send_id TEXT DEFAULT '',recv_id TEXT DEFAULT '',sender_platform_id INTEGER DEFAULT 2,sender_nick_name TEXT DEFAULT '',sender_face_url TEXT DEFAULT '',session_type INTEGER DEFAULT 1,msg_from INTEGER DEFAULT 100,content_type INTEGER DEFAULT 101,content TEXT DEFAULT '',is_read INTEGER DEFAULT 1,status INTEGER DEFAULT 2,seq INTEGER DEFAULT 1,send_time INTEGER,create_time INTEGER DEFAULT 0,attached_info TEXT DEFAULT '',ex TEXT DEFAULT '',local_ex TEXT DEFAULT '')`)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.InstallConversation("chat_logs_synthetic", "synthetic"); err != nil {
		t.Fatal(err)
	}
	s.MarkViewsReady(true)
	return s, root
}

func insert(t *testing.T, s *Store, id, body string, at int64) {
	t.Helper()
	if _, err := s.db.Exec(`INSERT INTO chat_logs_synthetic(client_msg_id,content,send_time) VALUES(?,?,?)`, id, body, at); err != nil {
		t.Fatal(err)
	}
}
func count(t *testing.T, s *Store, query string, args ...any) int {
	t.Helper()
	var result int
	if err := s.db.QueryRow(query, args...).Scan(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestTransactionOutboxRollback(t *testing.T) {
	s, _ := fixture(t)
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO chat_logs_synthetic(client_msg_id,content,send_time) VALUES('rollback','SYNTHETIC_ROLLBACK',1)`); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()
	if count(t, s, `SELECT count(*) FROM puty_archive_outbox`) != 0 {
		t.Fatal("rolled-back message produced archive")
	}
}

func TestArchiveCipherPageAndCrashReplay(t *testing.T) {
	s, root := fixture(t)
	at := time.Now().UnixMilli()
	insert(t, s, "one", "SYNTHETIC_ONLY_BODY", at)
	if count(t, s, `SELECT count(*) FROM puty_archive_outbox`) != 1 {
		t.Fatal("message not atomically queued")
	}
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	page, err := s.Page("synthetic", at+1, 20)
	if err != nil || len(page) != 1 || !bytes.Contains(page[0], []byte("SYNTHETIC_ONLY_BODY")) {
		t.Fatal("authenticated page failed", err)
	}
	if err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err == nil && bytes.Contains(body, []byte("SYNTHETIC_ONLY_BODY")) {
			t.Fatalf("plaintext persisted in %s", entry.Name())
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// Simulate crash after durable file rename but before encrypted DB ACK.
	if _, err = s.db.Exec(`INSERT INTO puty_archive_outbox(conversation,message_id,send_time,operation,payload) VALUES('synthetic','one',?,1,?)`, at, string(page[0])); err != nil {
		t.Fatal(err)
	}
	if err = s.Flush(100); err != nil {
		t.Fatal(err)
	}
	if count(t, s, `SELECT count(*) FROM puty_archive_index`) != 1 {
		t.Fatal("crash replay duplicated logical message")
	}
}

func TestDeletionRedactsPendingAndArchive(t *testing.T) {
	s, _ := fixture(t)
	at := time.Now().UnixMilli()
	insert(t, s, "pending", "SYNTHETIC_SECRET_PENDING", at)
	if _, err := s.db.Exec(`UPDATE chat_logs_synthetic SET status=4 WHERE client_msg_id='pending'`); err != nil {
		t.Fatal(err)
	}
	if count(t, s, `SELECT count(*) FROM puty_archive_outbox WHERE payload<>''`) != 0 {
		t.Fatal("deleted payload remained in outbox")
	}
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	insert(t, s, "archived", "SYNTHETIC_SECRET_ARCHIVED", at)
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	var path string
	if err := s.db.QueryRow(`SELECT path FROM puty_archive_index WHERE message_id='archived'`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM chat_logs_synthetic WHERE client_msg_id='archived'`); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.root, "archive", path)); !os.IsNotExist(err) {
		t.Fatal("deleted archive record remained")
	}
	if count(t, s, `SELECT count(*) FROM puty_archive_index`) != 0 {
		t.Fatal("deleted index remained")
	}
}

func TestLocalTombstoneRejectsResync(t *testing.T) {
	s, _ := fixture(t)
	at := time.Now().UnixMilli()
	insert(t, s, "clear", "SYNTHETIC_OLD", at)
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearRange("chat_logs_synthetic", "synthetic", at, at); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE chat_logs_synthetic SET content='SYNTHETIC_RESYNC',status=2 WHERE client_msg_id='clear'`); err != nil {
		t.Fatal(err)
	}
	var body string
	var status, seq int
	if err := s.db.QueryRow(`SELECT content,status,seq FROM chat_logs_synthetic WHERE client_msg_id='clear'`).Scan(&body, &status, &seq); err != nil {
		t.Fatal(err)
	}
	if body != "" || status != 4 || seq != 1 {
		t.Fatal("resync revived locally cleared payload")
	}
	if count(t, s, `SELECT count(*) FROM puty_archive_outbox WHERE payload<>''`) != 0 {
		t.Fatal("resync wrote cleared payload into outbox")
	}
	page, err := s.Page("synthetic", at+1, 10)
	if err != nil || len(page) != 0 {
		t.Fatal("tombstone failed archive filter", err)
	}
}

func TestHotEvictionPreservesArchiveUnreadPendingAndSeq(t *testing.T) {
	s, _ := fixture(t)
	at := time.Now().AddDate(0, 0, -100).UnixMilli()
	insert(t, s, "read", "SYNTHETIC_READ", at)
	insert(t, s, "unread", "SYNTHETIC_UNREAD", at)
	insert(t, s, "pending", "SYNTHETIC_PENDING", at)
	if _, err := s.db.Exec(`UPDATE chat_logs_synthetic SET is_read=0 WHERE client_msg_id='unread';UPDATE chat_logs_synthetic SET status=1,seq=0 WHERE client_msg_id='pending'`); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	if err := s.EvictHot("chat_logs_synthetic", "synthetic", time.Now().AddDate(0, 0, -90).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if count(t, s, `SELECT count(*) FROM chat_logs_synthetic WHERE client_msg_id='read' AND content='' AND seq=1 AND status=2`) != 1 {
		t.Fatal("hot eviction lost sequence")
	}
	if count(t, s, `SELECT count(*) FROM chat_logs_synthetic WHERE client_msg_id IN ('unread','pending') AND content<>''`) != 2 {
		t.Fatal("unread/pending evicted")
	}
	if count(t, s, `SELECT count(*) FROM puty_archive_outbox`) != 0 || count(t, s, `SELECT count(*) FROM puty_local_deleted`) != 0 {
		t.Fatal("hot eviction became permanent deletion")
	}
	page, err := s.Page("synthetic", time.Now().UnixMilli(), 10)
	if err != nil || len(page) != 3 {
		t.Fatal("hot archive unavailable", err)
	}
}

func TestExpiredLeaseDeniesSQLiteAndArchive(t *testing.T) {
	s, _ := fixture(t)
	insert(t, s, "test", "SYNTHETIC", time.Now().UnixMilli())
	s.mu.Lock()
	s.expires = time.Now().Add(-time.Second)
	s.mu.Unlock()
	if _, err := s.Page("synthetic", time.Now().UnixMilli()+1, 10); err != ErrLocked {
		t.Fatal("offline archive unlocked")
	}
	if _, err := s.db.Exec(`UPDATE chat_logs_synthetic SET content='OFFLINE'`); err == nil {
		t.Fatal("expired lease permitted SQL")
	}
}

func TestArchiveTamperingAndBoundedPage(t *testing.T) {
	s, _ := fixture(t)
	at := time.Now().UnixMilli()
	insert(t, s, "one", "SYNTHETIC", at)
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	var path string
	if err := s.db.QueryRow(`SELECT path FROM puty_archive_index`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(s.root, "archive", path)
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 1
	if err = os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Page("synthetic", at+1, 10); err == nil {
		t.Fatal("tampered record accepted")
	}
	if _, err = s.Page("synthetic", at+1, 1000); err == nil {
		t.Fatal("unbounded page accepted")
	}
	if !json.Valid([]byte(`{"synthetic":true}`)) {
		t.Fatal("unreachable")
	}
}

func TestPreparedStatementAndRowsExpire(t *testing.T) {
	s, _ := fixture(t)
	insert(t, s, "one", "SYNTHETIC", 1)
	statement, err := s.db.Prepare(`SELECT content FROM chat_logs_synthetic`)
	if err != nil {
		t.Fatal(err)
	}
	defer statement.Close()
	rows, err := statement.Query()
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.expires = time.Now().Add(-time.Second)
	s.mu.Unlock()
	if rows.Next() {
		t.Fatal("already-open rows read after authorization expiry")
	}
	if rows.Err() == nil {
		t.Fatal("expired rows did not return failure")
	}
	_ = rows.Close()
	if _, err = statement.Query(); err == nil {
		t.Fatal("prepared statement bypassed expiry")
	}
}

func TestEphemeralAndRevokeNeverRetainOriginalPayload(t *testing.T) {
	s, _ := fixture(t)
	insert(t, s, "burn", "SYNTHETIC_BURN", 1)
	if _, err := s.db.Exec(`UPDATE chat_logs_synthetic SET attached_info='{"isPrivateChat":true}' WHERE client_msg_id='burn'`); err != nil {
		t.Fatal(err)
	}
	if count(t, s, `SELECT count(*) FROM puty_archive_outbox WHERE payload<>''`) != 0 {
		t.Fatal("burn message queued permanently")
	}
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	if count(t, s, `SELECT count(*) FROM chat_logs_synthetic WHERE client_msg_id='burn' AND content<>'' AND status=2`) != 1 {
		t.Fatal("unread burn message prematurely hidden")
	}
	insert(t, s, "revoke", "SYNTHETIC_REVOKED_ORIGINAL", 2)
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE chat_logs_synthetic SET content_type=2101,content='synthetic revoke notice' WHERE client_msg_id='revoke'`); err != nil {
		t.Fatal(err)
	}
	if count(t, s, `SELECT count(*) FROM puty_archive_outbox WHERE payload<>''`) != 0 {
		t.Fatal("revoke payload queued")
	}
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	if count(t, s, `SELECT count(*) FROM puty_archive_index`) != 0 {
		t.Fatal("revoke/burn retained archive")
	}
}

func TestActualAttachmentNamesRetryAndCrossReference(t *testing.T) {
	s, root := fixture(t)
	at := time.Now().UnixMilli()
	insert(t, s, "first", "SYNTHETIC_FILE", at)
	insert(t, s, "second", "SYNTHETIC_REF", at+10)
	payload := bytes.Repeat([]byte("SYNTHETIC_ATTACHMENT_"), 100)
	write := func(task, object, message string) string {
		path, err := s.BeginAttachment(task, object, "职员名单.xlsx", "synthetic", message)
		if err != nil {
			t.Fatal(err)
		}
		if !s.AttachmentComplete(task) {
			if err = s.WriteAttachment(task, payload[:100]); err != nil {
				t.Fatal(err)
			}
			s.AbortAttachment(task)
			retry, err := s.BeginAttachment(task, object, "职员名单.xlsx", "synthetic", message)
			if err != nil || retry != path {
				t.Fatal("retry changed allocated name", err)
			}
			if err = s.WriteAttachment(task, payload); err != nil {
				t.Fatal(err)
			}
			if err = s.FinishAttachment(task, int64(len(payload))); err != nil {
				t.Fatal(err)
			}
		}
		return path
	}
	first := write("task1", "object1", "first")
	second := write("task2", "object2", "second")
	if filepath.Base(first) != "职员名单.xlsx" || filepath.Base(second) != "职员名单（1）.xlsx" {
		t.Fatal("actual names not preserved")
	}
	if !strings.Contains(first, filepath.Join(time.Now().Format("200601"), "attachments")) {
		t.Fatal("first-download month missing")
	}
	disk, err := os.ReadFile(first)
	if err != nil || bytes.Contains(disk, payload) || !bytes.HasPrefix(disk, []byte("PUTYLS1\n")) {
		t.Fatal("actual attachment not encrypted", err)
	}
	restored, err := s.ReadAttachment("task1", 1<<20)
	if err != nil || !bytes.Equal(restored, payload) {
		t.Fatal("encrypted attachment cannot be read", err)
	}
	if _, err = s.BeginAttachment("task1", "object1", "职员名单.xlsx", "synthetic", "second"); err != nil {
		t.Fatal(err)
	}
	if err = s.ClearRange("chat_logs_synthetic", "synthetic", at, at); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(first); err != nil {
		t.Fatal("shared object removed while second reference remains")
	}
	if err = s.ClearRange("chat_logs_synthetic", "synthetic", at+10, at+10); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(first); !os.IsNotExist(err) {
		t.Fatal("last reference did not remove shared object")
	}
	if _, err = os.Stat(second); !os.IsNotExist(err) {
		t.Fatal("last reference did not remove second object")
	}
	_ = root
}
