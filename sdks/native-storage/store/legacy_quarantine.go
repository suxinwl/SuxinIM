package nativestore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
	localstore "puty.local/localstore"
)

const maxLegacyDatabaseSnapshot = 256 << 20

type legacyMetadata struct {
	Source    string `json:"source"`
	Kind      string `json:"kind"`
	Account   string `json:"account"`
	Size      int64  `json:"size"`
	SHA256    string `json:"sha256"`
	CreatedAt int64  `json:"createdAt,omitempty"`
}

type limitedBuffer struct {
	*bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, errors.New("encrypted metadata exceeds limit")
	}
	return b.Buffer.Write(p)
}

// ConfigureLegacyQuarantine is native-only. Its device key is different from
// each account key; unknown historical content is never claimed by that actor.
func (s *Store) ConfigureLegacyQuarantine(directory string, key []byte) error {
	if len(key) != 32 || !filepath.IsAbs(directory) || filepath.Clean(directory) == string(filepath.Separator) {
		return ErrLocked
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	canonical, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.authorizedLocked() {
		return ErrLocked
	}
	if len(s.quarantineMaster) != 0 && (s.quarantineRoot != canonical || !bytes.Equal(key, s.quarantineMaster)) {
		return ErrLocked
	}
	s.quarantineRoot = canonical
	clear(s.quarantineMaster)
	s.quarantineMaster = bytes.Clone(key)
	return nil
}

func sourceDigest(source string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(source)))
	return hex.EncodeToString(sum[:])
}
func quarantineIdentity(objectID, kind string) localstore.ObjectIdentity {
	return localstore.ObjectIdentity{Account: "device-legacy", ObjectID: objectID, Type: kind}
}
func (s *Store) quarantineConfig() (string, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.authorizedLocked() || len(s.quarantineMaster) != 32 || s.quarantineRoot == "" {
		return "", nil, ErrLocked
	}
	return s.quarantineRoot, bytes.Clone(s.quarantineMaster), nil
}

