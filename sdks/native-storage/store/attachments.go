package nativestore

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"

	localstore "puty.local/localstore"
)

type attachmentWrite struct {
	file                *os.File
	writer              *localstore.ChunkWriter
	name, month, object string
}

var attachmentGuard sync.Mutex
var attachmentWrites = map[string]*attachmentWrite{}

func attachmentKey(account, task string) string { return account + "\x00" + task }

// BeginAttachment allocates the actual original filename with exclusive create.
// Its encrypted SQLCipher task row retains that same name/month on every retry.
// No plaintext .partial or alternate objectID.enc file is ever created.
func (s *Store) BeginAttachment(task, object, source, conversation, message string) (string, error) {
	if len(task) < 1 || len(task) > 256 || len(object) < 1 || len(object) > 4096 {
		return "", errors.New("invalid attachment identity")
	}
	if err := localstore.ValidateSourceName(source); err != nil {
		return "", err
	}
	s.mu.Lock()
	if !s.authorizedLocked() {
		s.mu.Unlock()
		return "", ErrLocked
	}
	key := bytes.Clone(s.master)
	s.mu.Unlock()
	attachmentGuard.Lock()
	defer attachmentGuard.Unlock()
	if conversation != "" && message != "" {
		var deleted int
		if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM puty_local_deleted WHERE conversation=? AND message_id=?)`, conversation, message).Scan(&deleted); err != nil || deleted != 0 {
			clear(key)
			return "", errors.New("locally cleared message cannot acquire an attachment")
		}
	}
	id := attachmentKey(s.account, task)
	if attachmentWrites[id] != nil {
		clear(key)
		return "", errors.New("attachment task already active")
	}
	var name, month, oldObject, oldSource, state string
	err := s.db.QueryRow(`SELECT allocated_name,month,object_id,source_name,state FROM puty_attachment_tasks WHERE task_id=?`, task).Scan(&name, &month, &oldObject, &oldSource, &state)
	if err == nil && (oldObject != object || oldSource != source) {
		clear(key)
		return "", errors.New("attachment task identity changed")
	}
	var file *os.File
	var writer *localstore.ChunkWriter
	context := localstore.ObjectIdentity{Account: s.account, Type: "attachment", ObjectID: object}
	if err == nil {
		path := filepath.Join(s.root, "archive", month, "attachments", name)
		if state == "complete" {
			if conversation != "" && message != "" {
				_, err = s.db.Exec(`INSERT OR IGNORE INTO puty_attachment_refs VALUES(?,?,?)`, conversation, message, task)
			}
			clear(key)
			return path, err
		}
		previous, openErr := os.Open(path)
		if openErr != nil {
			clear(key)
			return "", openErr
		}
		file, err = os.CreateTemp(filepath.Dir(path), ".resume-")
		if err != nil {
			_ = previous.Close()
			clear(key)
			return "", err
		}
		temporary := file.Name()
		writer, err = localstore.NewChunkWriter(file, key, context)
		var offset int64
		var complete bool
		if err == nil {
			offset, complete, err = localstore.RewrapPending(previous, writer, key, context)
		}
		_ = previous.Close()
		if err == nil && complete {
			err = writer.Finish()
		}
		if err == nil {
			err = file.Sync()
		}
		if err == nil {
			err = os.Rename(temporary, path)
		}
		if err == nil {
			err = syncDirectory(filepath.Dir(path))
		}
		if err != nil {
			if writer != nil {
				writer.Abort()
			}
			_ = file.Close()
			_ = os.Remove(temporary)
			clear(key)
			return "", err
		}
		if _, err = s.db.Exec(`UPDATE puty_attachment_tasks SET bytes=?,state=? WHERE task_id=?`, offset, map[bool]string{true: "complete", false: "pending"}[complete], task); err != nil {
			writer.Abort()
			_ = file.Close()
			clear(key)
			return "", err
		}
		if complete {
			if conversation != "" && message != "" {
				_, err = s.db.Exec(`INSERT OR IGNORE INTO puty_attachment_refs VALUES(?,?,?)`, conversation, message, task)
			}
			_ = file.Close()
			clear(key)
			return path, err
		}
	} else {
		if !errors.Is(err, sql.ErrNoRows) {
			clear(key)
			return "", err
		}
		month = namingClock().In(CompanyZone).Format("200601")
		directory := filepath.Join(s.root, "archive", month, "attachments")
		reservation, reserveErr := localstore.ReserveName(directory, source)
		if reserveErr != nil {
			clear(key)
			return "", reserveErr
		}
		file = reservation.File
		name = reservation.Name
		if _, err = s.db.Exec(`INSERT INTO puty_attachment_tasks(task_id,object_id,source_name,allocated_name,month,state) VALUES(?,?,?,?,?,'pending')`, task, object, source, name, month); err != nil {
			_ = reservation.Abort()
			clear(key)
			return "", err
		}
		writer, err = localstore.NewChunkWriter(file, key, context)
	}

	if err != nil {
		if writer != nil {
			writer.Abort()
		}
		if file != nil {
			_ = file.Close()
		}
		clear(key)
		return "", err
	}
	if conversation != "" && message != "" {
		if _, err = s.db.Exec(`INSERT OR IGNORE INTO puty_attachment_refs VALUES(?,?,?)`, conversation, message, task); err != nil {
			_ = file.Close()
			clear(key)
			return "", err
		}
	}
	clear(key)
	attachmentWrites[id] = &attachmentWrite{file: file, writer: writer, name: name, month: month, object: object}
	return filepath.Join(s.root, "archive", month, "attachments", name), nil
}
func (s *Store) WriteAttachment(task string, chunk []byte) error {
	s.mu.Lock()
	allowed := s.authorizedLocked() && s.viewsReady
	s.mu.Unlock()
	if !allowed {
		return ErrLocked
	}
	if len(chunk) > localstore.ChunkSize {
		return errors.New("attachment chunk too large")
	}
	attachmentGuard.Lock()
	defer attachmentGuard.Unlock()
	write := attachmentWrites[attachmentKey(s.account, task)]
	if write == nil {
		return errors.New("attachment task not active")
	}
	before := write.writer.DurablePlaintextBytes()
	if _, err := write.writer.Write(chunk); err != nil {
		return err
	}
	if write.writer.DurablePlaintextBytes() != before {
		if err := write.file.Sync(); err != nil {
			return err
		}
		_, err := s.db.Exec(`UPDATE puty_attachment_tasks SET bytes=? WHERE task_id=?`, write.writer.DurablePlaintextBytes(), task)
		return err
	}
	return nil
}
func (s *Store) FinishAttachment(task string, expectedSize int64) error {
	attachmentGuard.Lock()
	defer attachmentGuard.Unlock()
	id := attachmentKey(s.account, task)
	write := attachmentWrites[id]
	if write == nil {
		return errors.New("attachment task not active")
	}
	delete(attachmentWrites, id)
	defer write.file.Close()
	if expectedSize >= 0 && write.writer.TotalPlaintextBytes() != expectedSize {
		write.writer.Abort()
		_ = write.file.Sync()
		return errors.New("attachment length mismatch")
	}
	total := write.writer.TotalPlaintextBytes()
	if err := write.writer.Finish(); err != nil {
		return err
	}
	if err := write.file.Sync(); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(write.file.Name())); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE puty_attachment_tasks SET state='complete',bytes=? WHERE task_id=?`, total, task)
	return err
}
func (s *Store) AbortAttachment(task string) {
	attachmentGuard.Lock()
	defer attachmentGuard.Unlock()
	id := attachmentKey(s.account, task)
	if write := attachmentWrites[id]; write != nil {
		delete(attachmentWrites, id)
		write.writer.Abort()
		_ = write.file.Sync()
		_ = write.file.Close()
	}
}
func (s *Store) AttachmentOffset(task string) int64 {
	attachmentGuard.Lock()
	defer attachmentGuard.Unlock()
	if write := attachmentWrites[attachmentKey(s.account, task)]; write != nil {
		return write.writer.DurablePlaintextBytes()
	}
	return -1
}

