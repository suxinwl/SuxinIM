// Copy this independent review test into sdks/native-storage/store only when
// its owner is ready to build/run the SQLCipher Android suite. Synthetic only.
package nativestore

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func reviewPrivateSource(t *testing.T, s *Store, id string, at int64) string {
	t.Helper()
	if _, err := s.db.Exec(`INSERT INTO chat_logs_synthetic(client_msg_id,send_time,content,attached_info,status)VALUES(?,?,?, ?,1)`, id, at, "SYNTHETIC_PRIVATE_PENDING", `{"isPrivateChat":true}`); err != nil {
		t.Fatal(err)
	}
	file, err := s.StageOutgoing(bytes.NewBufferString("SYNTHETIC_PRIVATE_SOURCE"), "private.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.BindOutgoing(id, file); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestReviewPrivateNonArchiveIsNotDeleteAcknowledgement(t *testing.T) {
	s, _ := fixture(t)
	at := time.Now().UnixMilli()
	file := reviewPrivateSource(t, s, "private-unacked", at)
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	if count(t, s, `SELECT count(*) FROM puty_archive_index`) != 0 {
		t.Fatal("private record entered archive")
	}
	if count(t, s, `SELECT count(*) FROM puty_local_deleted WHERE message_id='private-unacked'`) != 0 {
		t.Fatal("live private message became deleted")
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("unacknowledged private source was released", err)
	}
	if count(t, s, `SELECT count(*) FROM puty_outgoing_refs WHERE message_id='private-unacked'`) != 1 {
		t.Fatal("pending private source reference lost")
	}
}

func TestReviewHotOnlyClearReleasesSourceAndAttachment(t *testing.T) {
	s, _ := fixture(t)
	at := time.Now().UnixMilli()
	source := reviewPrivateSource(t, s, "private-clear", at)
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	file, err := s.BeginAttachment("review-private", "review-object", "private.txt", "synthetic", "private-clear")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.WriteAttachment("review-private", []byte("SYNTHETIC_PRIVATE_DOWNLOAD")); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishAttachment("review-private", int64(len("SYNTHETIC_PRIVATE_DOWNLOAD"))); err != nil {
		t.Fatal(err)
	}
	if err = s.ClearRange("chat_logs_synthetic", "synthetic", at, at); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{source, file} {
		if _, err = os.Stat(name); !os.IsNotExist(err) {
			t.Fatal("hot-only clear retained encrypted payload", err)
		}
	}
	for _, table := range []string{"puty_attachment_refs", "puty_attachment_tasks", "puty_outgoing_refs", "puty_outgoing_sources"} {
		if count(t, s, `SELECT count(*) FROM `+table) != 0 {
			t.Fatal("hot-only clear retained metadata", table)
		}
	}
}

func TestReviewArchiveOnlyClearIncludesAttachmentReferences(t *testing.T) {
	s, _ := fixture(t)
	at := time.Now().UnixMilli()
	insert(t, s, "cold-only", "SYNTHETIC_COLD", at)
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	file, err := s.BeginAttachment("review-cold", "review-cold-object", "cold.txt", "synthetic", "cold-only")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.WriteAttachment("review-cold", []byte("SYNTHETIC_COLD_DOWNLOAD")); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishAttachment("review-cold", int64(len("SYNTHETIC_COLD_DOWNLOAD"))); err != nil {
		t.Fatal(err)
	}
	// ClearRange deliberately permits a missing derived working table. Monthly
	// records and their references must still be included in the clear request.
	if _, err = s.db.Exec(`DROP TABLE chat_logs_synthetic`); err != nil {
		t.Fatal(err)
	}
	if err = s.ClearRange("chat_logs_synthetic", "synthetic", at, at); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("archive-only clear retained attachment", err)
	}
	if count(t, s, `SELECT count(*) FROM puty_attachment_refs`) != 0 {
		t.Fatal("archive-only reference survived clear")
	}
}

func TestReviewOutgoingUnlinkFailureRemainsRetryable(t *testing.T) {
	s, _ := fixture(t)
	file, err := s.StageOutgoing(bytes.NewBufferString("SYNTHETIC_RETRY"), "retry.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.BindOutgoing("review-retry", file); err != nil {
		t.Fatal(err)
	}
	backup := file + ".review-hold"
	if err = os.Rename(file, backup); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(file, "synthetic-blocker")
	if err = os.WriteFile(blocker, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = s.ReleaseOutgoing("review-retry"); err == nil {
		t.Fatal("injected unlink failure was not reported")
	}
	if err = os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(backup, file); err != nil {
		t.Fatal(err)
	}
	if err = s.ReleaseOutgoing("review-retry"); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("retry lost exact object after refs were removed", err)
	}
}

func TestReviewAttachmentUnlinkFailureRemainsRetryable(t *testing.T) {
	s, _ := fixture(t)
	at := time.Now().UnixMilli()
	insert(t, s, "download-retry", "SYNTHETIC_RETRY_RECORD", at)
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	file, err := s.BeginAttachment("review-download-retry", "review-retry-object", "retry.txt", "synthetic", "download-retry")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.WriteAttachment("review-download-retry", []byte("SYNTHETIC_RETRY_FILE")); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishAttachment("review-download-retry", int64(len("SYNTHETIC_RETRY_FILE"))); err != nil {
		t.Fatal(err)
	}
	backup := file + ".review-hold"
	if err = os.Rename(file, backup); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(file, "synthetic-blocker")
	if err = os.WriteFile(blocker, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = s.ClearRange("chat_logs_synthetic", "synthetic", at, at); err == nil {
		t.Fatal("injected unlink failure was not reported")
	}
	if err = os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(backup, file); err != nil {
		t.Fatal(err)
	}
	if err = s.ClearRange("chat_logs_synthetic", "synthetic", at, at); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("retry lost attachment after refs/event ACK", err)
	}
}
