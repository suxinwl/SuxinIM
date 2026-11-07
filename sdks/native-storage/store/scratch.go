package nativestore

import (
	"database/sql"
	"errors"
	"io"
	localstore "puty.local/localstore"
	"sync"
)

const ScratchPageSize = 65536
const ScratchMaxSize = 2 << 30

var scratchGuard sync.Mutex

// Scratch pages are always SQLCipher-encrypted, including its WAL. Native
// platform proxy descriptors expose these bytes as a seekable virtual file to
// camera/muxer/compressor code without a plaintext cache or temporary file.
func (s *Store) BeginScratch(name string) (string, error) {
	if localstore.ValidateSourceName(name) != nil {
		return "", errors.New("invalid scratch source name")
	}
	id, err := randomName()
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(`INSERT INTO puty_scratch_objects(object_id,source_name)VALUES(?,?)`, id, name)
	return id, err
}
func (s *Store) ScratchSize(id string) (int64, error) {
	var size int64
	err := s.db.QueryRow(`SELECT size FROM puty_scratch_objects WHERE object_id=?`, id).Scan(&size)
	return size, err
}
func (s *Store) WriteScratch(id string, offset int64, data []byte) error {
	if offset < 0 || len(data) > 1<<20 || offset+int64(len(data)) > ScratchMaxSize {
		return errors.New("scratch write bounds")
	}
	scratchGuard.Lock()
	defer scratchGuard.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var size int64
	if err = tx.QueryRow(`SELECT size FROM puty_scratch_objects WHERE object_id=?`, id).Scan(&size); err != nil {
		return err
	}
	for cursor := 0; cursor < len(data); {
		position := offset + int64(cursor)
		page := position / ScratchPageSize
		within := int(position % ScratchPageSize)
		count := ScratchPageSize - within
		if count > len(data)-cursor {
			count = len(data) - cursor
		}
		var old []byte
		err = tx.QueryRow(`SELECT data FROM puty_scratch_pages WHERE object_id=? AND page_no=?`, id, page).Scan(&old)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		buffer := make([]byte, ScratchPageSize)
		copy(buffer, old)
		clear(old)
		copy(buffer[within:within+count], data[cursor:cursor+count])
		_, err = tx.Exec(`INSERT INTO puty_scratch_pages VALUES(?,?,?) ON CONFLICT(object_id,page_no)DO UPDATE SET data=excluded.data`, id, page, buffer)
		clear(buffer)
		if err != nil {
			return err
		}
		cursor += count
	}
	end := offset + int64(len(data))
	if end > size {
		if _, err = tx.Exec(`UPDATE puty_scratch_objects SET size=? WHERE object_id=?`, end, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) ReadScratch(id string, offset int64, count int) ([]byte, error) {
	if offset < 0 || count < 0 || count > 1<<20 {
		return nil, errors.New("scratch read bounds")
	}
	scratchGuard.Lock()
	defer scratchGuard.Unlock()
	size, err := s.ScratchSize(id)
	if err != nil {
		return nil, err
	}
	if offset >= size {
		return []byte{}, nil
	}
	if int64(count) > size-offset {
		count = int(size - offset)
	}
	result := make([]byte, count)
	for cursor := 0; cursor < count; {
		position := offset + int64(cursor)
		page := position / ScratchPageSize
		within := int(position % ScratchPageSize)
		n := ScratchPageSize - within
		if n > count-cursor {
			n = count - cursor
		}
		var data []byte
		err = s.db.QueryRow(`SELECT data FROM puty_scratch_pages WHERE object_id=? AND page_no=?`, id, page).Scan(&data)
		if err != nil && err != sql.ErrNoRows {
			clear(result)
			return nil, err
		}
		if len(data) > 0 {
			if len(data) != ScratchPageSize {
				clear(data)
				clear(result)
				return nil, errors.New("scratch page size mismatch")
			}
			copy(result[cursor:cursor+n], data[within:within+n])
			clear(data)
		}
		cursor += n
	}
	if s.checkAuthorized() != nil {
		clear(result)
		return nil, ErrLocked
	}
	return result, nil
}
func (s *Store) CloseScratch(id string) error {
	scratchGuard.Lock()
	defer scratchGuard.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM puty_scratch_pages WHERE object_id=?`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM puty_scratch_objects WHERE object_id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

type scratchReader struct {
	store  *Store
	id     string
	offset int64
}

func (r *scratchReader) Read(p []byte) (int, error) {
	data, err := r.store.ReadScratch(r.id, r.offset, len(p))
	if err != nil {
		return 0, err
	}
	defer clear(data)
	if len(data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, data)
	r.offset += int64(n)
	return n, nil
}
func (s *Store) StageScratch(id string) (string, error) {
	var name string
	if err := s.db.QueryRow(`SELECT source_name FROM puty_scratch_objects WHERE object_id=?`, id).Scan(&name); err != nil {
		return "", err
	}
	path, err := s.StageOutgoing(&scratchReader{store: s, id: id}, name)
	if err != nil {
		return "", err
	}
	if err = s.CloseScratch(id); err != nil {
		return "", err
	}
	return path, nil
}
