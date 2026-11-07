package nativestore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
)

type LocalImportItem = AttachmentItem

// Only a native-picker encrypted source may be explicitly copied into permanent
// attachments. The original external document is never deleted or claimed.
func (s *Store) ImportLocalAttachment(source, conversation string) (item LocalImportItem, err error) {
	if _, err = quoteIdentifier(conversation); err != nil || conversation == "" {
		return item, errors.New("an explicit conversation is required")
	}
	_, name, err := s.outgoingIdentity(source)
	if err != nil {
		return item, err
	}
	// A failed prior import keeps a durable delete event until unlink/fsync
	// succeeds. Drain it before reserving another random task/name, so retry
	// cannot silently leave the old physical name occupied.
	for {
		var pending int
		if err = s.db.QueryRow(`SELECT count(*) FROM puty_archive_outbox o JOIN puty_local_imports i ON i.conversation=o.conversation AND i.local_import_id=o.message_id WHERE o.operation=2`).Scan(&pending); err != nil {
			return item, err
		}
		if pending == 0 {
			break
		}
		if err = s.Flush(1000); err != nil {
			return item, err
		}
	}
	reader, err := OpenOutgoing(source)
	if err != nil {
		return item, err
	}
	defer reader.Close()
	nonce, err := randomName()
	if err != nil {
		return item, err
	}
	item.LocalImportID = "local-import-" + nonce
	item.FirstImportedAt = namingClock().UnixMilli()
	item.ConversationID = conversation
	hash := sha256.Sum256([]byte(s.account + "\x00" + item.LocalImportID))
	item.TaskID = hex.EncodeToString(hash[:])
	if _, err = s.db.Exec(`INSERT INTO puty_local_imports VALUES(?,?,?,?)`, item.LocalImportID, conversation, item.TaskID, item.FirstImportedAt); err != nil {
		return item, err
	}
	completed := false
	defer func() {
		if !completed {
			s.AbortAttachment(item.TaskID)
			_ = s.DeleteLocalMessage("chat_logs_"+conversation, conversation, item.LocalImportID, item.FirstImportedAt)
		}
	}()
	_, err = s.BeginAttachment(item.TaskID, "local-import:"+nonce, name, conversation, item.LocalImportID)
	if err != nil {
		return item, err
	}
	buffer := make([]byte, 65536)
	defer clear(buffer)
	var size int64
	for {
		n, readErr := reader.Read(buffer)
		if n > 0 {
			if err = s.WriteAttachment(item.TaskID, buffer[:n]); err != nil {
				return item, err
			}
			size += int64(n)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return item, readErr
		}
	}
	if err = s.FinishAttachment(item.TaskID, size); err != nil {
		return item, err
	}
	info, err := s.AttachmentInfo(item.TaskID)
	if err != nil {
		return item, err
	}
	item = info
	item.ConversationID = conversation
	item.MessageID = ""
	if err = s.DiscardUnboundSource(source); err != nil {
		return item, err
	}
	completed = true
	return item, s.checkAuthorized()
}

type ClearPreview struct {
	ConversationID       string `json:"conversationID"`
	From                 int64  `json:"from"`
	To                   int64  `json:"to"`
	MessageCount         int    `json:"messageCount"`
	LocalImportCount     int    `json:"localImportCount"`
	AttachmentReferences int    `json:"attachmentReferences"`
	FilesToDelete        int    `json:"filesToDelete"`
}

func (s *Store) PreviewClear(table, conversation string, from, to int64) (result ClearPreview, err error) {
	result.ConversationID = conversation
	result.From = from
	result.To = to
	q, err := quoteIdentifier(table)
	if err != nil || conversation == "" || from < 0 || to < from {
		return result, errors.New("invalid local clear scope")
	}
	if err = s.checkAuthorized(); err != nil {
		return result, err
	}
	ids := `SELECT message_id FROM puty_archive_index WHERE conversation=? AND send_time BETWEEN ? AND ?`
	args := []any{conversation, from, to}
	var present int
	if err = s.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&present); err != nil {
		return result, err
	}
	if present != 0 {
		ids += ` UNION SELECT client_msg_id FROM ` + q + ` WHERE status<4 AND send_time BETWEEN ? AND ?`
		args = append(args, from, to)
	}
	if err = s.db.QueryRow(`SELECT count(*) FROM (`+ids+`)`, args...).Scan(&result.MessageCount); err != nil {
		return result, err
	}
	if err = s.db.QueryRow(`SELECT count(*) FROM puty_local_imports WHERE conversation=? AND first_imported_at BETWEEN ? AND ?`, conversation, from, to).Scan(&result.LocalImportCount); err != nil {
		return result, err
	}
	ids += ` UNION SELECT local_import_id FROM puty_local_imports WHERE conversation=? AND first_imported_at BETWEEN ? AND ?`
	args = append(args, conversation, from, to)
	selected := `r.conversation=? AND r.message_id IN(` + ids + `)`
	selectedArgs := append([]any{conversation}, args...)
	if err = s.db.QueryRow(`SELECT count(*) FROM puty_attachment_refs r WHERE `+selected, selectedArgs...).Scan(&result.AttachmentReferences); err != nil {
		return result, err
	}
	// A referenced file is deleted only if no reference outside the selection
	// still needs it. Counts include hot-only/private and explicit local imports.
	query := `SELECT count(*) FROM puty_attachment_tasks t WHERE EXISTS(SELECT 1 FROM puty_attachment_refs r WHERE r.task_id=t.task_id AND ` + selected + `) AND NOT EXISTS(SELECT 1 FROM puty_attachment_refs r WHERE r.task_id=t.task_id AND NOT (` + selected + `))`
	both := append(append([]any{}, selectedArgs...), selectedArgs...)
	if err = s.db.QueryRow(query, both...).Scan(&result.FilesToDelete); err != nil {
		return result, err
	}
	return result, s.checkAuthorized()
}
