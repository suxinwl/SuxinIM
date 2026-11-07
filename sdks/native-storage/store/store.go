// Package nativestore owns SQLCipher handles and authenticated monthly archives.
// Keys enter only from the native OS broker after its online authorization check.
package nativestore

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
	localstore "puty.local/localstore"
)

var CompanyZone = time.FixedZone("Asia/Shanghai", 8*3600)
var namingClock = time.Now

var ErrLocked = errors.New("local store requires current online authorization")
var registry = struct {
	sync.Mutex
	stores map[string]*Store
}{stores: make(map[string]*Store)}
var once sync.Once

type Store struct {
	mu                    sync.Mutex
	openMu                sync.Mutex
	flushMu               sync.Mutex
	gcMu                  sync.Mutex
	account, root, driver string
	master                []byte
	expires               time.Time
	db                    *sql.DB
	dbPath                string
	closed                bool
	viewsReady            bool
	parent                *Store
	rangeMu               sync.Mutex
	rangeReaders          map[string]*rangeReader
	legacyRoots           []legacyRoot
	quarantineRoot        string
	quarantineMaster      []byte
}

// Configure is deliberately absent from any Dart MethodChannel. Native Java/Swift
// validate the current actor with the server, then unwrap their OS-owned key.
func Configure(account, directory string, master []byte, authorizedUntil int64) error {
	if account == "" || len(master) != 32 || !filepath.IsAbs(directory) {
		return ErrLocked
	}
	now := time.Now()
	remaining := time.UnixMilli(authorizedUntil).Sub(now)
	if remaining <= 0 || remaining > 10*time.Second {
		return ErrLocked
	}
	// Retain Go's monotonic component; wall-clock rollback cannot prolong a lease.
	deadline := now.Add(remaining)
	registry.Lock()
	defer registry.Unlock()
	if old := registry.stores[account]; old != nil {
		old.mu.Lock()
		defer old.mu.Unlock()
		if old.closed || old.root != directory || !bytes.Equal(old.master, master) {
			return ErrLocked
		}
		old.expires = deadline
		return nil
	}
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return ErrLocked
	}
	s := &Store{account: account, root: directory, master: bytes.Clone(master), expires: deadline, driver: "puty_cipher_" + hex.EncodeToString(suffix[:])}
	sql.Register(s.driver, &sqlite3.SQLiteDriver{KeyHook: func(c *sqlite3.SQLiteConn) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.authorizedLocked() {
			return ErrLocked
		}
		if err := c.SetRawKey(s.master); err != nil {
			return err
		}
		c.AccessCheck = func() error {
			s.mu.Lock()
			defer s.mu.Unlock()
			if !s.authorizedLocked() {
				return ErrLocked
			}
			return nil
		}
		// Every SQLite operation rechecks the in-memory lease, including cached SDK
		// handles after network failure. The broker must refresh it online.
		c.RegisterAuthorizer(func(action int, first, _, _ string) int {
			if action == sqlite3.SQLITE_TRANSACTION && first == "ROLLBACK" {
				return sqlite3.SQLITE_OK
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if !s.authorizedLocked() {
				return sqlite3.SQLITE_DENY
			}
			return sqlite3.SQLITE_OK
		})
		return nil
	}})
	registry.stores[account] = s
	return nil
}

