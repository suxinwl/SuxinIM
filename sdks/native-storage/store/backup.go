package nativestore

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
	localstore "puty.local/localstore"
)

type BackupFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type BackupManifest struct {
	Account   string       `json:"account"`
	CreatedAt int64        `json:"createdAt"`
	Files     []BackupFile `json:"files"`
	Format    string       `json:"format"`
	Platform  string       `json:"platform"`
}

func (s *Store) checkAuthorized() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.authorizedLocked() {
		return ErrLocked
	}
	return nil
}
func randomName() (string, error) {
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(random[:]), nil
}

func (s *Store) snapshot(path string, key []byte) error {
	connection, err := s.db.Conn(context.Background())
	if err != nil {
		return err
	}
	defer connection.Close()
	if _, err = connection.ExecContext(context.Background(), `ATTACH DATABASE ? AS protected`, path); err != nil {
		return err
	}
	defer connection.ExecContext(context.Background(), `DETACH DATABASE protected`)
	if err = connection.Raw(func(value any) error { return value.(*sqlite3.SQLiteConn).SetAttachedRawKey("protected", key) }); err != nil {
		return err
	}
	if _, err = connection.ExecContext(context.Background(), `SELECT sqlcipher_export('protected')`); err != nil {
		return err
	}
	if _, err = connection.ExecContext(context.Background(), `DETACH DATABASE protected`); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func (s *Store) openProtectedSnapshot(path string, key []byte) (*sql.DB, error) {
	nonce, err := randomName()
	if err != nil {
		return nil, err
	}
	driver := "puty_snapshot_" + nonce
	sql.Register(driver, &sqlite3.SQLiteDriver{KeyHook: func(c *sqlite3.SQLiteConn) error {
		if err := s.checkAuthorized(); err != nil {
			return err
		}
		if err := c.SetRawKey(key); err != nil {
			return err
		}
		c.AccessCheck = s.checkAuthorized
		return nil
	}})
	db, err := sql.Open(driver, path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// ExportBackup captures one consistent SQLCipher snapshot plus encrypted data
// files. ZIP never contains plaintext working DB/message/attachment content.
// The AEAD manifest authenticates the exact relative paths, sizes and hashes.
func (s *Store) ExportBackup(passphrase string) (string, error) {
	if err := s.checkAuthorized(); err != nil {
		return "", err
	}
	s.mu.Lock()
	ready := s.viewsReady
	s.mu.Unlock()
	if !ready {
		return "", ErrLocked
	}
	// Commit queued archive mutations before the snapshot, then freeze the two
	// file mutation paths. SDK transactions after the snapshot remain normal.
	// Remaining outbox rows are themselves in the consistent SQLCipher snapshot
	// and replay normally on restore; continuous traffic cannot starve backup.
	if err := s.Flush(1000); err != nil {
		return "", err
	}
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	attachmentGuard.Lock()
	defer attachmentGuard.Unlock()
	// ACK/discard must not unlink a pending source captured by the snapshot.
	// Keep lock order flush -> attachment -> outgoing, matching cleanup paths.
	outgoingGuard.Lock()
	defer outgoingGuard.Unlock()
	s.mu.Lock()
	if !s.authorizedLocked() {
		s.mu.Unlock()
		return "", ErrLocked
	}
	master := bytes.Clone(s.master)
	s.mu.Unlock()
	defer clear(master)
	recovery, err := localstore.ExportRecovery(master, s.account, passphrase)
	if err != nil {
		return "", err
	}
	nonce, err := randomName()
	if err != nil {
		return "", err
	}
	directory := filepath.Join(s.root, "backups")
	if err = os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	snapshot := filepath.Join(directory, ".snapshot-"+nonce)
	defer os.Remove(snapshot)
	if err = s.snapshot(snapshot, master); err != nil {
		return "", err
	}
	paths := map[string]string{"working.db": snapshot}
	// Only SQLCipher-owned references enter the backup; crash remnants and
	// uncommitted .pending/.resume files cannot expand its scope.
	// Read dependencies from the same frozen snapshot, never newer live rows.
	snapshotDB, err := s.openProtectedSnapshot(snapshot, master)
	if err != nil {
		return "", err
	}
	defer snapshotDB.Close()
	rows, err := snapshotDB.Query(`SELECT 'archive/'||path FROM puty_archive_index UNION SELECT 'archive/'||month||'/attachments/'||allocated_name FROM puty_attachment_tasks UNION SELECT 'outgoing/'||path FROM puty_outgoing_sources`)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var relative string
		if err = rows.Scan(&relative); err != nil {
			break
		}
		if !validBackupPath(relative) || len(paths) >= 200000 {
			err = errors.New("invalid indexed backup path")
			break
		}
		path := filepath.Join(s.root, filepath.FromSlash(relative))
		var info os.FileInfo
		info, err = os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			if err == nil {
				err = errors.New("backup refuses non-regular files")
			}
			break
		}
		paths[relative] = path
	}
	if err == nil {
		err = rows.Err()
	}
	_ = rows.Close()
	if err != nil {
		return "", err
	}
	var names []string
	for name := range paths {
		names = append(names, name)
	}
	sort.Strings(names)
	manifest := BackupManifest{Account: s.account, CreatedAt: time.Now().Unix(), Format: localstore.Format, Platform: "native-sqlcipher", Files: make([]BackupFile, 0, len(names))}
	output := filepath.Join(directory, "PUTY_IM-"+time.Now().In(CompanyZone).Format("20060102-150405")+"-"+nonce+".backup")
	f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	committed := false
	defer func() {
		_ = f.Close()
		if !committed {
			_ = os.Remove(output)
		}
	}()
	writer := zip.NewWriter(f)
	writeEntry := func(name string, source io.Reader) (int64, string, error) {
		header := &zip.FileHeader{Name: name, Method: zip.Store}
		header.SetMode(0600)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return 0, "", err
		}
		hash := sha256.New()
		count, err := copyAuthorized(io.MultiWriter(entry, hash), source, s.checkAuthorized)
		return count, hex.EncodeToString(hash.Sum(nil)), err
	}
	if _, _, err = writeEntry("recovery.json", bytes.NewReader(recovery)); err != nil {
		return "", err
	}
	for _, name := range names {
		input, err := os.Open(paths[name])
		if err != nil {
			return "", err
		}
		size, digest, copyErr := writeEntry("data/"+name, input)
		_ = input.Close()
		if copyErr != nil {
			return "", copyErr
		}
		manifest.Files = append(manifest.Files, BackupFile{Path: name, SHA256: digest, Size: size})
	}
	plain, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	defer clear(plain)
	var sealed bytes.Buffer
	if err = localstore.Encrypt(&sealed, bytes.NewReader(plain), master, localstore.ObjectIdentity{Account: s.account, Type: "backup-manifest", ObjectID: "manifest"}); err != nil {
		return "", err
	}
	if _, _, err = writeEntry("manifest.puty", bytes.NewReader(sealed.Bytes())); err != nil {
		return "", err
	}
	if err = writer.Close(); err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	if err = syncDirectory(directory); err != nil {
		return "", err
	}
	if err = s.checkAuthorized(); err != nil {
		return "", err
	}
	committed = true
	return output, nil
}