// sealLegacySnapshot uses the same versioned layout as the native unknown-file
// quarantine. A second payload at one source path cannot overwrite its first.
func (s *Store) sealLegacySnapshot(source, account string, snapshot []byte) error {
	return s.sealLegacyStream(source, "database", account, bytes.NewReader(snapshot))
}
func (s *Store) sealLegacyStream(source, kind, account string, input io.ReadSeeker) error {
	directory, key, err := s.quarantineConfig()
	if err != nil {
		return err
	}
	defer clear(key)
	hash := sha256.New()
	size, err := io.Copy(hash, &authorizedSource{reader: input, check: s.checkAuthorized, limit: 64 << 30})
	if err != nil {
		return err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	sourceID := sourceDigest(source)
	objectID := sourceID + "-" + digest
	if _, err = input.Seek(0, io.SeekStart); err != nil {
		return err
	}
	directory = filepath.Join(directory, "objects", sourceID)
	if err = os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	if err = syncDirectory(filepath.Dir(directory)); err != nil {
		return err
	}
	if err = syncDirectory(filepath.Dir(filepath.Dir(directory))); err != nil {
		return err
	}
	meta := legacyMetadata{Source: filepath.Clean(source), Kind: kind, Account: account, Size: size, SHA256: digest, CreatedAt: time.Now().UnixMilli()}
	encoded, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	defer clear(encoded)
	for _, item := range []struct {
		path, kind string
		reader     io.Reader
	}{{digest + ".data", "legacy-file", input}, {digest + ".meta", "legacy-metadata", bytes.NewReader(encoded)}} {
		target := filepath.Join(directory, item.path)
		if _, statErr := os.Stat(target); statErr == nil {
			if item.kind == "legacy-file" {
				continue
			}
			old, readErr := s.readQuarantineMetadata(target, key, objectID)
			if readErr != nil || old.Source != meta.Source || old.Account != account || old.Kind != kind || old.SHA256 != digest || old.Size != meta.Size {
				return errors.New("legacy quarantine receipt mismatch")
			}
			continue
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
		tmp, createErr := os.CreateTemp(directory, ".sealed-")
		if createErr != nil {
			return createErr
		}
		name := tmp.Name()
		err = localstore.Encrypt(tmp, &authorizedSource{reader: item.reader, check: s.checkAuthorized, limit: 64 << 30}, key, quarantineIdentity(objectID, item.kind))
		if err == nil {
			err = tmp.Sync()
		}
		closeErr := tmp.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = s.checkAuthorized()
		}
		if err == nil {
			err = os.Rename(name, target)
		}
		if err != nil {
			_ = os.Remove(name)
			return err
		}
		if err = syncDirectory(directory); err != nil {
			return err
		}
	}
	// A previous attempt may have renamed both objects but failed directory
	// fsync. Reusing its authenticated receipt must re-establish durability.
	for _, name := range []string{digest + ".data", digest + ".meta"} {
		file, openErr := os.OpenFile(filepath.Join(directory, name), os.O_RDWR, 0600)
		if openErr != nil {
			return openErr
		}
		syncErr := file.Sync()
		closeErr := file.Close()
		if syncErr != nil {
			return syncErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	for _, parent := range []string{directory, filepath.Dir(directory), filepath.Dir(filepath.Dir(directory))} {
		if err = syncDirectory(parent); err != nil {
			return err
		}
	}
	f, err := os.Open(filepath.Join(directory, digest+".data"))
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	r, err := localstore.OpenAuthenticatedReader(f, info.Size(), key, quarantineIdentity(objectID, "legacy-file"), s.checkAuthorized)
	if err != nil {
		return err
	}
	defer r.Close()
	hash.Reset()
	verified, err := io.Copy(hash, r)
	if err != nil || verified != meta.Size || hex.EncodeToString(hash.Sum(nil)) != digest {
		return errors.New("legacy quarantine verification failed")
	}
	return s.checkAuthorized()
}

func (s *Store) readQuarantineMetadata(path string, key []byte, objectID string) (legacyMetadata, error) {
	var meta legacyMetadata
	f, err := os.Open(path)
	if err != nil {
		return meta, err
	}
	defer f.Close()
	var data bytes.Buffer
	if err = localstore.Decrypt(&limitedBuffer{Buffer: &data, limit: 65536}, f, key, quarantineIdentity(objectID, "legacy-metadata")); err != nil {
		clear(data.Bytes())
		return meta, err
	}
	err = json.Unmarshal(data.Bytes(), &meta)
	clear(data.Bytes())
	return meta, err
}

type quarantinedSource struct {
	*localstore.AuthenticatedReader
	file *os.File
}

func (r *quarantinedSource) Close() error { _ = r.AuthenticatedReader.Close(); return r.file.Close() }

// openQuarantinedSource has no Dart route. A caller must first match the exact
// historical sender/path or the precise legacy database account identity.
func (s *Store) openQuarantinedSource(source, kind, account string) (*quarantinedSource, legacyMetadata, error) {
	var empty legacyMetadata
	directory, key, err := s.quarantineConfig()
	if err != nil {
		return nil, empty, err
	}
	defer clear(key)
	sourceID := sourceDigest(source)
	directory = filepath.Join(directory, "objects", sourceID)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, empty, err
	}
	var selected legacyMetadata
	var selectedPath, selectedID string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || len(name) != 69 || !strings.HasSuffix(name, ".meta") {
			continue
		}
		digest := strings.TrimSuffix(name, ".meta")
		if _, err = hex.DecodeString(digest); err != nil {
			continue
		}
		id := sourceID + "-" + digest
		meta, readErr := s.readQuarantineMetadata(filepath.Join(directory, name), key, id)
		if readErr != nil {
			return nil, empty, readErr
		}
		if meta.Source != filepath.Clean(source) || meta.Kind != kind || meta.Account != account || meta.SHA256 != digest || meta.Size < 0 {
			continue
		}
		if selectedPath != "" && meta.SHA256 != selected.SHA256 {
			return nil, empty, errors.New("multiple legacy versions require explicit recovery")
		}
		if selectedPath == "" {
			selected = meta
			selectedPath = filepath.Join(directory, digest+".data")
			selectedID = id
		}
	}
	if selectedPath == "" {
		return nil, empty, os.ErrNotExist
	}
	f, err := os.Open(selectedPath)
	if err != nil {
		return nil, empty, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, empty, err
	}
	r, err := localstore.OpenAuthenticatedReader(f, info.Size(), key, quarantineIdentity(selectedID, "legacy-file"), s.checkAuthorized)
	if err != nil {
		_ = f.Close()
		return nil, empty, err
	}
	if r.Size() != selected.Size {
		_ = r.Close()
		_ = f.Close()
		return nil, empty, errors.New("legacy quarantine size mismatch")
	}
	return &quarantinedSource{r, f}, selected, nil
}

// QuarantineLegacyDatabases is called only after this actor's exact DB/media
// migration. Every other historical DB is captured with its committed WAL view
// in one read transaction before any main/WAL/SHM file can be removed.
func (s *Store) QuarantineLegacyDatabases() error {
	if _, key, err := s.quarantineConfig(); err != nil {
		return err
	} else {
		clear(key)
	}
	s.mu.Lock()
	roots := append([]legacyRoot{}, s.legacyRoots...)
	s.mu.Unlock()
	seen := map[string]bool{}
	for _, root := range roots {
		err := filepath.WalkDir(root.canonical, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			relative, err := filepath.Rel(root.canonical, path)
			if err != nil {
				return err
			}
			declared := filepath.Join(root.declared, relative)
			if entry.IsDir() {
				if path != root.canonical && !s.legacyOwned(declared) {
					return filepath.SkipDir
				}
				return nil
			}
			name := entry.Name()
			if !strings.HasPrefix(name, "OpenIM_v3_") || !strings.HasSuffix(name, ".db") || !s.legacyOwned(declared) || seen[path] {
				return nil
			}
			account := strings.TrimSuffix(strings.TrimPrefix(name, "OpenIM_v3_"), ".db")
			if account == "" || len(account) > 512 {
				return errors.New("invalid historical database identity")
			}
			seen[path] = true
			return s.quarantineDatabase(declared, account)
		})
		if err != nil {
			return err
		}
	}
	return s.checkAuthorized()
}

