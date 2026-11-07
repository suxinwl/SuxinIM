package nativestore

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var clearConfirmationClock = time.Now

func confirmedClearUpper(requested int64) int64 {
	if confirmed := clearConfirmationClock().UnixMilli(); requested > confirmed {
		return confirmed
	}
	return requested
}

// Half-open UTC millisecond scope. UI calendar dates are Asia/Shanghai.
// Bytes are actual ciphertext file sizes; SQLCipher page compaction is excluded.
type ClearScopesPreview struct {
	ConversationIDs       []string `json:"conversationIDs"`
	From                  int64    `json:"from"`
	ToExclusive           int64    `json:"toExclusive"`
	MessageCount          int      `json:"messageCount"`
	LocalImportCount      int      `json:"localImportCount"`
	AttachmentReferences  int      `json:"attachmentReferences"`
	FilesToDelete         int      `json:"filesToDelete"`
	EstimatedReleaseBytes int64    `json:"estimatedReleaseBytes"`
}

func validateClearScopes(ids []string, from, toExclusive int64) ([]string, error) {
	if len(ids) == 0 || len(ids) > 100 || from < 0 || toExclusive <= from || toExclusive > 9007199254740991 {
		return nil, errors.New("invalid local clear scope")
	}
	set := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" {
			return nil, errors.New("invalid conversation")
		}
		if _, err := quoteIdentifier("chat_logs_" + id); err != nil {
			return nil, err
		}
		if !set[id] {
			set[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (s *Store) PreviewClearScopes(ids []string, from, toExclusive int64) (result ClearScopesPreview, err error) {
	ids, err = validateClearScopes(ids, from, toExclusive)
	if err != nil {
		return result, err
	}
	result.ConversationIDs = ids
	result.From = from
	result.ToExclusive = toExclusive
	if err = s.checkAuthorized(); err != nil {
		return result, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	parts := []string{}
	args := []any{}
	for _, id := range ids {
		parts = append(parts, `SELECT conversation,message_id,0 FROM puty_archive_index WHERE conversation=? AND send_time>=? AND send_time<?`)
		args = append(args, id, from, toExclusive)
		var present int
		table := "chat_logs_" + id
		if err = tx.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&present); err != nil {
			return result, err
		}
		if present != 0 {
			q, _ := quoteIdentifier(table)
			parts = append(parts, `SELECT ?,client_msg_id,0 FROM `+q+` WHERE status<4 AND send_time>=? AND send_time<?`)
			args = append(args, id, from, toExclusive)
		}
		parts = append(parts, `SELECT conversation,local_import_id,1 FROM puty_local_imports WHERE conversation=? AND first_imported_at>=? AND first_imported_at<?`)
		args = append(args, id, from, toExclusive)
	}
	cte := `WITH selected(conversation,message_id,kind) AS (` + strings.Join(parts, ` UNION `) + `) `
	for _, item := range []struct {
		query  string
		target *int
	}{
		{`SELECT count(*) FROM selected WHERE kind=0`, &result.MessageCount},
		{`SELECT count(*) FROM selected WHERE kind=1`, &result.LocalImportCount},
		{`SELECT count(*) FROM puty_attachment_refs r WHERE EXISTS(SELECT 1 FROM selected s WHERE s.conversation=r.conversation AND s.message_id=r.message_id)`, &result.AttachmentReferences},
	} {
		if err = tx.QueryRow(cte+item.query, args...).Scan(item.target); err != nil {
			return result, err
		}
	}
	// Each physical path appears once across all selected conversations. Other
	// references, even another day in a selected conversation, keep that file.
	paths := map[string]bool{}
	collect := func(query string, base string) error {
		rows, e := tx.Query(cte+query, args...)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var rel string
			if e = rows.Scan(&rel); e != nil {
				return e
			}
			clean := filepath.Clean(rel)
			if filepath.IsAbs(rel) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
				return errors.New("invalid owned cipher path")
			}
			paths[filepath.Join(s.root, base, clean)] = true
		}
		return rows.Err()
	}
	if err = collect(`SELECT i.path FROM puty_archive_index i WHERE EXISTS(SELECT 1 FROM selected s WHERE s.conversation=i.conversation AND s.message_id=i.message_id)`, "archive"); err != nil {
		return result, err
	}
	selectedRef := `EXISTS(SELECT 1 FROM selected s WHERE s.conversation=r.conversation AND s.message_id=r.message_id)`
	if err = collect(`SELECT t.month||'/attachments/'||t.allocated_name FROM puty_attachment_tasks t WHERE EXISTS(SELECT 1 FROM puty_attachment_refs r WHERE r.task_id=t.task_id AND `+selectedRef+`) AND NOT EXISTS(SELECT 1 FROM puty_attachment_refs r WHERE r.task_id=t.task_id AND NOT `+selectedRef+`)`, "archive"); err != nil {
		return result, err
	}
	if err = collect(`SELECT o.path FROM puty_outgoing_sources o WHERE EXISTS(SELECT 1 FROM puty_outgoing_refs r WHERE r.object_id=o.object_id AND EXISTS(SELECT 1 FROM selected s WHERE s.message_id=r.message_id)) AND NOT EXISTS(SELECT 1 FROM puty_outgoing_refs r WHERE r.object_id=o.object_id AND NOT EXISTS(SELECT 1 FROM selected s WHERE s.message_id=r.message_id))`, "outgoing"); err != nil {
		return result, err
	}
	for path := range paths {
		info, e := os.Lstat(path)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return result, e
		}
		if !info.Mode().IsRegular() {
			return result, errors.New("invalid cipher file")
		}
		result.FilesToDelete++
		result.EstimatedReleaseBytes += info.Size()
		if result.EstimatedReleaseBytes > 9007199254740991 {
			return result, errors.New("clear estimate overflow")
		}
	}
	return result, s.checkAuthorized()
}