func copyAuthorized(destination io.Writer, source io.Reader, authorized func() error) (int64, error) {
	buffer := make([]byte, 65536)
	defer clear(buffer)
	var total int64
	for {
		if err := authorized(); err != nil {
			return total, err
		}
		n, readErr := source.Read(buffer)
		if n > 0 {
			written, err := destination.Write(buffer[:n])
			total += int64(written)
			if err != nil {
				return total, err
			}
			if written != n {
				return total, io.ErrShortWrite
			}
		}
		if readErr == io.EOF {
			return total, nil
		}
		if readErr != nil {
			return total, readErr
		}
	}
}
func validBackupPath(path string) bool {
	if path == "working.db" {
		return true
	}
	if path == "" || strings.ContainsAny(path, "\\\x00") || filepath.IsAbs(path) || filepath.ToSlash(filepath.Clean(path)) != path {
		return false
	}
	parts := strings.Split(path, "/")
	if len(parts) == 2 && parts[0] == "outgoing" {
		base := strings.TrimSuffix(parts[1], filepath.Ext(parts[1]))
		return len(base) == 24 && strings.Trim(base, "0123456789abcdef") == "" && localstore.ValidateSourceName(parts[1]) == nil
	}
	if len(parts) != 4 || parts[0] != "archive" || len(parts[1]) != 6 || parts[1] < "197001" || parts[1] > "999912" {
		return false
	}
	for _, c := range parts[1] {
		if c < '0' || c > '9' {
			return false
		}
	}
	if parts[1][4:] < "01" || parts[1][4:] > "12" {
		return false
	}
	if parts[2] == "messages" {
		return len(parts[3]) == 64 && strings.Trim(parts[3], "0123456789abcdef") == ""
	}
	return parts[2] == "attachments" && localstore.ValidateSourceName(parts[3]) == nil
}

