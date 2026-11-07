package nativestore

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
	localstore "puty.local/localstore"
)

func TestRealLoopbackHTTPRangeEncryptedResume(t *testing.T) {
	s, _ := fixture(t)
	payload := bytes.Repeat([]byte{0x51}, 3*localstore.ChunkSize+123)
	var ranges atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"synthetic-etag"`)
		offset := 0
		if value := r.Header.Get("Range"); value != "" {
			if value != fmt.Sprintf("bytes=%d-", localstore.ChunkSize) || r.Header.Get("If-Range") != `"synthetic-etag"` {
				w.WriteHeader(400)
				return
			}
			offset = localstore.ChunkSize
			ranges.Add(1)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, len(payload)-1, len(payload)))
			w.WriteHeader(206)
		}
		_, _ = w.Write(payload[offset:])
	}))
	defer server.Close()
	path, err := s.BeginAttachment("real-range", "synthetic-object", "测试续传.zip", "synthetic", "range-msg")
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetAttachmentETag("real-range", response.Header.Get("ETag")); err != nil {
		t.Fatal(err)
	}
	prefix := make([]byte, localstore.ChunkSize+100)
	if _, err = io.ReadFull(response.Body, prefix); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	for len(prefix) > 0 {
		count := 65536
		if count > len(prefix) {
			count = len(prefix)
		}
		if err = s.WriteAttachment("real-range", prefix[:count]); err != nil {
			t.Fatal(err)
		}
		prefix = prefix[count:]
	}
	s.AbortAttachment("real-range")
	resumed, err := s.BeginAttachment("real-range", "synthetic-object", "测试续传.zip", "synthetic", "range-msg")
	if err != nil || resumed != path {
		t.Fatal("range changed actual allocated path", err)
	}
	offset := s.AttachmentOffset("real-range")
	if offset != localstore.ChunkSize {
		t.Fatal("resume restarted rather than retained authenticated bytes", offset)
	}
	request, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	request.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	request.Header.Set("If-Range", s.AttachmentETag("real-range"))
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 206 {
		t.Fatal("actual network did not return 206")
	}
	buffer := make([]byte, 65536)
	for {
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			if err = s.WriteAttachment("real-range", buffer[:n]); err != nil {
				t.Fatal(err)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			t.Fatal(readErr)
		}
	}
	if err = s.FinishAttachment("real-range", int64(len(payload))); err != nil {
		t.Fatal(err)
	}
	restored, err := s.ReadAttachment("real-range", 8<<20)
	if err != nil || !bytes.Equal(restored, payload) || ranges.Load() != 1 {
		t.Fatal("actual HTTP resumed payload mismatch", err)
	}
}

func TestCompanyMonthBoundaryZeroAttachmentAndPersistentFirstMonth(t *testing.T) {
	s, _ := fixture(t)
	oldClock := namingClock
	defer func() { namingClock = oldClock }()
	namingClock = func() time.Time { return time.Date(2026, 10, 31, 15, 59, 59, 0, time.UTC) }
	first, err := s.BeginAttachment("zero", "zero-object", "空文件.txt", "synthetic", "zero-msg")
	if err != nil {
		t.Fatal(err)
	}
	s.AbortAttachment("zero")
	namingClock = func() time.Time { return time.Date(2026, 10, 31, 16, 0, 0, 0, time.UTC) }
	resumed, err := s.BeginAttachment("zero", "zero-object", "空文件.txt", "synthetic", "zero-msg")
	if err != nil || resumed != first || filepath.Base(filepath.Dir(filepath.Dir(first))) != "202610" {
		t.Fatal("retry changed first-download company month", err)
	}
	if err = s.FinishAttachment("zero", 0); err != nil {
		t.Fatal(err)
	}
	data, err := s.ReadAttachment("zero", 1)
	if err != nil || len(data) != 0 {
		t.Fatal("zero-byte EOF authentication failed", err)
	}
	second, err := s.BeginAttachment("new-month", "second-object", "空文件.txt", "synthetic", "new-msg")
	if err != nil || filepath.Base(filepath.Dir(filepath.Dir(second))) != "202611" {
		t.Fatal("China midnight boundary incorrect", err)
	}
	s.AbortAttachment("new-month")
}

func TestLegacyPlaintextSQLiteRealEncryptedMigration(t *testing.T) {
	account := "synthetic-legacy"
	root := t.TempDir()
	oldRoot := t.TempDir()
	oldPath := filepath.Join(oldRoot, "OpenIM_v3_"+account+".db")
	sql.Register("synthetic-legacy-plain", &sqlite3.SQLiteDriver{})
	plain, err := sql.Open("synthetic-legacy-plain", oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = plain.Exec(`CREATE TABLE original_synthetic(body TEXT);INSERT INTO original_synthetic VALUES('SYNTHETIC_LEGACY_BODY')`); err != nil {
		t.Fatal(err)
	}
	_ = plain.Close()
	if err = Configure(account, root, bytes.Repeat([]byte{0x59}, 32), time.Now().Add(8*time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	defer Lock(account)
	s, err := Lookup(account)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.MigrateLegacy(oldPath); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("legacy plaintext source remained after encrypted commit")
	}
	db, err := s.OpenWorkingDatabase()
	if err != nil {
		t.Fatal(err)
	}
	var restored string
	if err = db.QueryRow(`SELECT body FROM original_synthetic`).Scan(&restored); err != nil || restored != "SYNTHETIC_LEGACY_BODY" {
		t.Fatal("legacy data lost", err)
	}
	sealed, err := os.ReadFile(filepath.Join(root, "working.db"))
	if err != nil || bytes.HasPrefix(sealed, []byte("SQLite format 3")) || bytes.Contains(sealed, []byte("SYNTHETIC_LEGACY_BODY")) {
		t.Fatal("migration database is plaintext", err)
	}
}

func TestColdSparseUpdateKeepsArchivedPayload(t *testing.T) {
	s, _ := fixture(t)
	at := time.Now().AddDate(0, 0, -100).UnixMilli()
	insert(t, s, "old", "SYNTHETIC_COLD_BODY", at)
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	if err := s.EvictHot("chat_logs_synthetic", "synthetic", time.Now().AddDate(0, 0, -90).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE chat_logs_synthetic SET is_read=1,seq=17 WHERE client_msg_id='old'`); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	record, err := s.RecordPayload("synthetic", "old")
	if err != nil || !bytes.Contains(record["content"], []byte("SYNTHETIC_COLD_BODY")) || string(record["seq"]) != "17" {
		t.Fatal("thin metadata update overwrote cold content", err)
	}
}
