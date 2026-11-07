package nativestore

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	localstore "puty.local/localstore"
	"time"
)

// Ciphertext-only descriptors are kept in a small, native-owned cache. An
// initial open authenticates the entire object including EOF; subsequent seeks
// authenticate the selected frames again. Every read is lease fenced and never
// writes a decrypted temporary file.
type rangeReader struct {
	file   *os.File
	reader *localstore.AuthenticatedReader
	used   time.Time
}

func (s *Store) closeRangeReaders() {
	s.rangeMu.Lock()
	defer s.rangeMu.Unlock()
	for key, r := range s.rangeReaders {
		_ = r.reader.Close()
		_ = r.file.Close()
		delete(s.rangeReaders, key)
	}
}
func (s *Store) discardRangeReader(key string) {
	s.rangeMu.Lock()
	defer s.rangeMu.Unlock()
	if r := s.rangeReaders[key]; r != nil {
		_ = r.reader.Close()
		_ = r.file.Close()
		delete(s.rangeReaders, key)
	}
}
func (s *Store) readObjectRange(cacheKey, path, kind, object string, size, offset int64, count int, check func() error) ([]byte, error) {
	if offset < 0 || count < 1 || count > 1<<20 || offset > size || check() != nil {
		return nil, ErrLocked
	}
	s.rangeMu.Lock()
	defer s.rangeMu.Unlock()
	if s.rangeReaders == nil {
		s.rangeReaders = map[string]*rangeReader{}
	}
	r := s.rangeReaders[cacheKey]
	if r == nil {
		if len(s.rangeReaders) >= 4 {
			var oldest string
			var at time.Time
			for k, item := range s.rangeReaders {
				if oldest == "" || item.used.Before(at) {
					oldest = k
					at = item.used
				}
			}
			old := s.rangeReaders[oldest]
			_ = old.reader.Close()
			_ = old.file.Close()
			delete(s.rangeReaders, oldest)
		}
		s.mu.Lock()
		if !s.authorizedLocked() {
			s.mu.Unlock()
			return nil, ErrLocked
		}
		key := bytes.Clone(s.master)
		s.mu.Unlock()
		f, err := os.Open(path)
		if err != nil {
			clear(key)
			return nil, err
		}
		info, err := f.Stat()
		if err != nil {
			clear(key)
			_ = f.Close()
			return nil, err
		}
		reader, err := localstore.OpenAuthenticatedReader(f, info.Size(), key, localstore.ObjectIdentity{Account: s.account, Type: kind, ObjectID: object}, check)
		clear(key)
		if err != nil {
			_ = f.Close()
			return nil, err
		}
		if reader.Size() != size {
			_ = reader.Close()
			_ = f.Close()
			return nil, errors.New("authenticated object size mismatch")
		}
		r = &rangeReader{file: f, reader: reader}
		s.rangeReaders[cacheKey] = r
	}
	r.used = time.Now()
	length := int64(count)
	if length > size-offset {
		length = size - offset
	}
	output := make([]byte, int(length))
	n, err := r.reader.ReadAt(output, offset)
	if err == io.EOF && n == len(output) {
		err = nil
	}
	if err != nil || n != len(output) || check() != nil {
		clear(output)
		_ = r.reader.Close()
		_ = r.file.Close()
		delete(s.rangeReaders, cacheKey)
		return nil, ErrLocked
	}
	return output, nil
}
func (s *Store) ReadOutgoingRange(path string, offset int64, count int) ([]byte, error) {
	object, _, err := s.outgoingIdentity(path)
	if err != nil {
		return nil, err
	}
	var size int64
	if err = s.db.QueryRow(`SELECT size FROM puty_outgoing_sources WHERE object_id=?`, object).Scan(&size); err != nil {
		return nil, err
	}
	return s.readObjectRange("outgoing:"+object, path, "outgoing", object, size, offset, count, s.checkAuthorized)
}
func (s *Store) ReadAttachmentRange(task string, offset int64, count int) ([]byte, error) {
	check := func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.authorizedLocked() || !s.viewsReady {
			return ErrLocked
		}
		return nil
	}
	if check() != nil {
		return nil, ErrLocked
	}
	var name, month, object, state string
	var size int64
	if err := s.db.QueryRow(`SELECT allocated_name,month,object_id,state,bytes FROM puty_attachment_tasks WHERE task_id=?`, task).Scan(&name, &month, &object, &state, &size); err != nil {
		return nil, err
	}
	if state != "complete" {
		return nil, errors.New("attachment is not complete")
	}
	return s.readObjectRange("attachment:"+task, filepath.Join(s.root, "archive", month, "attachments", name), "attachment", object, size, offset, count, check)
}