// PreparedRestore holds the recovered master only inside native code. Native
// Java/Swift wrap it with their OS key, then Commit promotes the staged vault.
type PreparedRestore struct {
	owner        *Store
	master       []byte
	stage        string
	closed       bool
	mu           sync.Mutex
	cancelled    atomic.Bool
	commitReady  bool
	wrappedHash  [32]byte
	commitRecord promotion
	preparedDB   *sql.DB
}

func (p *PreparedRestore) MasterKey() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.owner.checkAuthorized() != nil {
		return nil
	}
	return bytes.Clone(p.master)
}
func (p *PreparedRestore) Abort() {
	p.cancelled.Store(true)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	clear(p.master)
	_ = removeOwnedStaging(p.owner.root, p.stage)
}

func (s *Store) PrepareRestore(path, passphrase string) (*PreparedRestore, error) {
	if err := s.checkAuthorized(); err != nil {
		return nil, err
	}
	var reader *zip.Reader
	var err error
	if encrypted, openErr := OpenOutgoing(path); openErr == nil {
		defer encrypted.Close()
		reader, err = zip.NewReader(encrypted, encrypted.Size())
	} else {
		file, openErr := os.Open(path)
		if openErr != nil {
			return nil, openErr
		}
		defer file.Close()
		info, statErr := file.Stat()
		if statErr != nil || !info.Mode().IsRegular() {
			return nil, errors.New("invalid backup source")
		}
		reader, err = zip.NewReader(file, info.Size())
	}
	if err != nil {
		return nil, err
	}
	return s.prepareRestoreZIP(reader, passphrase)
}

