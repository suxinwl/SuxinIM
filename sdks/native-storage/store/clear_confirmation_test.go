package nativestore

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"
)

func TestClearConfirmationNeverMasksLaterMessagesButKeepsExactSelectedIDs(t *testing.T) {
	for _, mode := range []string{"range", "scopes"} {
		t.Run(mode, func(t *testing.T) {
			s, root := fixture(t)
			confirmed := time.Date(2026, 11, 4, 10, 0, 0, 0, CompanyZone).UnixMilli()
			clock := clearConfirmationClock
			clearConfirmationClock = func() time.Time { return time.UnixMilli(confirmed) }
			defer func() { clearConfirmationClock = clock }()
			insert(t, s, "old", "SYNTHETIC_OLD", confirmed-1)
			insert(t, s, "already-present-clock-skew", "SYNTHETIC_SKEWED", confirmed+10000)
			if err := s.Flush(100); err != nil {
				t.Fatal(err)
			}
			var err error
			if mode == "range" {
				err = s.ClearRange("chat_logs_synthetic", "synthetic", 0, 9007199254740991)
			} else {
				err = s.ClearScopes([]string{"synthetic"}, 0, 9007199254740991)
			}
			if err != nil {
				t.Fatal(err)
			}
			var upper int64
			if err = s.db.QueryRow(`SELECT MAX(to_time) FROM puty_local_tombstones WHERE conversation='synthetic'`).Scan(&upper); err != nil || upper != confirmed {
				t.Fatal("range did not stop at confirmation", upper, err)
			}
			// A changed timestamp cannot revive an explicitly selected message.
			if !s.Suppressed("synthetic", "old", confirmed+20000) || !s.Suppressed("synthetic", "already-present-clock-skew", confirmed+10000) || s.Suppressed("synthetic", "new-after-clear", confirmed+1) || s.Suppressed("other", "old", confirmed-1) {
				t.Fatal("confirmation/exact-ID boundary failed")
			}
			// Clear intentionally retains the sequence-preserving thin row. Real SDK
			// resync upserts that existing primary key rather than inserting a new one.
			if count(t, s, `SELECT count(*) FROM chat_logs_synthetic WHERE client_msg_id='old' AND status=4 AND content=''`) != 1 {
				t.Fatal("selected sync row did not remain thin before resync")
			}
			if _, err = s.db.Exec(`INSERT OR REPLACE INTO chat_logs_synthetic(client_msg_id,content,send_time) VALUES(?,?,?)`, "old", "SYNTHETIC_LATE_RESYNC", confirmed+20000); err != nil {
				t.Fatal(err)
			}
			insert(t, s, "new-after-clear", "SYNTHETIC_NEW", confirmed+1)
			if err = s.Flush(100); err != nil {
				t.Fatal(err)
			}
			if count(t, s, `SELECT count(*) FROM chat_logs_synthetic WHERE client_msg_id='old' AND status=4 AND content=''`) != 1 || count(t, s, `SELECT count(*) FROM chat_logs_synthetic WHERE client_msg_id='new-after-clear' AND status=2 AND content='SYNTHETIC_NEW'`) != 1 {
				t.Fatal("old resync revived or later message was swallowed")
			}
			Lock(s.account)
			if err = Configure(s.account, root, bytes.Repeat([]byte{0x71}, 32), time.Now().Add(8*time.Second).UnixMilli()); err != nil {
				t.Fatal(err)
			}
			fresh, err := Lookup(s.account)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = fresh.Open(filepath.Join(root, "synthetic.db")); err != nil {
				t.Fatal(err)
			}
			fresh.MarkViewsReady(true)
			if !fresh.Suppressed("synthetic", "old", confirmed+20000) || fresh.Suppressed("synthetic", "later-fresh-id", confirmed+2) {
				t.Fatal("restart lost exact deletion or broadened confirmation range")
			}
		})
	}
}

func TestFutureOnlyClearKeepsExactIDsWithoutCreatingAFutureRange(t *testing.T) {
	s, _ := fixture(t)
	confirmed := time.Now().UnixMilli()
	clock := clearConfirmationClock
	clearConfirmationClock = func() time.Time { return time.UnixMilli(confirmed) }
	defer func() { clearConfirmationClock = clock }()
	insert(t, s, "existing-future", "SYNTHETIC_EXISTING", confirmed+100)
	if err := s.ClearScopes([]string{"synthetic"}, confirmed+1, confirmed+1000); err != nil {
		t.Fatal(err)
	}
	if count(t, s, `SELECT count(*) FROM puty_local_tombstones`) != 0 || !s.Suppressed("synthetic", "existing-future", confirmed+100) || s.Suppressed("synthetic", "unseen-future", confirmed+100) {
		t.Fatal("future-only selection became an ongoing mask")
	}
}
