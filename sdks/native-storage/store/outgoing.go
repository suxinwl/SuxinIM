package nativestore

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	localstore "puty.local/localstore"
	"strings"
	"sync"
	"time"
)

var outgoingGuard sync.Mutex

// StageOutgoing streams into authenticated ciphertext. It never creates a
// plaintext SDK copy. The caller owns the original external document; only
// explicitly identified application temp files may subsequently be removed.
func (s *Store) StageOutgoing(source io.Reader, sourceName string) (string, error) {
	if localstore.ValidateSourceName(sourceName) != nil || s.checkAuthorized() != nil {
		return "", ErrLocked
	}
	outgoingGuard.Lock()
	defer outgoingGuard.Unlock()
	s.mu.Lock()
	if !s.authorizedLocked() {
		s.mu.Unlock()
		return "", ErrLocked
	}
	key := bytes.Clone(s.master)
	s.mu.Unlock()
	defer clear(key)
	object, err := randomName()
	if err != nil {
		return "", err
	}
	directory := filepath.Join(s.root, "outgoing")
	if err = os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(directory, object+filepath.Ext(sourceName))
	output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	committed := false
	defer func() {
		_ = output.Close()
		if !committed {
			_ = os.Remove(path)
		}
	}()
	counted := &authorizedSource{reader: source, check: s.checkAuthorized, limit: 1 << 30}
	if err = localstore.Encrypt(output, counted, key, localstore.ObjectIdentity{Account: s.account, Type: "outgoing", ObjectID: object}); err != nil {
		return "", err
	}
	if err = output.Sync(); err != nil {
		return "", err
	}
	if err = output.Close(); err != nil {
		return "", err
	}
	if err = syncDirectory(directory); err != nil {
		return "", err
	}
	if _, err = s.db.Exec(`INSERT INTO puty_outgoing_sources VALUES(?,?,?,?)`, object, filepath.Base(path), sourceName, counted.total); err != nil {
		return "", err
	}
	committed = true
	return path, nil
}

type authorizedSource struct {
	reader       io.Reader
	check        func() error
	total, limit int64
}

func (r *authorizedSource) Read(p []byte) (int, error) {
	if err := r.check(); err != nil {
		return 0, err
	}
	n, err := r.reader.Read(p)
	r.total += int64(n)
	if r.total > r.limit {
		clear(p[:n])
		return 0, errors.New("outgoing source exceeds local limit")
	}
	if denied := r.check(); denied != nil {
		clear(p[:n])
		return 0, denied
	}
	return n, err
}

// PrepareOutgoing accepts an already encrypted owned source or imports one
// readable user-selected document. It does not alter an external original.
func (s *Store) PrepareOutgoing(path string) (string, error) {
	if s.checkAuthorized() != nil {
		return "", ErrLocked
	}
	if strings.HasPrefix(path, "/proc/") {
		return "", errors.New("native descriptor must be imported by its bound broker")
	}
	if path == "" {
		return "", nil
	}
	if filepath.Dir(path) == filepath.Join(s.root, "outgoing") {
		_, _, err := s.outgoingIdentity(path)
		return path, err
	}
	input, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("invalid outgoing document")
	}
	return s.StageOutgoing(input, info.Name())
}