// Native SAF/security-scoped descriptors stay outside application storage.
// The caller retains ownership and must not expose this method to Dart.
func (s *Store) PrepareRestoreReader(input io.ReaderAt, size int64, passphrase string) (*PreparedRestore, error) {
	if s.checkAuthorized() != nil || size < 1 || size > 64<<30 {
		return nil, ErrLocked
	}
	reader, err := zip.NewReader(input, size)
	if err != nil {
		return nil, err
	}
	return s.prepareRestoreZIP(reader, passphrase)
}
func (s *Store) prepareRestoreZIP(reader *zip.Reader, passphrase string) (*PreparedRestore, error) {
	if _, err := os.Lstat(filepath.Join(s.root, ".restore-gc")); err == nil {
		return nil, errors.New("fresh synchronization and prior restore GC are required")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	var err error
	if len(reader.File) < 3 || len(reader.File) > 200002 {
		return nil, errors.New("invalid backup entry count")
	}
	entries := make(map[string]*zip.File, len(reader.File))
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() || entry.Mode()&os.ModeSymlink != 0 || entry.UncompressedSize64 > 1<<30 || entries[entry.Name] != nil {
			return nil, errors.New("invalid backup entry")
		}
		if entry.Name != "recovery.json" && entry.Name != "manifest.puty" && (!strings.HasPrefix(entry.Name, "data/") || !validBackupPath(strings.TrimPrefix(entry.Name, "data/"))) {
			return nil, errors.New("unknown backup entry")
		}
		entries[entry.Name] = entry
	}
	readSmall := func(name string, max int64) ([]byte, error) {
		entry := entries[name]
		if entry == nil || entry.UncompressedSize64 > uint64(max) {
			return nil, errors.New("missing backup metadata")
		}
		input, err := entry.Open()
		if err != nil {
			return nil, err
		}
		defer input.Close()
		return io.ReadAll(io.LimitReader(input, max+1))
	}
	recovery, err := readSmall("recovery.json", 65536)
	if err != nil {
		return nil, err
	}
	master, err := localstore.ImportRecovery(recovery, s.account, passphrase)
	if err != nil {
		return nil, err
	}
	keepMaster := false
	defer func() {
		if !keepMaster {
			clear(master)
		}
	}()
	sealed, err := readSmall("manifest.puty", 64<<20)
	if err != nil {
		return nil, err
	}
	var plain bytes.Buffer
	if err = localstore.Decrypt(&plain, bytes.NewReader(sealed), master, localstore.ObjectIdentity{Account: s.account, Type: "backup-manifest", ObjectID: "manifest"}); err != nil {
		return nil, err
	}
	defer clear(plain.Bytes())
	var manifest BackupManifest
	decoder := json.NewDecoder(&plain)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&manifest); err != nil {
		return nil, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("trailing backup manifest data")
	}
	if manifest.Format != localstore.Format || manifest.Account != s.account || manifest.Platform != "native-sqlcipher" || len(manifest.Files) != len(entries)-2 {
		return nil, errors.New("backup manifest identity mismatch")
	}
	nonce, err := randomName()
	if err != nil {
		return nil, err
	}
	stage := s.root + ".restore-" + nonce
	if err = os.Mkdir(stage, 0700); err != nil {
		return nil, err
	}
	prepared := &PreparedRestore{owner: s, stage: stage, master: master}
	keepStage := false
	defer func() {
		if !keepStage {
			_ = removeOwnedStaging(s.root, stage)
		}
	}()
	seen := map[string]bool{}
	var total int64
	for _, file := range manifest.Files {
		if !validBackupPath(file.Path) || seen[file.Path] || file.Size < 0 || file.Size > 1<<30 || len(file.SHA256) != 64 {
			return nil, errors.New("invalid backup manifest path")
		}
		seen[file.Path] = true
		entry := entries["data/"+file.Path]
		if entry == nil || entry.UncompressedSize64 != uint64(file.Size) {
			return nil, errors.New("backup file size mismatch")
		}
		total += file.Size
		if total > 64<<30 {
			return nil, errors.New("backup capacity limit exceeded")
		}
		target := filepath.Join(stage, filepath.FromSlash(file.Path))
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return nil, err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}
		input, err := entry.Open()
		if err != nil {
			_ = output.Close()
			return nil, err
		}
		hash := sha256.New()
		count, copyErr := copyAuthorized(io.MultiWriter(output, hash), input, s.checkAuthorized)
		_ = input.Close()
		if copyErr == nil {
			copyErr = output.Sync()
		}
		_ = output.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if count != file.Size || hex.EncodeToString(hash.Sum(nil)) != file.SHA256 {
			return nil, errors.New("backup ciphertext hash mismatch")
		}
	}
	if !seen["working.db"] {
		return nil, errors.New("backup working database missing")
	}
	if err = prepared.verifyAndMerge(); err != nil {
		return nil, err
	}
	if err = syncDirectory(stage); err != nil {
		return nil, err
	}
	keepMaster = true
	keepStage = true
	return prepared, nil
}

func removeOwnedStaging(root, stage string) error {
	if filepath.Dir(root) != filepath.Dir(stage) || !strings.HasPrefix(filepath.Base(stage), filepath.Base(root)+".restore-") || len(strings.TrimPrefix(filepath.Base(stage), filepath.Base(root)+".restore-")) != 24 {
		return errors.New("invalid owned restore staging")
	}
	return os.RemoveAll(stage)
}

