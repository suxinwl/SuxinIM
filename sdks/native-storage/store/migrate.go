package nativestore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	sqlite3 "github.com/mattn/go-sqlite3"
)

// MigrateLegacy exports the exact legacy SDK DB under its current online actor.
// The source is read-only and retained until encrypted export, integrity check,
// file fsync, atomic promotion, and parent-directory fsync have all succeeded.
func (s *Store) MigrateLegacy(path string) error {
	if filepath.Base(path) != "OpenIM_v3_"+s.account+".db" || !filepath.IsAbs(path) {
		return errors.New("invalid legacy database identity")
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return s.migrateQuarantinedDatabase(path)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("legacy database must be an owned regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	var magic [16]byte
	_, err = io.ReadFull(file, magic[:])
	_ = file.Close()
	if err != nil {
		return err
	}
	if string(magic[:]) != "SQLite format 3\x00" {
		return errors.New("unexpected legacy database format")
	}
	s.mu.Lock()
	if !s.authorizedLocked() {
		s.mu.Unlock()
		return ErrLocked
	}
	key := bytes.Clone(s.master)
	s.mu.Unlock()
	defer clear(key)
	final := filepath.Join(s.root, "working.db")
	if _, err = os.Stat(final); err == nil {
		// Only a committed receipt permits cleanup after a crash between new DB
		// promotion and old DB deletion. Never guess that an unrelated DB is newer.
		probe, openErr := sql.Open(s.driver, final)
		if openErr != nil {
			return openErr
		}
		var source string
		checkErr := probe.QueryRow(`SELECT source FROM puty_migration_receipt WHERE id=1`).Scan(&source)
		_ = probe.Close()
		if checkErr != nil || source != filepath.Base(path) {
			return errors.New("legacy migration requires explicit recovery")
		}
		if s.checkAuthorized() != nil {
			return ErrLocked
		}
		return removeLegacyFiles(path)
	} else if !os.IsNotExist(err) {
		return err
	}
	plainDriver := s.driver + "_migration"
	// Register once per account driver. It has no key and is used only by this
	// migration method; normal mobile opens can never select this driver.
	registerMigrationDriver(plainDriver, s)
	u := url.URL{Scheme: "file", Path: path}
	legacy, err := sql.Open(plainDriver, u.String()+"?mode=ro&_busy_timeout=10000")
	if err != nil {
		return err
	}
	legacy.SetMaxOpenConns(1)
	defer legacy.Close()
	connection, err := legacy.Conn(context.Background())
	if err != nil {
		return err
	}
	defer connection.Close()
	if err = s.promoteLegacyConnection(connection, path, key, final); err != nil {
		return err
	}
	_ = connection.Close()
	_ = legacy.Close()
	return removeLegacyFiles(path)
}

func (s *Store) promoteLegacyConnection(connection *sql.Conn, path string, key []byte, final string) error {
	var err error
	if err = os.MkdirAll(s.root, 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(s.root, ".migration-")
	if err != nil {
		return err
	}
	target := temporary.Name()
	_ = temporary.Close()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(target)
		}
	}()
	if _, err = connection.ExecContext(context.Background(), `ATTACH DATABASE ? AS protected`, target); err != nil {
		return err
	}
	if err = connection.Raw(func(value any) error { return value.(*sqlite3.SQLiteConn).SetAttachedRawKey("protected", key) }); err != nil {
		return err
	}
	if _, err = connection.ExecContext(context.Background(), `SELECT sqlcipher_export('protected')`); err != nil {
		return err
	}
	if _, err = connection.ExecContext(context.Background(), `CREATE TABLE protected.puty_migration_receipt(id INTEGER PRIMARY KEY,source TEXT NOT NULL);INSERT INTO protected.puty_migration_receipt(id,source) VALUES(1,?)`, filepath.Base(path)); err != nil {
		return err
	}
	if _, err = connection.ExecContext(context.Background(), `DETACH DATABASE protected`); err != nil {
		return err
	}
	probe, err := sql.Open(s.driver, target)
	if err != nil {
		return err
	}
	var integrity string
	err = probe.QueryRow(`PRAGMA integrity_check`).Scan(&integrity)
	_ = probe.Close()
	if err != nil || integrity != "ok" {
		return errors.New("encrypted migration integrity failed")
	}
	sealed, err := os.OpenFile(target, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	err = sealed.Sync()
	_ = sealed.Close()
	if err != nil {
		return err
	}
	if s.checkAuthorized() != nil {
		return ErrLocked
	}
	if err = os.Rename(target, final); err != nil {
		return err
	}
	if err = syncDirectory(s.root); err != nil {
		return err
	}
	committed = true
	if s.checkAuthorized() != nil {
		return ErrLocked
	}
	return nil
}

// Other accounts' legacy snapshots remain device-encrypted until that precise
// actor is online. Deserialize occurs exclusively in RAM; no plaintext restore
// file, journal, or temporary SQLite database is created.
func (s *Store) migrateQuarantinedDatabase(path string) error {
	if _, err := os.Stat(filepath.Join(s.root, "working.db")); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	s.mu.Lock()
	configured := len(s.quarantineMaster) == 32
	s.mu.Unlock()
	if !configured {
		return nil
	}
	source, meta, err := s.openQuarantinedSource(path, "database", s.account)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer source.Close()
	if meta.Size > maxLegacyDatabaseSnapshot {
		return errors.New("legacy database needs bounded explicit migration")
	}
	snapshot, err := io.ReadAll(io.LimitReader(source, maxLegacyDatabaseSnapshot+1))
	if err != nil {
		return err
	}
	defer clear(snapshot)
	sum := sha256.Sum256(snapshot)
	if int64(len(snapshot)) != meta.Size || hex.EncodeToString(sum[:]) != meta.SHA256 || len(snapshot) < 100 || string(snapshot[:16]) != "SQLite format 3\x00" {
		return errors.New("legacy database snapshot verification failed")
	}
	// SQLite's documented deserialize contract requires rollback format bits.
	// The authenticated snapshot includes committed WAL pages; it needs no WAL.
	snapshot[18] = 1
	snapshot[19] = 1
	driver := s.driver + "_deserialize"
	registerMigrationDriver(driver, s)
	db, err := sql.Open(driver, ":memory:")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	connection, err := db.Conn(context.Background())
	if err != nil {
		return err
	}
	defer connection.Close()
	if err = connection.Raw(func(value any) error { return value.(*sqlite3.SQLiteConn).Deserialize(snapshot, "") }); err != nil {
		return err
	}
	var integrity string
	if err = connection.QueryRowContext(context.Background(), "PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		return errors.New("legacy RAM snapshot integrity failed")
	}
	s.mu.Lock()
	key := bytes.Clone(s.master)
	s.mu.Unlock()
	defer clear(key)
	return s.promoteLegacyConnection(connection, path, key, filepath.Join(s.root, "working.db"))
}

var migrationDrivers = struct {
	sync.Mutex
	names map[string]bool
}{names: map[string]bool{}}

func registerMigrationDriver(name string, s *Store) {
	migrationDrivers.Lock()
	defer migrationDrivers.Unlock()
	if migrationDrivers.names[name] {
		return
	}
	sql.Register(name, &sqlite3.SQLiteDriver{KeyHook: func(c *sqlite3.SQLiteConn) error {
		c.AccessCheck = func() error {
			s.mu.Lock()
			defer s.mu.Unlock()
			if !s.authorizedLocked() {
				return ErrLocked
			}
			return nil
		}
		return c.AccessCheck()
	}})
	migrationDrivers.names[name] = true
}
func removeLegacyFiles(path string) error {
	for _, suffix := range []string{"-wal", "-shm", "-journal", ""} {
		name := path + suffix
		file, err := os.OpenFile(name, os.O_WRONLY, 0600)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if err = file.Truncate(0); err == nil {
			err = file.Sync()
		}
		_ = file.Close()
		if err != nil {
			return err
		}
		if err = os.Remove(name); err != nil {
			return err
		}
	}
	return syncDirectory(filepath.Dir(path))
}
