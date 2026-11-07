package nativestore

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClearScopesUnionPhysicalBytesAndBeijingExclusiveBoundary(t *testing.T) {
	s, _ := fixture(t)
	if _, err := s.db.Exec(`CREATE TABLE chat_logs_second AS SELECT * FROM chat_logs_synthetic WHERE 0`); err != nil {
		t.Fatal(err)
	}
	if err := s.InstallConversation("chat_logs_second", "second"); err != nil {
		t.Fatal(err)
	}
	boundary := time.Date(2026, 11, 1, 0, 0, 0, 0, CompanyZone).UnixMilli()
	insert(t, s, "a", "SYNTHETIC_MONTH_END", boundary-1)
	insert(t, s, "boundary", "SYNTHETIC_KEEP_NEXT_MONTH", boundary)
	insert(t, s, "private", "SYNTHETIC_PRIVATE", boundary-1)
	if _, err := s.db.Exec(`UPDATE chat_logs_synthetic SET attached_info='{"isPrivateChat":true}' WHERE client_msg_id='private'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO chat_logs_second(client_msg_id,content,send_time,status) VALUES('b','SYNTHETIC_B',?,2)`, boundary-1); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	shared, err := s.BeginAttachment("union-shared", "SYNTHETIC_SHARED", "共享.xlsx", "synthetic", "a")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.WriteAttachment("union-shared", []byte("SYNTHETIC_SHARED_BODY")); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishAttachment("union-shared", int64(len("SYNTHETIC_SHARED_BODY"))); err != nil {
		t.Fatal(err)
	}
	if _, err = s.BeginAttachment("union-shared", "SYNTHETIC_SHARED", "共享.xlsx", "second", "b"); err != nil {
		t.Fatal(err)
	}
	pending, err := s.StageOutgoing(bytes.NewBufferString("SYNTHETIC_PENDING_PRIVATE"), "私密.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.BindOutgoing("private", pending); err != nil {
		t.Fatal(err)
	}
	single, err := s.PreviewClearScopes([]string{"synthetic"}, 0, boundary)
	if err != nil {
		t.Fatal(err)
	}
	both, err := s.PreviewClearScopes([]string{"synthetic", "second", "synthetic"}, 0, boundary)
	if err != nil {
		t.Fatal(err)
	}
	if both.MessageCount != 3 || both.AttachmentReferences != 2 || len(both.ConversationIDs) != 2 || both.ConversationIDs[0] != "second" {
		t.Fatalf("incorrect union: %+v", both)
	}
	var expected int64
	expectedFiles := 0
	for _, path := range []string{shared, pending} {
		info, e := os.Stat(path)
		if e != nil {
			t.Fatal(e)
		}
		expected += info.Size()
		expectedFiles++
	}
	rows, err := s.db.Query(`SELECT path FROM puty_archive_index WHERE send_time<?`, boundary)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var path string
		if err = rows.Scan(&path); err != nil {
			t.Fatal(err)
		}
		info, e := os.Stat(filepath.Join(s.root, "archive", path))
		if e != nil {
			t.Fatal(e)
		}
		expected += info.Size()
		expectedFiles++
	}
	rows.Close()
	if both.EstimatedReleaseBytes != expected || both.FilesToDelete != expectedFiles {
		t.Fatalf("not actual deduplicated ciphertext bytes: %+v expected %d/%d", both, expected, expectedFiles)
	}
	if single.FilesToDelete >= both.FilesToDelete {
		t.Fatal("shared other-scope attachment was counted by scalar scope")
	}
	if err = s.ClearScopes([]string{"second", "synthetic"}, 0, boundary); err != nil {
		t.Fatal(err)
	}
	if !s.Suppressed("second", "b", boundary-1) || !s.Suppressed("synthetic", "private", boundary-1) || s.Suppressed("synthetic", "boundary", boundary) {
		t.Fatal("half-open suppression boundary failed")
	}
	for _, path := range []string{shared, pending} {
		if _, e := os.Stat(path); !os.IsNotExist(e) {
			t.Fatal("union retained exclusive cipher", e)
		}
	}
	if count(t, s, `SELECT count(*) FROM chat_logs_synthetic WHERE client_msg_id='boundary' AND status<4`) != 1 {
		t.Fatal("Beijing next-month midnight was cleared")
	}
	after, err := s.PreviewClearScopes([]string{"synthetic", "second"}, 0, boundary)
	if err != nil || after.MessageCount != 0 || after.EstimatedReleaseBytes != 0 {
		t.Fatal("clear not idempotent", err, after)
	}
	if _, err = s.PreviewClearScopes([]string{"synthetic", "invalid'"}, 0, boundary); err == nil {
		t.Fatal("invalid joint scope accepted")
	}
	if err = s.ClearScopes([]string{"synthetic", "invalid'"}, boundary, boundary+1); err == nil {
		t.Fatal("invalid joint clear accepted")
	}
	if s.Suppressed("synthetic", "boundary", boundary) {
		t.Fatal("failed multi-scope validation partially cleared")
	}
}

func TestImportFailureRetryReclaimsOriginalPhysicalName(t *testing.T) {
	s, _ := fixture(t)
	path, err := s.StageOutgoing(bytes.NewBufferString("SYNTHETIC_RETRY"), "职员名单.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	// Force the final producer-discard to fail after the import file is durable.
	// The deferred exact-ID cleanup must release that name before retry.
	if err = s.BindOutgoing("synthetic-pending", path); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportLocalAttachment(path, "synthetic"); err == nil {
		t.Fatal("active pending source was discarded")
	}
	if count(t, s, `SELECT count(*) FROM puty_local_imports`) != 0 || count(t, s, `SELECT count(*) FROM puty_attachment_tasks`) != 0 {
		t.Fatal("failed completed import left an occupied name")
	}
	// Simulate the producer releasing its reference without discarding the
	// encrypted source: retry uses identical bytes and the lowest free name.
	if _, err = s.db.Exec(`DELETE FROM puty_outgoing_refs WHERE message_id='synthetic-pending'`); err != nil {
		t.Fatal(err)
	}
	item, err := s.ImportLocalAttachment(path, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	if item.Name != "职员名单.xlsx" {
		t.Fatal("failed import consumed original filename")
	}
}