func (p *PreparedRestore) verifyAndMerge() error {
	if err := p.owner.checkAuthorized(); err != nil {
		return err
	}
	nonce, err := randomName()
	if err != nil {
		return err
	}
	driver := "puty_restore_" + nonce
	sql.Register(driver, &sqlite3.SQLiteDriver{KeyHook: func(c *sqlite3.SQLiteConn) error {
		if err := p.owner.checkAuthorized(); err != nil {
			return err
		}
		if err := c.SetRawKey(p.master); err != nil {
			return err
		}
		c.AccessCheck = p.owner.checkAuthorized
		return nil
	}})
	db, err := sql.Open(driver, filepath.Join(p.stage, "working.db")+"?_secure_delete=on&_journal_mode=WAL&_synchronous=FULL&_recursive_triggers=on")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var integrity string
	if err = db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		return errors.New("restored SQLCipher integrity failed")
	}
	if _, err = db.Exec(schema); err != nil {
		return err
	}
	// Never resurrect a locally cleared range when restoring an older backup.
	rows, err := p.owner.db.Query(`SELECT conversation,from_time,to_time FROM puty_local_tombstones`)
	if err != nil {
		return err
	}
	type tombstone struct {
		conversation string
		from, to     int64
	}
	var ranges []tombstone
	for rows.Next() {
		var t tombstone
		if err = rows.Scan(&t.conversation, &t.from, &t.to); err != nil {
			_ = rows.Close()
			return err
		}
		ranges = append(ranges, t)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	stageStore := &Store{account: p.owner.account, root: p.stage, master: p.master, db: db, parent: p.owner}
	for _, t := range ranges {
		if err = stageStore.replayHistoricalClear("chat_logs_"+t.conversation, t.conversation, t.from, t.to); err != nil {
			return err
		}
	}
	// Exact SDK deletion/expiry IDs are independent of a user's range clear.
	// Merge them too; a restoration must never revive a burned/recalled body.
	rows, err = p.owner.db.Query(`SELECT conversation,message_id,send_time FROM puty_local_deleted`)
	if err != nil {
		return err
	}
	type deletedMessage struct {
		conversation, id string
		sent             int64
	}
	var deleted []deletedMessage
	for rows.Next() {
		var d deletedMessage
		if err = rows.Scan(&d.conversation, &d.id, &d.sent); err != nil {
			_ = rows.Close()
			return err
		}
		deleted = append(deleted, d)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, d := range deleted {
		if err = stageStore.DeleteLocalMessage("chat_logs_"+d.conversation, d.conversation, d.id, d.sent); err != nil {
			return err
		}
	}
	rows, err = p.owner.db.Query(`SELECT conversation,message_id,send_time FROM puty_local_ephemeral`)
	if err != nil {
		return err
	}
	var ephemeral []deletedMessage
	for rows.Next() {
		var d deletedMessage
		if err = rows.Scan(&d.conversation, &d.id, &d.sent); err != nil {
			_ = rows.Close()
			return err
		}
		ephemeral = append(ephemeral, d)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, d := range ephemeral {
		if _, err = db.Exec(`INSERT OR IGNORE INTO puty_local_ephemeral VALUES(?,?,?)`, d.conversation, d.id, d.sent); err != nil {
			return err
		}
		if _, err = db.Exec(`DELETE FROM puty_archive_outbox WHERE conversation=? AND message_id=?`, d.conversation, d.id); err != nil {
			return err
		}
		if _, err = db.Exec(`INSERT INTO puty_archive_outbox(conversation,message_id,send_time,operation,payload)VALUES(?,?,?,2,'')`, d.conversation, d.id, d.sent); err != nil {
			return err
		}
	}
	for {
		var count int
		if err = db.QueryRow(`SELECT count(*) FROM puty_archive_outbox`).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			break
		}
		if err = stageStore.Flush(1000); err != nil {
			return err
		}
	}
	if _, err = db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return err
	}
	return p.owner.checkAuthorized()
}
