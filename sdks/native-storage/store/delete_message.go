package nativestore

import "errors"

// DeleteLocalMessage merges an exact deletion into a restored working store.
// It never expands the selection to unrelated messages sharing the timestamp.
func (s *Store) DeleteLocalMessage(table, conversation, message string, sent int64) error {
	q, err := quoteIdentifier(table)
	if err != nil || conversation == "" || message == "" || sent < 0 {
		return errors.New("invalid local message deletion")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT OR IGNORE INTO puty_local_deleted VALUES(?,?,?)`, conversation, message, sent); err != nil {
		return err
	}
	var present int
	if err = tx.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&present); err != nil {
		return err
	}
	if present != 0 {
		if _, err = tx.Exec(`UPDATE `+q+` SET content='',attached_info='',ex='',local_ex='',status=4 WHERE client_msg_id=?`, message); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`DELETE FROM puty_archive_outbox WHERE conversation=? AND message_id=?`, conversation, message); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO puty_archive_outbox(conversation,message_id,send_time,operation,payload)VALUES(?,?,?,2,'')`, conversation, message, sent); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return s.Flush(1000)
}