func (s *Store) authorizedLocked() bool {
	if s.parent != nil {
		s.parent.mu.Lock()
		defer s.parent.mu.Unlock()
		return !s.closed && s.parent.authorizedLocked()
	}
	return !s.closed && time.Now().Before(s.expires)
}
func (s *Store) MarkViewsReady(ready bool) { s.mu.Lock(); s.viewsReady = ready; s.mu.Unlock() }
func Lookup(account string) (*Store, error) {
	registry.Lock()
	defer registry.Unlock()
	s := registry.stores[account]
	if s == nil {
		return nil, ErrLocked
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.authorizedLocked() {
		return nil, ErrLocked
	}
	return s, nil
}
func Lock(account string) {
	registry.Lock()
	s := registry.stores[account]
	delete(registry.stores, account)
	registry.Unlock()
	if s != nil {
		s.mu.Lock()
		s.closed = true
		s.expires = time.Time{}
		clear(s.master)
		clear(s.quarantineMaster)
		s.mu.Unlock()
		// Opening SQLCipher may invoke AccessCheck, which takes mu. Mark the
		// owner closed first, then drain the opener without holding that mutex.
		s.openMu.Lock()
		s.mu.Lock()
		db := s.db
		s.mu.Unlock()
		s.closeRangeReaders()
		abortAccountAttachments(account)
		if db != nil {
			_ = db.Close()
		}
		s.openMu.Unlock()
	}
}

// The native vault, rather than an SDK wrapper, owns its working connection.
// Ownership remains meaningful after lease expiry; Lookup must not decide
// whether a handle is safe to close. SQL Close runs outside all owner locks.
func CloseSDKWorkingDatabase(account string, handle *sql.DB) error {
	if handle == nil {
		return nil
	}
	registry.Lock()
	for owner, s := range registry.stores {
		s.mu.Lock()
		owned := s.db == handle
		s.mu.Unlock()
		if owned {
			registry.Unlock()
			if owner != account {
				return ErrLocked
			}
			return nil
		}
	}
	registry.Unlock()
	return handle.Close()
}

func (s *Store) Open(file string) (*sql.DB, error) {
	s.openMu.Lock()
	defer s.openMu.Unlock()
	if filepath.Clean(file) != file || filepath.Dir(file) != s.root {
		return nil, ErrLocked
	}
	s.mu.Lock()
	if !s.authorizedLocked() {
		s.mu.Unlock()
		return nil, ErrLocked
	}
	if s.db != nil {
		db, samePath := s.db, s.dbPath == file
		s.mu.Unlock()
		if !samePath {
			return nil, ErrLocked
		}
		return db, nil
	}
	s.mu.Unlock()
	if err := os.MkdirAll(s.root, 0700); err != nil {
		return nil, err
	}
	// Never silently open/overwrite a historical plaintext DB. Migration must
	// export it through an explicitly authorized encrypted conversion first.
	if f, err := os.Open(file); err == nil {
		var magic [16]byte
		_, _ = f.Read(magic[:])
		_ = f.Close()
		if string(magic[:]) == "SQLite format 3\x00" {
			return nil, errors.New("plaintext local database requires encrypted migration")
		}
	}
	db, err := sql.Open(s.driver, file+"?_busy_timeout=10000&_secure_delete=on&_synchronous=FULL&_journal_mode=WAL&_recursive_triggers=on")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	fail := func(err error) (*sql.DB, error) { _ = db.Close(); return nil, err }
	var version string
	if err = db.QueryRow("PRAGMA cipher_version").Scan(&version); err != nil || version != "4.19.0 community" {
		return fail(errors.New("required SQLCipher runtime is unavailable"))
	}
	if _, err = db.Exec("PRAGMA temp_store=MEMORY; PRAGMA cipher_memory_security=ON;"); err != nil {
		return fail(err)
	}
	if _, err = db.Exec(schema); err != nil {
		return fail(err)
	}
	// No scratch descriptor survives process/login restart. Previous interrupted
	// media is encrypted, and is discarded before exposing this working handle.
	if _, err = db.Exec(`DELETE FROM puty_scratch_pages; DELETE FROM puty_scratch_objects;`); err != nil {
		return fail(err)
	}
	s.mu.Lock()
	if !s.authorizedLocked() {
		s.mu.Unlock()
		return fail(ErrLocked)
	}
	s.db = db
	s.dbPath = file
	s.mu.Unlock()
	return db, nil
}

func (s *Store) OpenWorkingDatabase() (*sql.DB, error) {
	return s.Open(filepath.Join(s.root, "working.db"))
}

const schema = `
CREATE TABLE IF NOT EXISTS puty_archive_outbox(id INTEGER PRIMARY KEY AUTOINCREMENT,conversation TEXT NOT NULL,message_id TEXT NOT NULL,send_time INTEGER NOT NULL,operation INTEGER NOT NULL,payload TEXT NOT NULL DEFAULT '');
CREATE INDEX IF NOT EXISTS puty_archive_pending ON puty_archive_outbox(conversation,message_id,id);
CREATE TABLE IF NOT EXISTS puty_archive_index(conversation TEXT NOT NULL,message_id TEXT NOT NULL,send_time INTEGER NOT NULL,month TEXT NOT NULL,path TEXT NOT NULL,event_id INTEGER NOT NULL,PRIMARY KEY(conversation,message_id));
CREATE INDEX IF NOT EXISTS puty_archive_page ON puty_archive_index(conversation,send_time,message_id);
CREATE TABLE IF NOT EXISTS puty_local_tombstones(conversation TEXT NOT NULL,from_time INTEGER NOT NULL,to_time INTEGER NOT NULL,PRIMARY KEY(conversation,from_time,to_time));
CREATE TABLE IF NOT EXISTS puty_local_deleted(conversation TEXT NOT NULL,message_id TEXT NOT NULL,send_time INTEGER NOT NULL,PRIMARY KEY(conversation,message_id));
CREATE TABLE IF NOT EXISTS puty_local_ephemeral(conversation TEXT NOT NULL,message_id TEXT NOT NULL,send_time INTEGER NOT NULL,PRIMARY KEY(conversation,message_id));
CREATE TABLE IF NOT EXISTS puty_legacy_media(source TEXT PRIMARY KEY,protected_path TEXT NOT NULL,source_name TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS puty_storage_control(id INTEGER PRIMARY KEY CHECK(id=1),hot_eviction INTEGER NOT NULL DEFAULT 0);
INSERT OR IGNORE INTO puty_storage_control(id) VALUES(1);
CREATE TABLE IF NOT EXISTS puty_attachment_tasks(task_id TEXT PRIMARY KEY,object_id TEXT NOT NULL,source_name TEXT NOT NULL,allocated_name TEXT NOT NULL,month TEXT NOT NULL,state TEXT NOT NULL,bytes INTEGER NOT NULL DEFAULT 0,etag TEXT NOT NULL DEFAULT '',UNIQUE(month,allocated_name));
CREATE TABLE IF NOT EXISTS puty_attachment_refs(conversation TEXT NOT NULL,message_id TEXT NOT NULL,task_id TEXT NOT NULL,PRIMARY KEY(conversation,message_id,task_id));
CREATE TABLE IF NOT EXISTS puty_local_imports(local_import_id TEXT PRIMARY KEY,conversation TEXT NOT NULL,task_id TEXT NOT NULL UNIQUE,first_imported_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS puty_outgoing_sources(object_id TEXT PRIMARY KEY,path TEXT NOT NULL UNIQUE,source_name TEXT NOT NULL,size INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS puty_outgoing_refs(message_id TEXT NOT NULL,object_id TEXT NOT NULL,PRIMARY KEY(message_id,object_id));
CREATE TABLE IF NOT EXISTS puty_scratch_objects(object_id TEXT PRIMARY KEY,source_name TEXT NOT NULL,size INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS puty_scratch_pages(object_id TEXT NOT NULL,page_no INTEGER NOT NULL,data BLOB NOT NULL,PRIMARY KEY(object_id,page_no));`

var identifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,512}$`)

func quoteIdentifier(v string) (string, error) {
	if !identifier.MatchString(v) {
		return "", errors.New("invalid local conversation table")
	}
	return `"` + v + `"`, nil
}
func sqlLiteral(v string) string { return "'" + strings.ReplaceAll(v, "'", "''") + "'" }

// InstallConversation installs SQL triggers; the SDK write and its archive
// outbox are one transaction, including batch writes and existing update paths.
func (s *Store) InstallConversation(table, conversation string) error {
	q, err := quoteIdentifier(table)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	conv := sqlLiteral(conversation)
	h := sha256.Sum256([]byte(table))
	prefix := "puty_" + hex.EncodeToString(h[:8])
	// A privacy deletion never serializes OLD.content into an event. Pending
	// upserts for that message are removed in the very same transaction.
	deleted := `NEW.status>=4 OR NEW.content_type=2101 OR EXISTS(SELECT 1 FROM puty_local_deleted WHERE conversation=` + conv + ` AND message_id=NEW.client_msg_id) OR EXISTS(SELECT 1 FROM puty_local_tombstones WHERE conversation=` + conv + ` AND NEW.send_time BETWEEN from_time AND to_time)`
	// Burn-after-reading payload stays only in the encrypted live working table
	// until the SDK expiry/delete path removes it. It never enters a permanent
	// month archive and is therefore never resurrected from one after expiry.
	ephemeral := `CASE WHEN json_valid(NEW.attached_info) THEN COALESCE(json_extract(NEW.attached_info,'$.isPrivateChat'),0) ELSE 0 END=1`
	var conversationsPresent int
	if err = tx.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='local_conversations'`).Scan(&conversationsPresent); err != nil {
		return err
	}
	if conversationsPresent != 0 {
		ephemeral += ` OR EXISTS(SELECT 1 FROM local_conversations WHERE conversation_id=` + conv + ` AND (is_private_chat=1 OR is_msg_destruct=1))`
	}
	ephemeral = `(` + ephemeral + `) OR EXISTS(SELECT 1 FROM puty_local_ephemeral WHERE conversation=` + conv + ` AND message_id=NEW.client_msg_id)`
	jsonFields := []string{"client_msg_id", "server_msg_id", "send_id", "recv_id", "sender_platform_id", "sender_nick_name", "sender_face_url", "session_type", "msg_from", "content_type", "content", "is_read", "status", "seq", "send_time", "create_time", "attached_info", "ex", "local_ex"}
	var pairs []string
	for _, field := range jsonFields {
		pairs = append(pairs, sqlLiteral(field), "NEW."+field)
	}
	payload := `json_object(` + strings.Join(pairs, ",") + `)`
	cold := `NEW.content='' AND EXISTS(SELECT 1 FROM puty_archive_index WHERE conversation=` + conv + ` AND message_id=NEW.client_msg_id)`
	for _, action := range []string{"insert", "update", "delete"} {
		if _, err = tx.Exec(`DROP TRIGGER IF EXISTS ` + prefix + "_" + action); err != nil {
			return err
		}
	}
	for _, action := range []string{"INSERT", "UPDATE"} {
		name := prefix + "_" + strings.ToLower(action)
		statement := `CREATE TRIGGER IF NOT EXISTS ` + name + ` AFTER ` + action + ` ON ` + q + ` WHEN (SELECT hot_eviction FROM puty_storage_control WHERE id=1)=0 BEGIN
DELETE FROM puty_archive_outbox WHERE conversation=` + conv + ` AND message_id=NEW.client_msg_id;
INSERT OR IGNORE INTO puty_local_deleted(conversation,message_id,send_time) SELECT ` + conv + `,NEW.client_msg_id,NEW.send_time WHERE ` + deleted + `;
INSERT OR IGNORE INTO puty_local_ephemeral(conversation,message_id,send_time) SELECT ` + conv + `,NEW.client_msg_id,NEW.send_time WHERE ` + ephemeral + `;
INSERT INTO puty_archive_outbox(conversation,message_id,send_time,operation,payload) VALUES(` + conv + `,NEW.client_msg_id,NEW.send_time,CASE WHEN (` + deleted + `) OR (` + ephemeral + `) THEN 2 WHEN (` + cold + `) THEN 3 ELSE 1 END,CASE WHEN (` + deleted + `) OR (` + ephemeral + `) THEN '' ELSE ` + payload + ` END);
UPDATE ` + q + ` SET content='',attached_info='',ex='',local_ex='',sender_nick_name='',sender_face_url='',status=4 WHERE client_msg_id=NEW.client_msg_id AND (` + deleted + `) AND (content<>'' OR attached_info<>'' OR ex<>'' OR local_ex<>'' OR sender_nick_name<>'' OR sender_face_url<>'' OR status<>4);
END;`
		if _, err = tx.Exec(statement); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`CREATE TRIGGER IF NOT EXISTS ` + prefix + `_delete AFTER DELETE ON ` + q + ` WHEN (SELECT hot_eviction FROM puty_storage_control WHERE id=1)=0 BEGIN
DELETE FROM puty_archive_outbox WHERE conversation=` + conv + ` AND message_id=OLD.client_msg_id;
INSERT OR IGNORE INTO puty_local_deleted(conversation,message_id,send_time) VALUES(` + conv + `,OLD.client_msg_id,OLD.send_time);
INSERT INTO puty_archive_outbox(conversation,message_id,send_time,operation,payload) VALUES(` + conv + `,OLD.client_msg_id,OLD.send_time,2,'');END;`)
	if err != nil {
		return err
	}
	backfill := `INSERT INTO puty_archive_outbox(conversation,message_id,send_time,operation,payload) SELECT ` + conv + `,m.client_msg_id,m.send_time,1,` + strings.ReplaceAll(payload, "NEW.", "m.") + ` FROM ` + q + ` m WHERE m.status<4 AND m.content_type<>2101 AND NOT (` + strings.ReplaceAll(ephemeral, "NEW.", "m.") + `) AND m.content<>'' AND NOT EXISTS(SELECT 1 FROM puty_archive_index i WHERE i.conversation=` + conv + ` AND i.message_id=m.client_msg_id) AND NOT EXISTS(SELECT 1 FROM puty_archive_outbox o WHERE o.conversation=` + conv + ` AND o.message_id=m.client_msg_id) AND NOT EXISTS(SELECT 1 FROM puty_local_deleted d WHERE d.conversation=` + conv + ` AND d.message_id=m.client_msg_id) AND NOT EXISTS(SELECT 1 FROM puty_local_tombstones t WHERE t.conversation=` + conv + ` AND m.send_time BETWEEN t.from_time AND t.to_time)`
	if _, err = tx.Exec(backfill); err != nil {
		return err
	}
	// Historical status-4/revoke rows may still contain a body in the old DB.
	// Installing a vault never carries that deleted body into a new archive.
	if _, err = tx.Exec(`UPDATE ` + q + ` SET content='',attached_info='',ex='',local_ex='',sender_nick_name='',sender_face_url='',status=4 WHERE (status>=4 OR content_type=2101 OR EXISTS(SELECT 1 FROM puty_local_deleted d WHERE d.conversation=` + conv + ` AND d.message_id=` + q + `.client_msg_id)) AND (content<>'' OR attached_info<>'' OR ex<>'' OR local_ex<>'' OR sender_nick_name<>'' OR sender_face_url<>'' OR status<>4)`); err != nil {
		return err
	}

	if conversationsPresent != 0 {
		// Changing an existing conversation to private/destruct removes its old
		// permanent copies too. It does not acknowledge/delete live pending media.
		body := `DELETE FROM puty_archive_outbox WHERE conversation=` + conv + `;
INSERT OR IGNORE INTO puty_local_ephemeral SELECT ` + conv + `,client_msg_id,send_time FROM ` + q + `;
INSERT OR IGNORE INTO puty_local_ephemeral SELECT conversation,message_id,send_time FROM puty_archive_index WHERE conversation=` + conv + `;
INSERT INTO puty_archive_outbox(conversation,message_id,send_time,operation,payload) SELECT conversation,message_id,send_time,2,'' FROM puty_local_ephemeral WHERE conversation=` + conv + `;`
		if _, err = tx.Exec(`DROP TRIGGER IF EXISTS ` + prefix + `_privacy`); err != nil {
			return err
		}
		if _, err = tx.Exec(`CREATE TRIGGER ` + prefix + `_privacy AFTER UPDATE OF is_private_chat,is_msg_destruct ON local_conversations WHEN NEW.conversation_id=` + conv + ` AND (NEW.is_private_chat=1 OR NEW.is_msg_destruct=1) BEGIN ` + body + ` END;`); err != nil {
			return err
		}
		var private int
		if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM local_conversations WHERE conversation_id=? AND (is_private_chat=1 OR is_msg_destruct=1))`, conversation).Scan(&private); err != nil {
			return err
		}
		if private != 0 {
			if _, err = tx.Exec(body); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

type Event struct {
	ID                      int64
	Conversation, MessageID string
	SendTime                int64
	Operation               int
	Payload                 string
}

func recordID(conv, id string) string {
	h := sha256.Sum256([]byte(conv + "\x00" + id))
	return hex.EncodeToString(h[:])
}

// Flush is idempotent after every crash point: encrypted file fsync+rename and
// directory fsync precede the index/outbox ACK. A newer outbox row is untouched.
func (s *Store) Flush(limit int) error {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	if limit < 1 || limit > 1000 {
		return errors.New("invalid archive batch size")
	}
	s.mu.Lock()
	if !s.authorizedLocked() {
		s.mu.Unlock()
		return ErrLocked
	}
	key := bytes.Clone(s.master)
	s.mu.Unlock()
	defer clear(key)
	rows, err := s.db.Query(`SELECT id,conversation,message_id,send_time,operation,payload FROM puty_archive_outbox ORDER BY id LIMIT ?`, limit)
	if err != nil {
		return err
	}
	var events []Event
	for rows.Next() {
		var e Event
		if err = rows.Scan(&e.ID, &e.Conversation, &e.MessageID, &e.SendTime, &e.Operation, &e.Payload); err != nil {
			_ = rows.Close()
			return err
		}
		events = append(events, e)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	deletedAny := false
	for _, e := range events {
		id := recordID(e.Conversation, e.MessageID)
		month := time.UnixMilli(e.SendTime).In(CompanyZone).Format("200601")
		relative := filepath.Join(month, "messages", id)
		path := filepath.Join(s.root, "archive", relative)
		if e.Operation == 3 {
			original, readErr := s.RecordPayload(e.Conversation, e.MessageID)
			if readErr != nil {
				return readErr
			}
			if original == nil {
				e.Operation = 2
				e.Payload = ""
			} else {
				var patch map[string]json.RawMessage
				if err = json.Unmarshal([]byte(e.Payload), &patch); err != nil {
					return err
				}
				for _, field := range []string{"content", "attached_info", "ex", "local_ex"} {
					if bytes.Equal(patch[field], []byte(`""`)) {
						patch[field] = original[field]
					}
				}
				encoded, marshalErr := json.Marshal(patch)
				if marshalErr != nil {
					return marshalErr
				}
				e.Payload = string(encoded)
			}
		}
		if e.Operation == 2 {
			deletedAny = true
			var previous string
			_ = s.db.QueryRow(`SELECT path FROM puty_archive_index WHERE conversation=? AND message_id=?`, e.Conversation, e.MessageID).Scan(&previous)
			if previous != "" {
				if err = removeOwnedArchive(s.root, previous); err != nil {
					return err
				}
			}
			// A retry after a previous write-before-ACK must remove that same file.
			if err = removeOwnedArchive(s.root, relative); err != nil {
				return err
			}
			// Operation 2 also means "live ephemeral, never archive". Only an
			// actual local deletion may release a pending producer/source. Keep
			// the durable event until every exact reference/file cleanup succeeds.
			var deleted int
			if err = s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM puty_local_deleted WHERE conversation=? AND message_id=?)`, e.Conversation, e.MessageID).Scan(&deleted); err != nil {
				return err
			}
			if deleted != 0 {
				if err = s.clearMessageAttachmentReferences(e.Conversation, e.MessageID); err != nil {
					return err
				}
				if err = s.ReleaseOutgoing(e.MessageID); err != nil {
					return err
				}
			}
		} else {
			if err = atomicEncrypted(path, []byte(e.Payload), key, localstore.ObjectIdentity{Account: s.account, Type: "message", ObjectID: id}); err != nil {
				return err
			}
			var previous string
			_ = s.db.QueryRow(`SELECT path FROM puty_archive_index WHERE conversation=? AND message_id=?`, e.Conversation, e.MessageID).Scan(&previous)
			if previous != "" && previous != relative {
				if err = removeOwnedArchive(s.root, previous); err != nil {
					return err
				}
			}
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if e.Operation == 2 {
			_, err = tx.Exec(`DELETE FROM puty_archive_index WHERE conversation=? AND message_id=?`, e.Conversation, e.MessageID)
			if err == nil {
				_, err = tx.Exec(`DELETE FROM puty_local_imports WHERE conversation=? AND local_import_id=?`, e.Conversation, e.MessageID)
			}
		} else {
			_, err = tx.Exec(`INSERT INTO puty_archive_index(conversation,message_id,send_time,month,path,event_id) VALUES(?,?,?,?,?,?) ON CONFLICT(conversation,message_id) DO UPDATE SET send_time=excluded.send_time,month=excluded.month,path=excluded.path,event_id=excluded.event_id`, e.Conversation, e.MessageID, e.SendTime, month, relative, e.ID)
		}
		if err == nil {
			_, err = tx.Exec(`DELETE FROM puty_archive_outbox WHERE id=?`, e.ID)
		}
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	if deletedAny {
		if _, err = s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
			return err
		}
	}
	return nil
}

