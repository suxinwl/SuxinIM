//go:build android || ios

package db

import (
	"database/sql"
	"encoding/json"
	"github.com/openimsdk/openim-sdk-core/v3/pkg/db/model_struct"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	nativestore "puty.local/nativestore"
)

func (d *DataBase) openLocalWorkingDB(legacyPath string, _ logger.LogLevel) (*gorm.DB, error) {
	s, err := nativestore.Lookup(d.loginUserID)
	if err != nil {
		return nil, err
	}
	if err = s.MigrateLegacy(legacyPath); err != nil {
		return nil, err
	}
	connection, err := s.OpenWorkingDatabase()
	if err != nil {
		return nil, err
	}
	if err = s.MigrateLegacyMedia(); err != nil {
		_ = d.closeSDKWorkingDatabase(connection)
		return nil, err
	}
	if err = s.QuarantineLegacyDatabases(); err != nil {
		_ = d.closeSDKWorkingDatabase(connection)
		return nil, err
	}
	// SQL text may contain messages/tokens, so native vault databases never use
	// the upstream Info SQL logger even when old callers pass logLevel=6.
	db, err := gorm.Open(sqlite.Dialector{Conn: connection}, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	// Native SDK history keeps its original protocol/model. Cold rows are
	// hydrated after the bounded SQL query, never copied back into hot storage.
	err = db.Callback().Query().After("gorm:query").Register("puty:cold_history", func(query *gorm.DB) {
		if query.Error != nil || !strings.HasPrefix(query.Statement.Table, "chat_logs_") {
			return
		}
		conversation := strings.TrimPrefix(query.Statement.Table, "chat_logs_")
		hydrate := func(row *model_struct.LocalChatLog) {
			if row == nil || row.Content != "" || row.Status != 2 || row.Seq <= 0 {
				return
			}
			payload, readErr := s.RecordPayload(conversation, row.ClientMsgID)
			if readErr != nil {
				query.AddError(readErr)
				return
			}
			if payload == nil {
				return
			}
			for key, destination := range map[string]*string{"content": &row.Content, "attached_info": &row.AttachedInfo, "ex": &row.Ex, "local_ex": &row.LocalEx} {
				if value, ok := payload[key]; ok {
					if readErr = json.Unmarshal(value, destination); readErr != nil {
						query.AddError(readErr)
						return
					}
				}
			}
		}
		switch rows := query.Statement.Dest.(type) {
		case *model_struct.LocalChatLog:
			hydrate(rows)
		case *[]model_struct.LocalChatLog:
			for i := range *rows {
				hydrate(&(*rows)[i])
			}
		case *[]*model_struct.LocalChatLog:
			for _, row := range *rows {
				hydrate(row)
			}
		}
	})
	return db, err
}

// The native Store owns this handle. SDK logout drains its workers without
// closing the database that backup/restore and native readers still borrow.
func (d *DataBase) closeSDKWorkingDatabase(connection *sql.DB) error {
	return nativestore.CloseSDKWorkingDatabase(d.loginUserID, connection)
}

func (d *DataBase) installLocalArchive(table, conversation string) error {
	s, err := nativestore.Lookup(d.loginUserID)
	if err != nil {
		return err
	}
	return s.InstallConversation(table, conversation)
}
func (d *DataBase) startLocalArchive(tables []string) error {
	s, err := nativestore.Lookup(d.loginUserID)
	if err != nil {
		return err
	}
	for _, table := range tables {
		if strings.HasPrefix(table, "chat_logs_") {
			if err = s.InstallConversation(table, strings.TrimPrefix(table, "chat_logs_")); err != nil {
				return err
			}
		}
	}
	d.archiveMu.Lock()
	d.archiveStop = make(chan struct{})
	d.archiveDone = make(chan struct{})
	stop, done := d.archiveStop, d.archiveDone
	d.archiveMu.Unlock()
	go func() {
		// The final flush uses this worker's Store, never a later account owner.
		// Close waits for it before restore may inspect or promote working.db.
		defer close(done)
		defer func() { _ = s.Flush(1000) }()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		ticks := 0
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if s.Flush(100) == nil {
					ticks++
					if ticks == 1 || ticks%720 == 0 {
						d.mRWMutex.Lock()
						currentTables, _ := d.GetExistTablesForArchive()
						for _, table := range currentTables {
							if strings.HasPrefix(table, "chat_logs_") {
								_ = s.EvictHot(table, strings.TrimPrefix(table, "chat_logs_"), time.Now().AddDate(0, 0, -90).UnixMilli())
							}
						}
						d.mRWMutex.Unlock()
					}
				}
			}
		}
	}()
	return nil
}
func (d *DataBase) GetExistTablesForArchive() ([]string, error) {
	var tables []string
	err := d.conn.Raw(`SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'chat_logs_%'`).Scan(&tables).Error
	return tables, err
}
func (d *DataBase) stopLocalArchive() {
	d.archiveMu.Lock()
	if d.archiveStop != nil {
		close(d.archiveStop)
		d.archiveStop = nil
	}
	done := d.archiveDone
	d.archiveMu.Unlock()
	// Do not hold mRWMutex: an in-flight eviction must finish before this fence.
	// Keep archiveDone for concurrent/repeated SDK Close calls to wait on too.
	if done != nil {
		<-done
	}
}
