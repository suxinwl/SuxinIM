package nativestore

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupAcknowledgementCannotDeleteSnapshotSource(t *testing.T) {
	s, _ := fixture(t)
	source, err := s.StageOutgoing(bytes.NewBufferString("SYNTHETIC_PENDING_BACKUP"), "pending.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.BindOutgoing("pending-backup", source); err != nil {
		t.Fatal(err)
	}
	type result struct {
		path string
		err  error
	}
	backup := make(chan result, 1)
	go func() { path, err := s.ExportBackup("synthetic recovery password"); backup <- result{path, err} }()
	deadline := time.Now().Add(3 * time.Second)
	held := false
	for time.Now().Before(deadline) {
		if outgoingGuard.TryLock() {
			outgoingGuard.Unlock()
			time.Sleep(time.Millisecond)
		} else {
			held = true
			break
		}
	}
	if !held {
		t.Fatal("backup did not protect outgoing snapshot dependencies")
	}
	ack := make(chan error, 1)
	go func() { ack <- s.ReleaseOutgoing("pending-backup") }()
	select {
	case err := <-ack:
		t.Fatal("ACK bypassed snapshot source freeze", err)
	case <-time.After(80 * time.Millisecond):
	}
	exported := <-backup
	if exported.err != nil {
		t.Fatal(exported.err)
	}
	if err = <-ack; err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(exported.path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	found := false
	for _, entry := range archive.File {
		if entry.Name == "data/outgoing/"+filepath.Base(source) {
			found = true
		}
	}
	if !found {
		t.Fatal("snapshot retained a pending ref without its encrypted source")
	}
	prepared, err := s.PrepareRestore(exported.path, "synthetic recovery password")
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Abort()
	if _, err = os.Stat(filepath.Join(prepared.stage, "outgoing", filepath.Base(source))); err != nil {
		t.Fatal("restorable pending source missing", err)
	}
}

func TestRestoreCommitMergesPostPrepareDeletesRangesAndPrivacy(t *testing.T) {
	s, root := fixture(t)
	for index, id := range []string{"recalled", "range", "private", "keep"} {
		insert(t, s, id, "SYNTHETIC_"+id, int64(index+1)*10)
	}
	output, err := s.ExportBackup("synthetic recovery password")
	if err != nil {
		t.Fatal(err)
	}
	if err = Configure(s.account, root, bytes.Repeat([]byte{0x71}, 32), time.Now().Add(8*time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	prepared, err := s.PrepareRestore(output, "synthetic recovery password")
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Abort()
	if err = s.DeleteLocalMessage("chat_logs_synthetic", "synthetic", "recalled", 10); err != nil {
		t.Fatal(err)
	}
	if err = s.ClearScopes([]string{"synthetic"}, 20, 21); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`UPDATE chat_logs_synthetic SET attached_info='{"isPrivateChat":true}' WHERE client_msg_id='private'`); err != nil {
		t.Fatal(err)
	}
	if err = s.Flush(100); err != nil {
		t.Fatal(err)
	}
	if err = prepared.Commit(bytes.Repeat([]byte{0x42}, 64)); err != nil {
		t.Fatal(err)
	}
	account := s.account
	Lock(account)
	if err = Configure(account, root, bytes.Repeat([]byte{0x71}, 32), time.Now().Add(8*time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	restored, err := Lookup(account)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restored.OpenWorkingDatabase(); err != nil {
		t.Fatal(err)
	}
	restored.MarkViewsReady(true)
	page, err := restored.Page("synthetic", 100, 20)
	if err != nil || len(page) != 1 {
		t.Fatal("post-prepare suppressed data revived", len(page), err)
	}
	if !bytes.Contains(page[0], []byte(`"client_msg_id":"keep"`)) {
		t.Fatal("wrong surviving archive")
	}
	if !restored.Suppressed("synthetic", "recalled", 10) || !restored.Suppressed("synthetic", "range", 20) {
		t.Fatal("post-prepare suppression omitted")
	}
	if count(t, restored, `SELECT count(*) FROM puty_local_ephemeral WHERE message_id='private'`) != 1 {
		t.Fatal("post-prepare privacy omitted")
	}
}

func TestCompleteBackupRestorePreservesNewerLocalClear(t *testing.T) {
	s, root := fixture(t)
	insert(t, s, "old", "SYNTHETIC_BACKUP_PAYLOAD", 10)
	insert(t, s, "keep", "SYNTHETIC_KEEP", 30)
	task, err := s.BeginAttachment("backup-task", "object", "原名.txt", "synthetic", "old")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.WriteAttachment("backup-task", []byte("SYNTHETIC_ATTACHMENT")); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishAttachment("backup-task", 20); err != nil {
		t.Fatal(err)
	}
	output, err := s.ExportBackup("synthetic recovery password")
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range reader.File {
		r, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("SYNTHETIC_BACKUP_PAYLOAD")) || bytes.Contains(data, []byte("SYNTHETIC_ATTACHMENT")) {
			t.Fatal("backup retained plaintext")
		}
	}
	_ = reader.Close()
	if _, err = s.PrepareRestore(output, "incorrect synthetic password"); err == nil {
		t.Fatal("wrong recovery password accepted")
	}
	if err = Configure(s.account, root, bytes.Repeat([]byte{0x71}, 32), time.Now().Add(8*time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err = s.ClearRange("chat_logs_synthetic", "synthetic", 0, 20); err != nil {
		t.Fatal(err)
	}
	prepared, err := s.PrepareRestore(output, "synthetic recovery password")
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Abort()
	if len(prepared.MasterKey()) != 32 {
		t.Fatal("native recovered key unavailable")
	}
	if _, err = os.Stat(filepath.Join(prepared.stage, "archive", filepath.Base(filepath.Dir(filepath.Dir(task))), "attachments", filepath.Base(task))); err == nil {
		t.Fatal("newer local clear restored attachment")
	}
	if err = prepared.Commit(bytes.Repeat([]byte{0x42}, 64)); err != nil {
		t.Fatal(err)
	}
	account := s.account
	Lock(account)
	if err = Configure(account, root, bytes.Repeat([]byte{0x71}, 32), time.Now().Add(8*time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	restored, err := Lookup(account)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restored.OpenWorkingDatabase(); err != nil {
		t.Fatal(err)
	}
	if _, err = restored.Page("synthetic", 100, 10); err == nil {
		t.Fatal("restored archive exposed before fresh server sync")
	}
	restored.MarkViewsReady(true)
	page, err := restored.Page("synthetic", 100, 10)
	if err != nil || len(page) != 1 {
		t.Fatal("restore local tombstone mismatch", len(page), err)
	}
	var record map[string]json.RawMessage
	_ = json.Unmarshal(page[0], &record)
	if string(record["client_msg_id"]) != "\"keep\"" {
		t.Fatal("wrong restored record")
	}
}

func TestBackupRejectsTraversalMonthAndUnauthenticatedMetadata(t *testing.T) {
	for _, path := range []string{"../working.db", "archive/202613/messages/" + string(bytes.Repeat([]byte{'a'}, 64)), "archive/202611/attachments/../x", "archive/202611/attachments/x.enc/child"} {
		if validBackupPath(path) {
			t.Fatal("invalid backup path accepted")
		}
	}
	s, _ := fixture(t)
	insert(t, s, "one", "secret", 1)
	output, err := s.ExportBackup("synthetic recovery password")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)/2] ^= 1
	broken := filepath.Join(t.TempDir(), "tampered.backup")
	if err = os.WriteFile(broken, data, 0600); err != nil {
		t.Fatal(err)
	}
	if prepared, err := s.PrepareRestore(broken, "synthetic recovery password"); err == nil {
		prepared.Abort()
		t.Fatal("tampered backup accepted")
	}
}