// RecordPayload hydrates one cold thin row without reinserting it into the hot
// working table or generating a new archive event.
func (s *Store) RecordPayload(conversation, messageID string) (map[string]json.RawMessage, error) {
	s.mu.Lock()
	if !s.authorizedLocked() {
		s.mu.Unlock()
		return nil, ErrLocked
	}
	key := bytes.Clone(s.master)
	s.mu.Unlock()
	defer clear(key)
	var path string
	err := s.db.QueryRow(`SELECT path FROM puty_archive_index i WHERE conversation=? AND message_id=? AND NOT EXISTS(SELECT 1 FROM puty_local_deleted d WHERE d.conversation=i.conversation AND d.message_id=i.message_id) AND NOT EXISTS(SELECT 1 FROM puty_local_tombstones t WHERE t.conversation=i.conversation AND i.send_time BETWEEN t.from_time AND t.to_time) AND NOT EXISTS(SELECT 1 FROM puty_local_ephemeral e WHERE e.conversation=i.conversation AND e.message_id=i.message_id)`, conversation, messageID).Scan(&path)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(s.root, "archive", path))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out bytes.Buffer
	if err = localstore.Decrypt(&out, io.LimitReader(f, 16<<20), key, localstore.ObjectIdentity{Account: s.account, Type: "message", ObjectID: recordID(conversation, messageID)}); err != nil {
		return nil, err
	}
	var result map[string]json.RawMessage
	err = json.Unmarshal(out.Bytes(), &result)
	clear(out.Bytes())
	if s.checkAuthorized() != nil {
		for k, value := range result {
			clear(value)
			delete(result, k)
		}
		return nil, ErrLocked
	}
	return result, err
}