func (s *Store) AttachmentETag(task string) string {
	var etag string
	_ = s.db.QueryRow(`SELECT etag FROM puty_attachment_tasks WHERE task_id=?`, task).Scan(&etag)
	return etag
}
func (s *Store) SetAttachmentETag(task, etag string) error {
	if len(etag) > 256 {
		return errors.New("invalid object validator")
	}
	for _, c := range etag {
		if c < 32 || c > 126 {
			return errors.New("invalid object validator")
		}
	}
	_, err := s.db.Exec(`UPDATE puty_attachment_tasks SET etag=? WHERE task_id=?`, etag, task)
	return err
}
func (s *Store) ResetAttachment(task string) error {
	s.AbortAttachment(task)
	var month, name string
	if err := s.db.QueryRow(`SELECT month,allocated_name FROM puty_attachment_tasks WHERE task_id=?`, task).Scan(&month, &name); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(s.root, "archive", month, "attachments", name), os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	err = file.Sync()
	_ = file.Close()
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE puty_attachment_tasks SET state='pending',bytes=0,etag='' WHERE task_id=?`, task)
	return err
}

// ReadAttachment authenticates to a private in-memory buffer, then returns it.
// No bytes are exposed if the end marker or any AEAD frame is missing/modified.
func (s *Store) ReadAttachment(task string, maxBytes int64) ([]byte, error) {
	if maxBytes < 0 || maxBytes > 32<<20 {
		return nil, errors.New("protected preview exceeds memory limit")
	}
	s.mu.Lock()
	if !s.authorizedLocked() || !s.viewsReady {
		s.mu.Unlock()
		return nil, ErrLocked
	}
	key := bytes.Clone(s.master)
	s.mu.Unlock()
	defer clear(key)
	var name, month, object, state string
	var size int64
	if err := s.db.QueryRow(`SELECT allocated_name,month,object_id,state,bytes FROM puty_attachment_tasks WHERE task_id=?`, task).Scan(&name, &month, &object, &state, &size); err != nil {
		return nil, err
	}
	if state != "complete" || size > maxBytes {
		return nil, errors.New("protected attachment unavailable")
	}
	file, err := os.Open(filepath.Join(s.root, "archive", month, "attachments", name))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var output bytes.Buffer
	if err = localstore.Decrypt(&output, file, key, localstore.ObjectIdentity{Account: s.account, Type: "attachment", ObjectID: object}); err != nil {
		return nil, err
	}
	if int64(output.Len()) != size {
		return nil, errors.New("protected attachment length mismatch")
	}
	s.mu.Lock()
	allowed := s.authorizedLocked()
	s.mu.Unlock()
	if !allowed {
		clear(output.Bytes())
		return nil, ErrLocked
	}
	return output.Bytes(), nil
}

func (s *Store) ClearAttachmentReferences(conversation string, from, to int64) error {
	attachmentGuard.Lock()
	defer attachmentGuard.Unlock()
	// Only refs belonging to IDs selected by the local clear interval are removed.
	rows, err := s.db.Query(`SELECT DISTINCT t.task_id,t.month,t.allocated_name FROM puty_attachment_tasks t JOIN puty_attachment_refs r ON r.task_id=t.task_id JOIN puty_local_deleted d ON d.conversation=r.conversation AND d.message_id=r.message_id WHERE r.conversation=? AND d.send_time BETWEEN ? AND ?`, conversation, from, to)
	if err != nil {
		return err
	}
	type orphan struct{ task, month, name string }
	var items []orphan
	for rows.Next() {
		var i orphan
		if err = rows.Scan(&i.task, &i.month, &i.name); err != nil {
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
		var refs int
		if err = s.db.QueryRow(`SELECT count(*) FROM puty_attachment_refs WHERE task_id=? AND NOT(conversation=? AND message_id IN(SELECT message_id FROM puty_local_deleted WHERE conversation=? AND send_time BETWEEN ? AND ?))`, i.task, conversation, conversation, from, to).Scan(&refs); err != nil {
			return err
		}
		if refs > 0 {
			if _, err = s.db.Exec(`DELETE FROM puty_attachment_refs WHERE task_id=? AND conversation=? AND message_id IN(SELECT message_id FROM puty_local_deleted WHERE conversation=? AND send_time BETWEEN ? AND ?)`, i.task, conversation, conversation, from, to); err != nil {
				return err
			}
			continue
		}
		id := attachmentKey(s.account, i.task)
		if write := attachmentWrites[id]; write != nil {
			delete(attachmentWrites, id)
			write.writer.Abort()
			_ = write.file.Sync()
			_ = write.file.Close()
		}
		if err = removeOwnedArchive(s.root, filepath.Join(i.month, "attachments", i.name)); err != nil {
			return err
		}
		s.discardRangeReader("attachment:" + i.task)
		// Preserve exact metadata/refs until unlink + directory fsync succeed.
		// Missing files are an idempotent retry, never a reason to lose context.
		tx, beginErr := s.db.Begin()
		if beginErr != nil {
			return beginErr
		}
		if _, err = tx.Exec(`DELETE FROM puty_attachment_refs WHERE task_id=?`, i.task); err == nil {
			_, err = tx.Exec(`DELETE FROM puty_attachment_tasks WHERE task_id=?`, i.task)
		}
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) AttachmentComplete(task string) bool {
	var state string
	return s.db.QueryRow(`SELECT state FROM puty_attachment_tasks WHERE task_id=?`, task).Scan(&state) == nil && state == "complete"
}

func (s *Store) clearMessageAttachmentReferences(conversation, message string) error {
	var sent int64
	err := s.db.QueryRow(`SELECT send_time FROM puty_local_deleted WHERE conversation=? AND message_id=?`, conversation, message).Scan(&sent)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	return s.ClearAttachmentReferences(conversation, sent, sent)
}
func abortAccountAttachments(account string) {
	attachmentGuard.Lock()
	defer attachmentGuard.Unlock()
	for id, write := range attachmentWrites {
		if len(id) > len(account) && id[:len(account)+1] == account+"\x00" {
			delete(attachmentWrites, id)
			write.writer.Abort()
			_ = write.file.Sync()
			_ = write.file.Close()
		}
	}
}
