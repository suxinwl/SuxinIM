package nativestore

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	sqlite3 "github.com/mattn/go-sqlite3"
	localstore "puty.local/localstore"
)

type restoreGarbage struct {
	Previous     string `json:"previous"`
	SHA256       string `json:"sha256"`
	Phase        string `json:"phase"`
	RootDevice   string `json:"rootDevice"`
	RootInode    string `json:"rootInode"`
	LedgerSHA256 string `json:"ledgerSHA256"`
}
type gcEntry struct {
	path, kind, hash, device, inode string
	size                            int64
}

// Replayed receipts may have survived a rename whose directory fsync failed.
// Re-verify and re-sync the exact encrypted receipt and pinned ledger on every
// attempt, including an already-deleting phase, before its first unlink.
var gcDurabilityBoundary = syncVerifiedGCBoundary

func syncVerifiedGCBoundary(s *Store, receipt restoreGarbage, key []byte) error {
	root := s.root
	file, err := os.Open(filepath.Join(root, ".restore-gc"))
	if err != nil {
		return err
	}
	defer file.Close()
	var plain bytes.Buffer
	err = localstore.Decrypt(&plain, io.LimitReader(file, 65537), key, localstore.ObjectIdentity{Account: s.account, Type: "restore-gc", ObjectID: "previous"})
	if err != nil {
		return err
	}
	defer clear(plain.Bytes())
	expected, _ := json.Marshal(receipt)
	if !bytes.Equal(plain.Bytes(), expected) {
		return errors.New("GC receipt changed before durability")
	}
	if err = file.Sync(); err != nil {
		return err
	}
	ledger, err := os.Open(filepath.Join(root, ".restore-gc.db"))
	if err != nil {
		return err
	}
	defer ledger.Close()
	hash := sha256.New()
	if _, err = copyAuthorized(hash, ledger, s.checkAuthorized); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != receipt.LedgerSHA256 {
		return errors.New("GC ledger changed before durability")
	}
	if err = ledger.Sync(); err != nil {
		return err
	}
	if err = syncDirectory(root); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(root))
}

