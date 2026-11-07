package nativestore

import (
	"errors"
	"path/filepath"
)

type AttachmentItem struct {
	TaskID          string `json:"taskID"`
	Name            string `json:"name"`
	Month           string `json:"month"`
	Size            int64  `json:"size"`
	ConversationID  string `json:"conversationID"`
	MessageID       string `json:"messageID"`
	LocalImportID   string `json:"localImportID,omitempty"`
	FirstImportedAt int64  `json:"firstImportedAt,omitempty"`
}
type AttachmentPage struct {
	Items      []AttachmentItem `json:"items"`
	NextCursor string           `json:"nextCursor,omitempty"`
}

func (s *Store) AttachmentInventory(conversation, cursor string, limit int) (AttachmentPage, error) {
	result := AttachmentPage{Items: []AttachmentItem{}}
	if limit < 1 || limit > 100 || len(cursor) > 256 {
		return result, errors.New("invalid attachment page")
	}
	s.mu.Lock()
	ready := s.authorizedLocked() && s.viewsReady
	s.mu.Unlock()
	if !ready {
		return result, ErrLocked
	}
	rows, err := s.db.Query(`SELECT t.task_id,t.allocated_name,t.month,t.bytes,COALESCE(MIN(r.conversation),''),CASE WHEN l.local_import_id IS NULL THEN COALESCE(MIN(r.message_id),'') ELSE '' END,COALESCE(l.local_import_id,''),COALESCE(l.first_imported_at,0) FROM puty_attachment_tasks t LEFT JOIN puty_attachment_refs r ON r.task_id=t.task_id LEFT JOIN puty_local_imports l ON l.task_id=t.task_id WHERE t.state='complete' AND t.task_id>? AND (?='' OR r.conversation=?) GROUP BY t.task_id ORDER BY t.task_id LIMIT ?`, cursor, conversation, conversation, limit+1)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var i AttachmentItem
		if err = rows.Scan(&i.TaskID, &i.Name, &i.Month, &i.Size, &i.ConversationID, &i.MessageID, &i.LocalImportID, &i.FirstImportedAt); err != nil {
			_ = rows.Close()
			return result, err
		}
		result.Items = append(result.Items, i)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return result, err
	}
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
		result.NextCursor = result.Items[limit-1].TaskID
	}
	s.mu.Lock()
	ready = s.authorizedLocked() && s.viewsReady
	s.mu.Unlock()
	if !ready {
		return AttachmentPage{}, ErrLocked
	}
	return result, nil
}
func (s *Store) AttachmentInfo(task string) (AttachmentItem, error) {
	var i AttachmentItem
	i.TaskID = task
	err := s.db.QueryRow(`SELECT t.allocated_name,t.month,t.bytes,COALESCE(l.local_import_id,''),COALESCE(l.conversation,''),COALESCE(l.first_imported_at,0) FROM puty_attachment_tasks t LEFT JOIN puty_local_imports l ON l.task_id=t.task_id WHERE t.task_id=? AND t.state='complete'`, task).Scan(&i.Name, &i.Month, &i.Size, &i.LocalImportID, &i.ConversationID, &i.FirstImportedAt)
	if err != nil {
		return i, err
	}
	return i, s.checkAuthorized()
}
func (s *Store) AttachmentCipherPath(task string) (string, error) {
	i, err := s.AttachmentInfo(task)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.root, "archive", i.Month, "attachments", i.Name), nil
}