// Suppressed is also checked on the SDK callback path: SQL sanitization alone
// cannot protect an incoming Go message pointer serialized before a DB re-read.
func (s *Store) Suppressed(conversation, messageID string, sentAt int64) bool {
	if s.checkAuthorized() != nil {
		return true
	}
	var blocked int
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM puty_local_deleted WHERE conversation=? AND message_id=?) OR EXISTS(SELECT 1 FROM puty_local_tombstones WHERE conversation=? AND ? BETWEEN from_time AND to_time)`, conversation, messageID, conversation, sentAt).Scan(&blocked)
	return err != nil || blocked != 0
}

func atomicEncrypted(path string, payload, key []byte, id localstore.ObjectIdentity) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(directory, ".pending-")
	if err != nil {
		return err
	}
	name := f.Name()
	committed := false
	defer func() {
		_ = f.Close()
		if !committed {
			_ = os.Remove(name)
		}
	}()
	if err = localstore.Encrypt(f, bytes.NewReader(payload), key, id); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	if err = syncDirectory(directory); err != nil {
		return err
	}
	committed = true
	return nil
}
func syncDirectory(directory string) error {
	f, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func removeOwnedArchive(root, relative string) error {
	clean := filepath.Clean(relative)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errors.New("invalid archive path")
	}
	path := filepath.Join(root, "archive", clean)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(filepath.Dir(path)); os.IsNotExist(err) {
		return nil
	}
	return syncDirectory(filepath.Dir(path))
}

// Page reads only a bounded page from authenticated monthly records. SQLCipher
// contains the encrypted metadata index, never an unbounded in-memory history.
func (s *Store) Page(conversation string, before int64, limit int) ([]json.RawMessage, error) {
	return s.PageCursor(conversation, before, "", limit)
}
func (s *Store) PageCursor(conversation string, before int64, beforeID string, limit int) ([]json.RawMessage, error) {
	if limit < 1 || limit > 100 {
		return nil, errors.New("invalid archive page size")
	}
	s.mu.Lock()
	if !s.authorizedLocked() || !s.viewsReady {
		s.mu.Unlock()
		return nil, ErrLocked
	}
	key := bytes.Clone(s.master)
	s.mu.Unlock()
	defer clear(key)
	rows, err := s.db.Query(`SELECT message_id,path FROM puty_archive_index i WHERE conversation=? AND (send_time<? OR(send_time=? AND message_id<?)) AND NOT EXISTS(SELECT 1 FROM puty_local_deleted d WHERE d.conversation=i.conversation AND d.message_id=i.message_id) AND NOT EXISTS(SELECT 1 FROM puty_local_tombstones t WHERE t.conversation=i.conversation AND i.send_time BETWEEN t.from_time AND t.to_time) AND NOT EXISTS(SELECT 1 FROM puty_local_ephemeral e WHERE e.conversation=i.conversation AND e.message_id=i.message_id) ORDER BY send_time DESC,message_id DESC LIMIT ?`, conversation, before, before, beforeID, limit)
	if err != nil {
		return nil, err
	}
	type item struct{ id, path string }
	var items []item
	for rows.Next() {
		var i item
		if err = rows.Scan(&i.id, &i.path); err != nil {
			_ = rows.Close()
			return nil, err
		}
		items = append(items, i)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	result := make([]json.RawMessage, 0, len(items))
	for _, i := range items {
		f, err := os.Open(filepath.Join(s.root, "archive", i.path))
		if err != nil {
			return nil, err
		}
		var out bytes.Buffer
		err = localstore.Decrypt(&out, io.LimitReader(f, 16<<20), key, localstore.ObjectIdentity{Account: s.account, Type: "message", ObjectID: recordID(conversation, i.id)})
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		if !json.Valid(out.Bytes()) {
			return nil, errors.New("invalid authenticated archive record")
		}
		result = append(result, bytes.Clone(out.Bytes()))
		clear(out.Bytes())
	}
	s.mu.Lock()
	ready := s.authorizedLocked() && s.viewsReady
	s.mu.Unlock()
	if !ready {
		for _, payload := range result {
			clear(payload)
		}
		return nil, ErrLocked
	}
	return result, nil
}

// ClearRange affects this account on this installation only. A synchronized
// reinsert retains a thin status-4/seq row; its original payload is never queued.
func (s *Store) ClearRange(table, conversation string, from, to int64) error {
	return s.clearRangeWithMask(table, conversation, from, to, confirmedClearUpper(to))
}

// Only authenticated restore state may replay an existing historical mask.
// New UI clears continue to cap suppression at their confirmation time.
func (s *Store) replayHistoricalClear(table, conversation string, from, to int64) error {
	return s.clearRangeWithMask(table, conversation, from, to, to)
}

func (s *Store) clearRangeWithMask(table, conversation string, from, to, maskTo int64) error {
	if from < 0 || to < from || conversation == "" {
		return errors.New("invalid local clear interval")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.stageClearRange(tx, table, conversation, from, to, maskTo); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if err = s.finishClearRange(conversation, from, to); err != nil {
		return err
	}
	_, err = s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return err
}

// EvictHot preserves unread, unsent and every row's sequence metadata. Only
// already durably archived rows may lose their hot payload; archive records stay.
func (s *Store) EvictHot(table, conversation string, cutoff int64) error {
	q, err := quoteIdentifier(table)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE puty_storage_control SET hot_eviction=1 WHERE id=1`); err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE `+q+` SET content='',attached_info='',ex='',local_ex='' WHERE send_time<? AND is_read=1 AND status=2 AND seq>0 AND EXISTS(SELECT 1 FROM puty_archive_index i WHERE i.conversation=? AND i.message_id=`+q+`.client_msg_id) AND NOT EXISTS(SELECT 1 FROM puty_archive_outbox o WHERE o.conversation=? AND o.message_id=`+q+`.client_msg_id)`, cutoff, conversation, conversation)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE puty_storage_control SET hot_eviction=0 WHERE id=1`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DB() *sql.DB { return s.db }
func (s *Store) String() string {
	return fmt.Sprintf("PUTY encrypted native store (locked=%t)", func() bool { s.mu.Lock(); defer s.mu.Unlock(); return !s.authorizedLocked() }())
}
