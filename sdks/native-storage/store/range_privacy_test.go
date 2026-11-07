package nativestore

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestSameTimestampArchiveCursorDoesNotSkip(t *testing.T) {
	s, _ := fixture(t)
	at := time.Now().UnixMilli()
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		insert(t, s, id, "SYNTHETIC", at)
	}
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	before := at + 1
	last := ""
	seen := map[string]bool{}
	for {
		page, err := s.PageCursor("synthetic", before, last, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		for _, raw := range page {
			var item struct {
				ID string `json:"client_msg_id"`
				At int64  `json:"send_time"`
			}
			if json.Unmarshal(raw, &item) != nil || seen[item.ID] {
				t.Fatal("duplicate or invalid page")
			}
			seen[item.ID] = true
			before = item.At
			last = item.ID
		}
	}
	if len(seen) != 5 {
		t.Fatal("same timestamp rows skipped", len(seen))
	}
}
func TestAuthenticatedRangesAreAccountBoundAndClearClosesCache(t *testing.T) {
	s, _ := fixture(t)
	at := time.Now().UnixMilli()
	insert(t, s, "range", "SYNTHETIC", at)
	body := bytes.Repeat([]byte("synthetic"), 150000)
	path, err := s.StageOutgoing(bytes.NewReader(body), "sample.mp4")
	if err != nil {
		t.Fatal(err)
	}
	part, err := s.ReadOutgoingRange(path, (1<<20)-5, 45)
	if err != nil || !bytes.Equal(part, body[(1<<20)-5:(1<<20)+40]) {
		t.Fatal("source cross-frame range", err)
	}
	t.Run("otherAccount", func(t *testing.T) {
		other, _ := fixture(t)
		if data, err := other.ReadOutgoingRange(path, 0, 5); err == nil || data != nil {
			t.Fatal("another account read source")
		}
	})
	name, err := s.BeginAttachment("range-task", "range-object", "same.mp4", "synthetic", "range")
	if err != nil {
		t.Fatal(err)
	}
	for start := 0; start < len(body); start += 65536 {
		end := start + 65536
		if end > len(body) {
			end = len(body)
		}
		if err = s.WriteAttachment("range-task", body[start:end]); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.FinishAttachment("range-task", int64(len(body))); err != nil {
		t.Fatal(err)
	}
	part, err = s.ReadAttachmentRange("range-task", (1<<20)-3, 42)
	if err != nil || !bytes.Equal(part, body[(1<<20)-3:(1<<20)+39]) {
		t.Fatal("attachment range", err)
	}
	if err = s.ClearRange("chat_logs_synthetic", "synthetic", at, at); err != nil {
		t.Fatal(err)
	}
	if data, err := s.ReadAttachmentRange("range-task", 0, 4); err == nil || data != nil {
		t.Fatal("cleared range remained readable")
	}
	if _, err = os.Stat(name); !os.IsNotExist(err) {
		t.Fatal("attachment not removed")
	}
	if _, err = s.BeginAttachment("range-task", "range-object", "same.mp4", "synthetic", "range"); err == nil {
		t.Fatal("cleared message reacquired source")
	}
}
func TestDestructTransitionPurgesOldArchiveWithoutAcknowledgingPendingSource(t *testing.T) {
	s, _ := fixture(t)
	at := time.Now().UnixMilli()
	_, err := s.db.Exec(`CREATE TABLE local_conversations(conversation_id TEXT PRIMARY KEY,is_private_chat INTEGER DEFAULT 0,is_msg_destruct INTEGER DEFAULT 0);INSERT INTO local_conversations(conversation_id)VALUES('synthetic')`)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.InstallConversation("chat_logs_synthetic", "synthetic"); err != nil {
		t.Fatal(err)
	}
	insert(t, s, "old-private", "SYNTHETIC_OLD", at)
	if err = s.Flush(100); err != nil {
		t.Fatal(err)
	}
	file, err := s.StageOutgoing(bytes.NewBufferString("SYNTHETIC_PENDING"), "pending.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.BindOutgoing("old-private", file); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`UPDATE local_conversations SET is_msg_destruct=1 WHERE conversation_id='synthetic'`); err != nil {
		t.Fatal(err)
	}
	if err = s.Flush(100); err != nil {
		t.Fatal(err)
	}
	if count(t, s, `SELECT count(*) FROM puty_archive_index`) != 0 || count(t, s, `SELECT count(*) FROM puty_local_deleted`) != 0 {
		t.Fatal("privacy archive/deletion semantics")
	}
	if _, err = os.Stat(file); err != nil {
		t.Fatal("live source incorrectly released", err)
	}
	if _, err = s.db.Exec(`UPDATE local_conversations SET is_msg_destruct=0;UPDATE chat_logs_synthetic SET is_read=0 WHERE client_msg_id='old-private'`); err != nil {
		t.Fatal(err)
	}
	if err = s.Flush(100); err != nil {
		t.Fatal(err)
	}
	if count(t, s, `SELECT count(*) FROM puty_archive_index`) != 0 {
		t.Fatal("old ephemeral message became permanent")
	}
}