// All tombstones and thin working rows commit together. Physical cleanup then
// uses the durable IDs-only outbox; any unlink/fsync failure remains retryable.
func (s *Store) ClearScopes(ids []string, from, toExclusive int64) error {
	ids, err := validateClearScopes(ids, from, toExclusive)
	if err != nil {
		return err
	}
	if err = s.checkAuthorized(); err != nil {
		return err
	}
	maskTo := confirmedClearUpper(toExclusive - 1)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, id := range ids {
		if err = s.stageClearRange(tx, "chat_logs_"+id, id, from, toExclusive-1, maskTo); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	for _, id := range ids {
		if err = s.finishClearRange(id, from, toExclusive-1); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	return err
}

// sql.Tx keeps multi-scope suppression indivisible; no message payload is
// written into a deletion event, including hot-only/private/destruct rows.
func (s *Store) stageClearRange(tx *sql.Tx, table, conversation string, from, to, maskTo int64) error {
	q, err := quoteIdentifier(table)
	if err != nil {
		return err
	}
	// A calendar end can be later today or even unbounded. Preserve the whole
	// selection as exact IDs, but never suppress messages created after this clear.
	if from <= maskTo {
		if _, err = tx.Exec(`INSERT OR IGNORE INTO puty_local_tombstones VALUES(?,?,?)`, conversation, from, maskTo); err != nil {
			return err
		}
	}
	var present int
	if err = tx.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&present); err != nil {
		return err
	}
	if present != 0 {
		if _, err = tx.Exec(`INSERT OR IGNORE INTO puty_local_deleted SELECT ?,client_msg_id,send_time FROM `+q+` WHERE send_time BETWEEN ? AND ?`, conversation, from, to); err != nil {
			return err
		}
		if _, err = tx.Exec(`UPDATE `+q+` SET content='',attached_info='',ex='',local_ex='',status=4 WHERE send_time BETWEEN ? AND ?`, from, to); err != nil {
			return err
		}
	}
	if err = tx.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='local_conversations'`).Scan(&present); err != nil {
		return err
	}
	if present != 0 {
		if _, err = tx.Exec(`UPDATE local_conversations SET latest_msg='',latest_msg_send_time=0,unread_count=0 WHERE conversation_id=? AND latest_msg_send_time BETWEEN ? AND ?`, conversation, from, to); err != nil {
			return err
		}
		if _, err = tx.Exec(`UPDATE local_conversations SET draft_text='',draft_text_time=0 WHERE conversation_id=? AND draft_text_time BETWEEN ? AND ?`, conversation, from, to); err != nil {
			return err
		}
	}
	for _, statement := range []string{
		`DELETE FROM puty_archive_outbox WHERE conversation=? AND send_time BETWEEN ? AND ?`,
		`INSERT OR IGNORE INTO puty_local_deleted SELECT conversation,message_id,send_time FROM puty_archive_index WHERE conversation=? AND send_time BETWEEN ? AND ?`,
		`INSERT OR IGNORE INTO puty_local_deleted SELECT conversation,local_import_id,first_imported_at FROM puty_local_imports WHERE conversation=? AND first_imported_at BETWEEN ? AND ?`,
		`INSERT INTO puty_archive_outbox(conversation,message_id,send_time,operation,payload) SELECT conversation,message_id,send_time,2,'' FROM puty_local_deleted WHERE conversation=? AND send_time BETWEEN ? AND ?`,
	} {
		if _, err = tx.Exec(statement, conversation, from, to); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) finishClearRange(conversation string, from, to int64) error {
	for {
		var count int
		if err := s.db.QueryRow(`SELECT count(*) FROM puty_archive_outbox WHERE conversation=? AND send_time BETWEEN ? AND ?`, conversation, from, to).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			break
		}
		if err := s.Flush(100); err != nil {
			return err
		}
	}
	return s.ClearAttachmentReferences(conversation, from, to)
}