func fileIdentity(info os.FileInfo) (string, string, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", "", errors.New("file identity unavailable")
	}
	return fmt.Sprint(stat.Dev), fmt.Sprint(stat.Ino), nil
}
func hashFile(path string, check func() error) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := copyAuthorized(hash, file, check)
	return hex.EncodeToString(hash.Sum(nil)), size, err
}
func walkHashedTree(root string, check func() error, visit func(gcEntry) error) (string, error) {
	hash := sha256.New()
	var count int
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = check(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("restore tree contains symlink")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		device, inode, err := fileIdentity(info)
		if err != nil {
			return err
		}
		record := gcEntry{path: relative, kind: "directory", device: device, inode: inode}
		if entry.IsDir() {
			fmt.Fprintf(hash, "D\x00%s\x00", relative)
		} else {
			if !info.Mode().IsRegular() || count >= 1000000 {
				return errors.New("invalid restore tree")
			}
			count++
			record.kind = "file"
			record.hash, record.size, err = hashFile(path, check)
			if err != nil {
				return err
			}
			if record.size != info.Size() {
				return errors.New("restore tree changed")
			}
			current, err := os.Lstat(path)
			if err != nil || !os.SameFile(info, current) || info.Size() != current.Size() || !info.ModTime().Equal(current.ModTime()) {
				return errors.New("restore tree changed")
			}
			fmt.Fprintf(hash, "F\x00%s\x00%d\x00%s\x00", relative, record.size, record.hash)
		}
		if visit != nil {
			return visit(record)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *Store) openGCLedger(path string, key []byte, readonly bool) (*sql.DB, error) {
	nonce, err := randomName()
	if err != nil {
		return nil, err
	}
	driver := "puty_gc_" + nonce
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
	suffix := "?_journal_mode=DELETE&_synchronous=FULL&_temp_store=MEMORY"
	if readonly {
		suffix = "?mode=ro"
	}
	db, err := sql.Open(driver, path+suffix)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA temp_store=MEMORY; PRAGMA cipher_memory_security=ON;`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// The item inventory is a paged encrypted SQLCipher index, not a giant JSON
// held in RAM. Receipt pins its ciphertext hash and the old root's inode/dev.
func (s *Store) buildRestoreGarbage(sourceRoot, stage, previous string, key []byte, check func() error) (receipt restoreGarbage, err error) {
	path := filepath.Join(stage, ".restore-gc.db")
	if _, err = os.Lstat(path); !os.IsNotExist(err) {
		return receipt, errors.New("GC ledger already exists")
	}
	db, err := s.openGCLedger(path, key, false)
	if err != nil {
		return receipt, err
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE entries(path TEXT PRIMARY KEY,kind TEXT NOT NULL,size INTEGER NOT NULL,sha256 TEXT NOT NULL,device TEXT NOT NULL,inode TEXT NOT NULL)`); err != nil {
		return receipt, err
	}
	tx, err := db.Begin()
	if err != nil {
		return receipt, err
	}
	defer tx.Rollback()
	digest, err := walkHashedTree(sourceRoot, check, func(entry gcEntry) error {
		if entry.path == "." {
			receipt.RootDevice = entry.device
			receipt.RootInode = entry.inode
		}
		_, err := tx.Exec(`INSERT INTO entries VALUES(?,?,?,?,?,?)`, entry.path, entry.kind, entry.size, entry.hash, entry.device, entry.inode)
		return err
	})
	if err != nil {
		return receipt, err
	}
	if err = tx.Commit(); err != nil {
		return receipt, err
	}
	if err = db.Close(); err != nil {
		return receipt, err
	}
	ledgerHash, _, err := hashFile(path, check)
	if err != nil {
		return receipt, err
	}
	receipt.Previous = previous
	receipt.SHA256 = digest
	receipt.Phase = "pending"
	receipt.LedgerSHA256 = ledgerHash
	return receipt, nil
}
func ledgerEntry(db *sql.DB, path string) (expected gcEntry, err error) {
	expected.path = path
	err = db.QueryRow(`SELECT kind,size,sha256,device,inode FROM entries WHERE path=?`, path).Scan(&expected.kind, &expected.size, &expected.hash, &expected.device, &expected.inode)
	return
}
func compareGCEntry(expected, actual gcEntry) error {
	if expected.path != actual.path || expected.kind != actual.kind || expected.device != actual.device || expected.inode != actual.inode || expected.size != actual.size || expected.hash != actual.hash {
		return errors.New("previous vault entry changed")
	}
	return nil
}

func (s *Store) FinalizeRestoredPrevious() error {
	s.gcMu.Lock()
	defer s.gcMu.Unlock()
	s.mu.Lock()
	ready := s.viewsReady && s.authorizedLocked()
	key := bytes.Clone(s.master)
	s.mu.Unlock()
	defer clear(key)
	if !ready {
		return ErrLocked
	}
	receiptPath := filepath.Join(s.root, ".restore-gc")
	ledgerPath := filepath.Join(s.root, ".restore-gc.db")
	file, err := os.Open(receiptPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var plain bytes.Buffer
	err = localstore.Decrypt(&plain, io.LimitReader(file, 65537), key, localstore.ObjectIdentity{Account: s.account, Type: "restore-gc", ObjectID: "previous"})
	_ = file.Close()
	if err != nil {
		return err
	}
	defer clear(plain.Bytes())
	var receipt restoreGarbage
	decoder := json.NewDecoder(&plain)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&receipt) != nil || decoder.Decode(new(any)) != io.EOF || !noncePath(receipt.Previous, filepath.Base(s.root)+".previous-") || !hashText(receipt.SHA256) || !hashText(receipt.LedgerSHA256) || receipt.RootDevice == "" || receipt.RootInode == "" || (receipt.Phase != "pending" && receipt.Phase != "deleting") {
		return errors.New("invalid restore GC receipt")
	}
	previous := filepath.Join(filepath.Dir(s.root), receipt.Previous)
	info, err := os.Lstat(previous)
	if err == nil {
		device, inode, e := fileIdentity(info)
		if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || device != receipt.RootDevice || inode != receipt.RootInode {
			return errors.New("previous root replaced")
		}
		ledgerHash, _, e := hashFile(ledgerPath, s.checkAuthorized)
		if e != nil {
			return e
		}
		if ledgerHash != receipt.LedgerSHA256 {
			return errors.New("previous GC ledger changed")
		}
		db, e := s.openGCLedger(ledgerPath, key, true)
		if e != nil {
			return e
		}
		// Validate the entire remaining subset before deleting anything. New files,
		// replaced roots/children and edited ciphertext fail without further unlink.
		digest, e := walkHashedTree(previous, s.checkAuthorized, func(actual gcEntry) error {
			expected, e := ledgerEntry(db, actual.path)
			if e != nil {
				return errors.New("unknown previous vault entry")
			}
			return compareGCEntry(expected, actual)
		})
		if e == nil && receipt.Phase == "pending" && digest != receipt.SHA256 {
			e = errors.New("previous vault incomplete before GC")
		}
		if e != nil {
			_ = db.Close()
			return e
		}
		if receipt.Phase == "pending" {
			receipt.Phase = "deleting"
			encoded, _ := json.Marshal(receipt)
			if e = atomicEncrypted(receiptPath, encoded, key, localstore.ObjectIdentity{Account: s.account, Type: "restore-gc", ObjectID: "previous"}); e != nil {
				_ = db.Close()
				return e
			}
		}
		if e = gcDurabilityBoundary(s, receipt, key); e != nil {
			_ = db.Close()
			return e
		}
		e = removeLedgerTree(previous, previous, db, s.checkAuthorized)
		_ = db.Close()
		if e != nil {
			return e
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = s.checkAuthorized(); err != nil {
		return err
	}
	if err = os.Remove(ledgerPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err = syncDirectory(s.root); err != nil {
		return err
	}
	if err = os.Remove(receiptPath); err != nil {
		return err
	}
	return syncDirectory(s.root)
}
func removeLedgerTree(root, path string, db *sql.DB, check func() error) error {
	if err := check(); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("previous GC symlink")
	}
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	expected, err := ledgerEntry(db, filepath.ToSlash(relative))
	if err != nil {
		return errors.New("unknown previous GC item")
	}
	device, inode, err := fileIdentity(info)
	if err != nil || device != expected.device || inode != expected.inode {
		return errors.New("previous GC item replaced")
	}
	if info.IsDir() {
		if expected.kind != "directory" {
			return errors.New("previous GC type changed")
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err = removeLedgerTree(root, filepath.Join(path, entry.Name()), db, check); err != nil {
				return err
			}
		}
	} else {
		if !info.Mode().IsRegular() || expected.kind != "file" {
			return errors.New("previous GC type changed")
		}
		digest, size, err := hashFile(path, check)
		if err != nil {
			return err
		}
		if digest != expected.hash || size != expected.size {
			return errors.New("previous GC payload changed")
		}
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, current) {
		return errors.New("previous GC item replaced")
	}
	if err = check(); err != nil {
		return err
	}
	if err = os.Remove(path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}