func (s *Store) quarantineDatabase(path, account string) error {
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
		return errors.New("unknown historical database retained for explicit recovery")
	}
	driver := s.driver + "_quarantine"
	registerMigrationDriver(driver, s)
	u := url.URL{Scheme: "file", Path: path}
	db, err := sql.Open(driver, u.String()+"?mode=ro&_busy_timeout=2500")
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	conn, err := db.Conn(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(context.Background(), "BEGIN"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	var count, page int64
	if err = conn.QueryRowContext(context.Background(), "PRAGMA page_count").Scan(&count); err != nil {
		return err
	}
	if err = conn.QueryRowContext(context.Background(), "PRAGMA page_size").Scan(&page); err != nil {
		return err
	}
	if page <= 0 || count < 0 || count > maxLegacyDatabaseSnapshot/page {
		return errors.New("legacy database needs bounded explicit migration")
	}
	var integrity string
	if err = conn.QueryRowContext(context.Background(), "PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		return errors.New("legacy database integrity failed; original retained")
	}
	var snapshot []byte
	if err = conn.Raw(func(value any) error {
		var err error
		snapshot, err = value.(*sqlite3.SQLiteConn).Serialize("")
		return err
	}); err != nil {
		return err
	}
	defer clear(snapshot)
	if len(snapshot) > maxLegacyDatabaseSnapshot {
		return errors.New("legacy database needs bounded explicit migration")
	}
	if err = s.sealLegacySnapshot(path, account, snapshot); err != nil {
		return err
	}
	if _, err = conn.ExecContext(context.Background(), "ROLLBACK"); err != nil {
		return err
	}
	_ = conn.Close()
	_ = db.Close()
	if err = s.checkAuthorized(); err != nil {
		return err
	}
	return removeLegacyFiles(path)
}

// QuarantineLegacyOwnedFiles runs only after all legacy preferences/Hive and
// current-account DB/media imports have committed and old producers stopped.
// The broker supplies fixed application roots; public originals are excluded.
func (s *Store) QuarantineLegacyOwnedFiles() error {
	s.mu.Lock()
	roots := append([]legacyRoot{}, s.legacyRoots...)
	s.mu.Unlock()
	seen := map[string]bool{}
	for _, root := range roots {
		err := filepath.WalkDir(root.canonical, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			relative, err := filepath.Rel(root.canonical, path)
			if err != nil {
				return err
			}
			declared := filepath.Join(root.declared, relative)
			if entry.IsDir() {
				if path != root.canonical && !s.legacyOwned(declared) {
					return filepath.SkipDir
				}
				return nil
			}
			if !s.legacyOwned(declared) || seen[path] {
				return nil
			}
			seen[path] = true
			name := entry.Name()
			if strings.HasPrefix(name, "OpenIM_v3_") && (strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".db-wal") || strings.HasSuffix(name, ".db-shm") || strings.HasSuffix(name, ".db-journal")) {
				return nil
			}
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			before, err := file.Stat()
			if err != nil {
				_ = file.Close()
				return err
			}
			if !before.Mode().IsRegular() {
				_ = file.Close()
				return nil
			}
			err = s.sealLegacyStream(declared, "file", "", file)
			after, statErr := file.Stat()
			closeErr := file.Close()
			if err != nil {
				return err
			}
			if statErr != nil {
				return statErr
			}
			if closeErr != nil {
				return closeErr
			}
			current, err := os.Lstat(path)
			if err != nil {
				return err
			}
			if !os.SameFile(before, current) || !os.SameFile(before, after) || before.Size() != after.Size() || before.ModTime() != after.ModTime() || current.ModTime() != after.ModTime() || current.Size() != after.Size() {
				return errors.New("legacy source changed; original retained")
			}
			if err = s.checkAuthorized(); err != nil {
				return err
			}
			if err = os.Remove(path); err != nil {
				return err
			}
			return syncDirectory(filepath.Dir(path))
		})
		if err != nil {
			return err
		}
	}
	return s.checkAuthorized()
}