func (s *Store) SourceInfo(path string) (string, int64, error) {
	_, name, err := s.outgoingIdentity(path)
	if err != nil {
		return "", 0, err
	}
	var size int64
	err = s.db.QueryRow(`SELECT size FROM puty_outgoing_sources WHERE path=?`, filepath.Base(path)).Scan(&size)
	return name, size, err
}
func (s *Store) DiscardUnboundSource(path string) error {
	if _, _, err := s.outgoingIdentity(path); err != nil {
		return err
	}
	outgoingGuard.Lock()
	defer outgoingGuard.Unlock()
	var refs int
	if err := s.db.QueryRow(`SELECT count(*) FROM puty_outgoing_refs r JOIN puty_outgoing_sources s ON s.object_id=r.object_id WHERE s.path=?`, filepath.Base(path)).Scan(&refs); err != nil {
		return err
	}
	if refs != 0 {
		return errors.New("pending message still references source")
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if object, _, err := s.outgoingIdentity(path); err == nil {
		s.discardRangeReader("outgoing:" + object)
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM puty_outgoing_sources WHERE path=?`, filepath.Base(path)); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}
func (s *Store) outgoingIdentity(path string) (object, name string, err error) {
	if s.checkAuthorized() != nil {
		return "", "", ErrLocked
	}
	if filepath.Dir(path) != filepath.Join(s.root, "outgoing") {
		return "", "", errors.New("not an owned outgoing file")
	}
	err = s.db.QueryRow(`SELECT object_id,source_name FROM puty_outgoing_sources WHERE path=?`, filepath.Base(path)).Scan(&object, &name)
	return
}

type OutgoingReader struct {
	*localstore.AuthenticatedReader
	file *os.File
	name string
}

func (r *OutgoingReader) Close() error { _ = r.AuthenticatedReader.Close(); return r.file.Close() }
func (r *OutgoingReader) Stat() (os.FileInfo, error) {
	return outgoingInfo{name: r.name, size: r.Size()}, nil
}

type outgoingInfo struct {
	name string
	size int64
}

func (i outgoingInfo) Name() string       { return i.name }
func (i outgoingInfo) Size() int64        { return i.size }
func (i outgoingInfo) Mode() os.FileMode  { return 0600 }
func (i outgoingInfo) ModTime() time.Time { return time.Time{} }
func (i outgoingInfo) IsDir() bool        { return false }
func (i outgoingInfo) Sys() any           { return nil }

func OpenOutgoing(path string) (*OutgoingReader, error) {
	registry.Lock()
	var candidates []*Store
	for _, store := range registry.stores {
		if strings.HasPrefix(path, store.root+string(filepath.Separator)) {
			candidates = append(candidates, store)
		}
	}
	registry.Unlock()
	for _, store := range candidates {
		object, name, err := store.outgoingIdentity(path)
		if err != nil {
			continue
		}
		store.mu.Lock()
		if !store.authorizedLocked() {
			store.mu.Unlock()
			return nil, ErrLocked
		}
		key := bytes.Clone(store.master)
		store.mu.Unlock()
		defer clear(key)
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return nil, err
		}
		reader, err := localstore.OpenAuthenticatedReader(file, info.Size(), key, localstore.ObjectIdentity{Account: store.account, Type: "outgoing", ObjectID: object}, store.checkAuthorized)
		if err != nil {
			_ = file.Close()
			return nil, err
		}
		return &OutgoingReader{reader, file, name}, nil
	}
	return nil, ErrLocked
}

func (s *Store) BindOutgoing(message string, paths ...string) error {
	outgoingGuard.Lock()
	defer outgoingGuard.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var deleted int
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM puty_local_deleted WHERE message_id=?)`, message).Scan(&deleted); err != nil {
		return err
	}
	if deleted != 0 {
		return errors.New("locally cleared message cannot acquire a producer source")
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		var object string
		if filepath.Dir(path) != filepath.Join(s.root, "outgoing") {
			return errors.New("unowned outgoing reference")
		}
		if err = tx.QueryRow(`SELECT object_id FROM puty_outgoing_sources WHERE path=?`, filepath.Base(path)).Scan(&object); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT OR IGNORE INTO puty_outgoing_refs VALUES(?,?)`, message, object); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ReleaseOutgoing is called only after server ACK or a confirmed local delete.
// Failed/queued messages retain encrypted source and can retry after restart.
func (s *Store) ReleaseOutgoing(message string) error {
	outgoingGuard.Lock()
	defer outgoingGuard.Unlock()
	rows, err := s.db.Query(`SELECT s.object_id,s.path FROM puty_outgoing_sources s JOIN puty_outgoing_refs r ON r.object_id=s.object_id WHERE r.message_id=?`, message)
	if err != nil {
		return err
	}
	type item struct{ id, path string }
	var items []item
	for rows.Next() {
		var i item
		if err = rows.Scan(&i.id, &i.path); err != nil {
			_ = rows.Close()
			return err
		}
		items = append(items, i)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, i := range items {
		var count int
		if err = s.db.QueryRow(`SELECT count(*) FROM puty_outgoing_refs WHERE object_id=? AND message_id<>?`, i.id, message).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			if _, err = s.db.Exec(`DELETE FROM puty_outgoing_refs WHERE message_id=? AND object_id=?`, message, i.id); err != nil {
				return err
			}
			continue
		}
		if filepath.Base(i.path) != i.path || strings.ContainsAny(i.path, "/\\") {
			return errors.New("invalid owned outgoing path")
		}
		if err = os.Remove(filepath.Join(s.root, "outgoing", i.path)); err != nil && !os.IsNotExist(err) {
			return err
		}
		s.discardRangeReader("outgoing:" + i.id)
		// File deletion and its directory fsync precede the SQL ACK. A failed
		// unlink/fsync leaves the exact source/ref available to the next retry.
		if err = syncDirectory(filepath.Join(s.root, "outgoing")); err != nil {
			return err
		}
		tx, beginErr := s.db.Begin()
		if beginErr != nil {
			return beginErr
		}
		if _, err = tx.Exec(`DELETE FROM puty_outgoing_refs WHERE message_id=? AND object_id=?`, message, i.id); err == nil {
			_, err = tx.Exec(`DELETE FROM puty_outgoing_sources WHERE object_id=?`, i.id)
		}
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	if len(items) > 0 {
		return syncDirectory(filepath.Join(s.root, "outgoing"))
	}
	return nil
}

func (s *Store) ReadOutgoing(path string, max int64) ([]byte, error) {
	if max < 0 || max > 32<<20 {
		return nil, ErrLocked
	}
	if _, _, err := s.outgoingIdentity(path); err != nil {
		return nil, err
	}
	reader, err := OpenOutgoing(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	if reader.Size() > max {
		return nil, errors.New("outgoing preview limit")
	}
	data, err := io.ReadAll(reader)
	if err != nil || s.checkAuthorized() != nil {
		clear(data)
		return nil, ErrLocked
	}
	return data, nil
}

var _ io.ReadSeekCloser = (*OutgoingReader)(nil)
